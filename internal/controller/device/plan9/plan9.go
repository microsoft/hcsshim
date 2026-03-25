//go:build windows && !wcow

package plan9

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"

	"github.com/sirupsen/logrus"
)

// share-flag constants used in Plan9 HCS requests.
//
// These are marked private in the HCS schema. When public variants become
// available, we should replace these.
const (
	shareFlagsReadOnly           int32 = 0x00000001
	shareFlagsLinuxMetadata      int32 = 0x00000004
	shareFlagsRestrictFileAccess int32 = 0x00000080
)

// Manager is the concrete implementation which manages plan9 shares to the UVM.
type Manager struct {
	// mu protects the shares map and serializes name allocation across concurrent callers.
	mu sync.Mutex

	// shares maps share name → shareEntry for every active or pending share.
	// Access must be guarded by mu.
	shares map[string]*shareEntry

	// noWritableFileShares disallows adding writable Plan9 shares.
	noWritableFileShares bool

	// vmPlan9Mgr performs host-side Plan9 add/remove on the VM.
	vmPlan9Mgr vmPlan9Manager

	// nameCounter is the monotonically increasing index used to generate unique share names.
	// Access must be guarded by mu.
	nameCounter uint64
}

// New creates a ready-to-use [Manager].
func New(
	vmPlan9Mgr vmPlan9Manager,
	noWritableFileShares bool,
) *Manager {
	return &Manager{
		vmPlan9Mgr:           vmPlan9Mgr,
		noWritableFileShares: noWritableFileShares,
		shares:               make(map[string]*shareEntry),
	}
}

// ResolveShareName pre-emptively allocates a share name for the given [AddOptions] and returns it.
// If a matching share is already tracked, the existing name is returned without
// allocating a new entry. ResolveShareName does not drive any HCS call or increment the
// reference count; callers must follow up with [Manager.AddToVM] to claim the share.
func (m *Manager) ResolveShareName(ctx context.Context, opts *AddOptions) (string, error) {
	if !opts.ReadOnly && m.noWritableFileShares {
		return "", fmt.Errorf("adding writable shares is denied: %w", hcs.ErrOperationDenied)
	}

	log.G(ctx).WithField(logfields.HostPath, opts.HostPath).Debug("resolving plan9 share name")

	entry := m.getOrAllocateEntry(ctx, opts)
	return entry.name, nil
}

// AddToVM adds a Plan9 share to the host VM and returns the generated share name.
// If a share with identical [AddOptions] is already added or in flight, AddToVM
// blocks until that operation completes and returns the share name, incrementing
// the internal reference count.
func (m *Manager) AddToVM(ctx context.Context, opts *AddOptions) (_ string, err error) {
	// Validate write-share policy before touching shared state.
	if !opts.ReadOnly && m.noWritableFileShares {
		return "", fmt.Errorf("adding writable shares is denied: %w", hcs.ErrOperationDenied)
	}

	entry := m.getOrAllocateEntry(ctx, opts)

	// Acquire the per-entry lock to check state and potentially drive the HCS call.
	// Multiple goroutines requesting the same share will serialize here.
	entry.mu.Lock()
	defer entry.mu.Unlock()

	ctx, _ = log.WithContext(ctx, logrus.WithField("shareName", entry.name))

	log.G(ctx).Debug("received share entry, checking state")

	switch entry.state {
	case shareAdded:
		// ==============================================================================
		// Found an existing live share — reuse it.
		// ==============================================================================
		entry.refCount++
		log.G(ctx).Debug("plan9 share already added to VM, reusing existing share")
		return entry.name, nil

	case sharePending:
		// ==============================================================================
		// New share — we own the HCS call.
		// Other callers requesting the same share will block on entry.mu until we
		// transition the state out of sharePending.
		// ==============================================================================
		flags := shareFlagsLinuxMetadata
		if opts.ReadOnly {
			flags |= shareFlagsReadOnly
		}
		if opts.Restrict {
			flags |= shareFlagsRestrictFileAccess
		}

		log.G(ctx).WithFields(logrus.Fields{
			logfields.HostPath:   opts.HostPath,
			logfields.ReadOnly:   opts.ReadOnly,
			"RestrictFileAccess": opts.Restrict,
			"AllowedFiles":       opts.AllowedNames,
		}).Trace("adding plan9 share to host VM")

		if err = m.vmPlan9Mgr.AddPlan9(ctx, hcsschema.Plan9Share{
			Name:         entry.name,
			AccessName:   entry.name,
			Path:         opts.HostPath,
			Port:         vmutils.Plan9Port,
			Flags:        flags,
			AllowedFiles: opts.AllowedNames,
		}); err != nil {
			// Transition to Invalid so that waiting goroutines see the real failure reason.
			entry.state = shareInvalid
			entry.stateErr = err

			// Remove from the map so subsequent calls can retry with a fresh entry.
			m.mu.Lock()
			delete(m.shares, entry.name)
			m.mu.Unlock()

			return "", fmt.Errorf("add plan9 share %s to host: %w", entry.name, err)
		}

		entry.state = shareAdded
		entry.refCount++

		log.G(ctx).Info("plan9 share added to host VM")

		return entry.name, nil

	case shareInvalid:
		// ==============================================================================
		// A previous AddPlan9 call for this entry failed.
		// ==============================================================================
		// Return the original error. The map entry has already been removed
		// by the goroutine that drove the failed add.
		return "", fmt.Errorf("previous attempt to add plan9 share %s to VM failed: %w",
			entry.name, entry.stateErr)

	default:
		return "", fmt.Errorf("plan9 share in unexpected state %s during add", entry.state)
	}
}

// getOrAllocateEntry either reuses an existing [shareEntry] whose options match opts,
// or allocates a new pending entry with a freshly generated name.
// The returned entry's refCount is not incremented; callers that claim the share
// must increment it themselves.
func (m *Manager) getOrAllocateEntry(ctx context.Context, opts *AddOptions) *shareEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Reuse an existing entry if its options match the caller's.
	for _, existing := range m.shares {
		if optionsMatch(existing.opts, opts) {
			return existing
		}
	}

	log.G(ctx).Debug("no existing plan9 share found for options, allocating new entry")

	name := strconv.FormatUint(m.nameCounter, 10)
	m.nameCounter++

	entry := &shareEntry{
		opts:  opts,
		name:  name,
		state: sharePending,
		// refCount is 0; it will be incremented by the goroutine that drives AddPlan9.
		refCount: 0,
	}
	m.shares[name] = entry
	return entry
}

// RemoveFromVM removes the Plan9 share identified by shareName from the host VM.
// If the share is held by multiple callers, RemoveFromVM decrements the reference
// count and returns without tearing down the share until the last caller removes it.
func (m *Manager) RemoveFromVM(ctx context.Context, shareName string) error {
	ctx, _ = log.WithContext(ctx, logrus.WithField("shareName", shareName))

	m.mu.Lock()
	entry := m.shares[shareName]
	m.mu.Unlock()

	if entry == nil {
		log.G(ctx).Debug("plan9 share not found, skipping removal")
		return nil
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.state == shareInvalid {
		// AddPlan9 never succeeded; nothing to remove from HCS.
		return nil
	}

	if entry.refCount > 1 {
		entry.refCount--
		log.G(ctx).Debug("plan9 share still in use by other callers, not removing from VM")
		return nil
	}

	// refCount is 0 (pre-allocated via ResolveShareName but never added) or 1 (last caller).
	// Only call RemovePlan9 when the share was actually added to the VM.
	if entry.state == shareAdded {
		log.G(ctx).Debug("starting plan9 share removal")

		if err := m.vmPlan9Mgr.RemovePlan9(ctx, hcsschema.Plan9Share{
			Name:       shareName,
			AccessName: shareName,
			Port:       vmutils.Plan9Port,
		}); err != nil {
			return fmt.Errorf("remove plan9 share %s from host: %w", shareName, err)
		}

		entry.state = shareRemoved
		log.G(ctx).Info("plan9 share removed from host VM")
	}

	// Clean up from the map regardless of whether AddPlan9 was ever called.
	m.mu.Lock()
	delete(m.shares, shareName)
	m.mu.Unlock()

	return nil
}
