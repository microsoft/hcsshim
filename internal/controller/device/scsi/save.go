//go:build windows && (lcow || wcow)

package scsi

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	scsisave "github.com/Microsoft/hcsshim/internal/controller/device/scsi/save"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/go-winio/pkg/guid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Save returns a serialized snapshot of the controller's current state,
// suitable for transferring to another host during live migration. After it
// returns, all operations are rejected until migration is resumed.
func (c *Controller) Save(ctx context.Context) (*anypb.Any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Capture the topology, attached disks, and outstanding reservations.
	state := &scsisave.Payload{
		SchemaVersion:  scsisave.SchemaVersion,
		NumControllers: uint32(len(c.controllerSlots) / numLUNsPerController),
		Disks:          make(map[uint32]*scsisave.DiskState, len(c.disksByPath)),
		Reservations:   make(map[string]*scsisave.Reservation, len(c.reservations)),
	}

	// Save every occupied slot.
	for slot, d := range c.controllerSlots {
		if d == nil {
			continue
		}
		ds, err := d.Save()
		if err != nil {
			return nil, err
		}
		state.Disks[uint32(slot)] = ds
	}

	// Save all the reservations.
	for id, r := range c.reservations {
		state.Reservations[id.String()] = &scsisave.Reservation{
			Slot:      uint32(r.controllerSlot),
			Partition: r.partition,
		}
	}

	payload, err := proto.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal scsi saved state: %w", err)
	}

	// Block all further operations until migration is resumed.
	c.isMigrating = true

	log.G(ctx).Debug("saved scsi controller state")
	return &anypb.Any{TypeUrl: scsisave.TypeURL, Value: payload}, nil
}

// Import reconstructs a controller from a snapshot produced by [Controller.Save].
// The result cannot serve disk operations until [Controller.Resume] supplies the
// live host and guest interfaces.
func Import(ctx context.Context, env *anypb.Any) (*Controller, error) {
	if env == nil {
		return nil, fmt.Errorf("scsi saved-state envelope is nil")
	}

	// Reject payloads that did not come from a compatible Save.
	if env.GetTypeUrl() != scsisave.TypeURL {
		return nil, fmt.Errorf("unsupported scsi saved-state type %q", env.GetTypeUrl())
	}

	// Unmarshall the payload.
	state := &scsisave.Payload{}
	if err := proto.Unmarshal(env.GetValue(), state); err != nil {
		return nil, fmt.Errorf("unmarshal scsi saved state: %w", err)
	}

	// Reject payloads written by an incompatible shim version.
	if v := state.GetSchemaVersion(); v != scsisave.SchemaVersion {
		return nil, fmt.Errorf("unsupported scsi saved-state schema version %d (want %d)", v, scsisave.SchemaVersion)
	}

	// Create a new controller.
	numCtrls := int(state.GetNumControllers())
	c := &Controller{
		reservations:    make(map[guid.GUID]*reservation, len(state.GetReservations())),
		disksByPath:     make(map[string]int, len(state.GetDisks())),
		controllerSlots: make([]*disk.Disk, numCtrls*numLUNsPerController),
		isMigrating:     true,
	}

	// Place each saved disk back at its original slot.
	for slot, ds := range state.GetDisks() {
		idx := int(slot)
		if idx >= len(c.controllerSlots) {
			return nil, fmt.Errorf("invalid controller slot: %d", slot)
		}

		// Derive the controller and LUN from the slot index and rebuild the disk.
		controller, lun := uint(idx/numLUNsPerController), uint(idx%numLUNsPerController)
		d := disk.Import(ds, controller, lun)
		if d == nil {
			return nil, fmt.Errorf("failed to import disk at controller=%d lun=%d", controller, lun)
		}

		// Store the disk and index it by host path for later lookups.
		c.controllerSlots[idx] = d
		if hp := ds.GetConfig().GetHostPath(); hp != "" {
			c.disksByPath[hp] = idx
		}
	}

	// Rehydrate all the reservations.
	for idStr, r := range state.GetReservations() {
		// Skip any reservation whose ID cannot be parsed.
		id, err := guid.FromString(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid reservation id %q: %w", idStr, err)
		}

		c.reservations[id] = &reservation{
			controllerSlot: int(r.GetSlot()),
			partition:      r.GetPartition(),
		}
	}

	log.G(ctx).Debug("imported scsi controller state")
	return c, nil
}

// Resume binds the live host and guest interfaces to an imported controller,
// enabling normal disk operations. It must be called on the destination
// before any reserve, attach, or mount calls.
func (c *Controller) Resume(ctx context.Context, vm VMSCSIOps, guest GuestSCSIOps) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.vm = vm
	c.guest = guest
	c.isMigrating = false

	log.G(ctx).Debug("resumed scsi controller")
}

// Disks returns the configuration of every disk currently attached to the
// controller.
func (c *Controller) Disks() []disk.Config {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Collect the config of each indexed disk.
	configs := make([]disk.Config, 0, len(c.disksByPath))
	for _, slot := range c.disksByPath {
		if d := c.controllerSlots[slot]; d != nil {
			configs = append(configs, d.Config())
		}
	}

	return configs
}

// HCSAttachments returns every attached disk described in HCS schema form, ready
// to be handed to HCS when constructing or resuming a VM.
func (c *Controller) HCSAttachments() map[string]hcsschema.Scsi {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Group attachments by their controller GUID.
	out := map[string]hcsschema.Scsi{}
	for idx, d := range c.controllerSlots {
		if d == nil {
			continue
		}

		// Map the slot index to its controller GUID.
		ctrlIdx := idx / numLUNsPerController
		gid := guestrequest.ScsiControllerGuids[ctrlIdx]
		s, ok := out[gid]
		if !ok {
			s = hcsschema.Scsi{Attachments: map[string]hcsschema.Attachment{}}
		}

		// Record the disk at its LUN within the controller.
		s.Attachments[strconv.FormatUint(uint64(idx%numLUNsPerController), 10)] = hcsschema.Attachment{
			Path:                      d.Config().HostPath,
			Type_:                     string(d.Config().Type),
			ReadOnly:                  d.Config().ReadOnly,
			ExtensibleVirtualDiskType: d.Config().EVDType,
		}
		out[gid] = s
	}

	return out
}

// UpdateDiskHostPath points the disk backing the given reservation at a new
// host path. It is only valid between [Import] and [Controller.Resume], so the
// destination's disk locations can be corrected before the VM resumes.
func (c *Controller) UpdateDiskHostPath(ctx context.Context, reservationID guid.GUID, newPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isMigrating {
		return fmt.Errorf("UpdateDiskHostPath is only valid while migrating")
	}

	// Find the reservation.
	r, ok := c.reservations[reservationID]
	if !ok {
		return fmt.Errorf("reservation %s not found", reservationID)
	}

	// Find the requested disk.
	d := c.controllerSlots[r.controllerSlot]
	if d == nil {
		return fmt.Errorf("disk for reservation %s not found", reservationID)
	}

	// Update old path to new path.
	oldPath := d.HostPath()
	if oldPath == newPath {
		return nil
	}
	if slot, ok := c.disksByPath[oldPath]; ok && slot == r.controllerSlot {
		delete(c.disksByPath, oldPath)
	}

	d.UpdateHostPath(newPath)
	c.disksByPath[newPath] = r.controllerSlot

	log.G(ctx).WithFields(logrus.Fields{
		"OldPath": oldPath,
		"NewPath": newPath,
	}).Debug("updated disk host path")

	return nil
}
