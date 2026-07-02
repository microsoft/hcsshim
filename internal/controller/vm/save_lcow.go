//go:build windows && lcow

package vm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	vmsave "github.com/Microsoft/hcsshim/internal/controller/vm/save"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/wclayer"

	"github.com/Microsoft/go-winio"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Save captures the migrating VM's state into a serialized snapshot that the
// destination host consumes to recreate an equivalent VM.
func (c *Controller) Save(ctx context.Context) (*anypb.Any, error) {
	// CompatibilityInfo takes its own read lock; fetch it before acquiring
	// ours to avoid recursive RLock acquisition.
	compatInfo, err := c.CompatibilityInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("get compatibility info: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Save is only valid once the source has begun migrating.
	if c.vmState != StateSourceMigrating {
		return nil, fmt.Errorf("cannot save VM: VM is in state %s", c.vmState)
	}

	// Seed the payload with the VM identity, creation options, and compat blob.
	state := &vmsave.Payload{
		SchemaVersion:  vmsave.SchemaVersion,
		VmID:           c.vmID,
		SandboxOptions: sandboxOptionsToProto(c.sandboxOptions),
		CompatInfo:     compatInfo,
	}

	// Ship the final HCS ComputeSystem document so the destination can
	// recreate an identical VM. We encode it as JSON because the schema is
	// owned by hcsschema (not protobuf) and JSON is the canonical wire
	// format HCS itself consumes.
	if c.hcsDocument != nil {
		docBytes, err := json.Marshal(c.hcsDocument)
		if err != nil {
			return nil, fmt.Errorf("marshal hcs document: %w", err)
		}

		state.HcsDocument = docBytes
	}

	if c.scsiController != nil {
		s, err := c.scsiController.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("save scsi controller: %w", err)
		}

		state.Scsi = s
	}

	// VPCI and Plan9 carry no transferable state today; Save fails if any
	// is present so unsupported topologies surface instead of silently dropping.
	if c.vpciController != nil {
		if err := c.vpciController.Save(); err != nil {
			return nil, fmt.Errorf("save vpci controller: %w", err)
		}
	}

	if c.plan9Controller != nil {
		if err := c.plan9Controller.Save(); err != nil {
			return nil, fmt.Errorf("save plan9 controller: %w", err)
		}
	}

	// Capture the GCS port and bridge-id allocator floors so the destination
	// resumes its allocators above ids the guest still has outstanding.
	if p := c.guest.NextPort(); p != 0 {
		state.GcsNextPort = p
	}

	if id := c.guest.BridgeNextID(); id != 0 {
		state.BridgeNextID = id
	}

	payload, err := proto.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal vm saved state: %w", err)
	}

	log.G(ctx).WithField(logfields.UVMID, c.vmID).Debug("saved VM migration state")
	return &anypb.Any{TypeUrl: vmsave.TypeURL, Value: payload}, nil
}

// Import rebuilds a controller's static state from a snapshot produced by
// Save. The controller comes back inert in the migrating state and performs no
// live work until Resume supplies the running VM.
func (c *Controller) Import(ctx context.Context, env *anypb.Any) error {
	if env == nil {
		return fmt.Errorf("vm saved-state envelope is nil")
	}

	// Reject envelopes that did not originate from a compatible Save.
	if env.GetTypeUrl() != vmsave.TypeURL {
		return fmt.Errorf("unsupported vm saved-state type %q", env.GetTypeUrl())
	}

	state := &vmsave.Payload{}
	if err := proto.Unmarshal(env.GetValue(), state); err != nil {
		return fmt.Errorf("unmarshal vm saved state: %w", err)
	}

	// Reject payloads written by an incompatible shim version.
	if v := state.GetSchemaVersion(); v != vmsave.SchemaVersion {
		return fmt.Errorf("unsupported vm saved-state schema version %d (want %d)", v, vmsave.SchemaVersion)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// We can import a new VM only on a freshly created controller.
	if c.vmState != StateNotCreated {
		return fmt.Errorf("unsupported vm state during Import %q", c.vmState)
	}

	// Restore the VM identity, allocator floors, and compat blob, then mark
	// the controller migrating so only migration APIs are permitted.
	c.vmID = state.GetVmID()
	c.sandboxOptions = sandboxOptionsFromProto(state.GetSandboxOptions())
	if c.sandboxOptions != nil {
		c.isPhysicallyBacked = c.sandboxOptions.FullyPhysicallyBacked
	}
	c.nextGuestPort = state.GetGcsNextPort()
	c.nextBridgeID = state.GetBridgeNextID()
	c.compatInfo = state.GetCompatInfo()
	c.vmState = StateMigratingImported

	// Decode the HCS document so [Controller.CreateVM] (called next on the
	// destination with MigrationOptions populated) can reuse it verbatim.
	if raw := state.GetHcsDocument(); len(raw) > 0 {
		doc := &hcsschema.ComputeSystem{}
		if err := json.Unmarshal(raw, doc); err != nil {
			return fmt.Errorf("unmarshal hcs document: %w", err)
		}

		c.hcsDocument = doc
	}

	// Import the SCSI sub-controller.
	if env := state.GetScsi(); env != nil {
		s, err := scsi.Import(ctx, env)
		if err != nil {
			return fmt.Errorf("import scsi controller: %w", err)
		}
		c.scsiController = s
	}

	log.G(ctx).Debug("imported VM migration state")
	return nil
}

// Patch grants the migrated VM filesystem access to its backing disk paths on
// the destination host, readying it for [Controller.Resume]. Run after the
// disk locations have been rewritten to their destination-local paths.
func (c *Controller) Patch(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.vmState != StateMigratingImported && c.vmState != StateMigratingCreated {
		return fmt.Errorf("cannot patch VM: VM is in state %s", c.vmState)
	}

	if c.scsiController == nil {
		return fmt.Errorf("cannot patch VM: SCSI controller is nil")
	}

	// Grant access only for disk types whose host paths the VM must reach.
	for _, cfg := range c.scsiController.Disks() {
		if cfg.Type != disk.TypeVirtualDisk && cfg.Type != disk.TypePassThru {
			continue
		}
		if err := wclayer.GrantVmAccess(ctx, c.vmID, cfg.HostPath); err != nil {
			return fmt.Errorf("grant vm access to %s: %w", cfg.HostPath, err)
		}
	}

	log.G(ctx).WithField(logfields.UVMID, c.vmID).Debug("patched VM disk access for migration")
	return nil
}

// Resume reactivates a migrated VM and returns it to the running state. The
// source side rebuilds its guest bridge to recover outstanding RPCs; the
// destination side reuses the connection already armed at start.
func (c *Controller) Resume(ctx context.Context, rebuildBridge bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Resume returns either migration side to the running state.
	if c.vmState != StateSourceMigrating && c.vmState != StateDestinationMigrating {
		return fmt.Errorf("cannot resume from migration: VM is in state %s", c.vmState)
	}

	switch {
	case rebuildBridge:
		// Source rollback: re-arm the listener and swap the bridge transport
		// onto the fresh hvsock so outstanding RPCs (e.g. WaitForProcess) survive.
		if err := c.guest.PrepareConnection(winio.VsockServiceID(prot.LinuxGcsVsockPort)); err != nil {
			return fmt.Errorf("prepare guest connection on resume: %w", err)
		}
		if err := c.guest.ResumeConnection(ctx); err != nil {
			return fmt.Errorf("resume guest connection: %w", err)
		}
	default:
		// Destination: reuse the connection already armed at start.
		if err := c.guest.CreateConnection(ctx, false); err != nil {
			return fmt.Errorf("resume guest connection: %w", err)
		}
	}

	// Clear migrating flag only now that the new transport is in place.
	c.guest.SetMigrating(false)

	// Lift the GCS port and bridge-id allocators above the values the guest
	// still has outstanding so newly issued ids cannot collide.
	if c.nextGuestPort != 0 {
		c.guest.SetNextPort(c.nextGuestPort)
	}

	if c.nextBridgeID != 0 {
		// Seed before sub-controller Resume so pre-registered ids stay below new ones.
		c.guest.SeedBridgeNextID(c.nextBridgeID)
	}

	// Sub-controller Resume: required on destination, no-op on source.
	if c.scsiController != nil {
		c.scsiController.Resume(ctx, c.uvm, c.guest)
	}

	c.vmState = StateRunning

	if c.sandboxOptions != nil {
		c.sandboxOptions.LiveMigrationSupportEnabled = true
	}

	// Destination never ran setupLoggingListener; close so [Controller.Wait]
	// does not block. Already closed on source — receive falls through.
	select {
	case <-c.logOutputDone:
	default:
		close(c.logOutputDone)
	}

	log.G(ctx).WithField(logfields.UVMID, c.vmID).Debug("resumed VM from migration")
	return nil
}

// sandboxOptionsToProto converts the in-memory sandbox options into their
// wire form for inclusion in a migration payload.
func sandboxOptionsToProto(o *lcow.SandboxOptions) *vmsave.SandboxOptions {
	if o == nil {
		return nil
	}
	return &vmsave.SandboxOptions{
		NoWritableFileShares:    o.NoWritableFileShares,
		EnableScratchEncryption: o.EnableScratchEncryption,
		PolicyBasedRouting:      o.PolicyBasedRouting,
		Architecture:            o.Architecture,
		FullyPhysicallyBacked:   o.FullyPhysicallyBacked,
	}
}

// sandboxOptionsFromProto reconstructs the in-memory sandbox options from a
// migration payload's wire form.
func sandboxOptionsFromProto(p *vmsave.SandboxOptions) *lcow.SandboxOptions {
	if p == nil {
		return nil
	}
	return &lcow.SandboxOptions{
		NoWritableFileShares:    p.GetNoWritableFileShares(),
		EnableScratchEncryption: p.GetEnableScratchEncryption(),
		PolicyBasedRouting:      p.GetPolicyBasedRouting(),
		Architecture:            p.GetArchitecture(),
		FullyPhysicallyBacked:   p.GetFullyPhysicallyBacked(),
	}
}
