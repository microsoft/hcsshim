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

// Manager is the concrete implementation of [Controller].
type Manager struct {
	mu sync.Mutex

	// shares is the set of currently configured Plan9 share names.
	// Guarded by mu.
	shares map[string]struct{}

	// noWritableFileShares disallows adding writable Plan9 shares.
	noWritableFileShares bool

	// vmPlan9Mgr performs host-side Plan9 add/remove on the VM.
	vmPlan9Mgr vmPlan9Manager

	// nameCounter is the monotonically increasing index used to
	// generate unique share names. Guarded by mu.
	nameCounter uint64
}

var _ Controller = (*Manager)(nil)

// New creates a ready-to-use [Manager].
func New(
	vmPlan9Mgr vmPlan9Manager,
	noWritableFileShares bool,
) *Manager {
	return &Manager{
		vmPlan9Mgr:           vmPlan9Mgr,
		noWritableFileShares: noWritableFileShares,
		shares:               make(map[string]struct{}),
	}
}

// AddToVM adds a Plan9 share to the host VM and returns the generated share name.
func (m *Manager) AddToVM(ctx context.Context, opts *AddOptions) (_ string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure that adding the share is allowed.
	if !opts.ReadOnly && m.noWritableFileShares {
		return "", fmt.Errorf("adding writable shares is denied: %w", hcs.ErrOperationDenied)
	}

	// Build the Plan9 share flags bitmask from the caller-provided options.
	flags := shareFlagsLinuxMetadata
	if opts.ReadOnly {
		flags |= shareFlagsReadOnly
	}
	if opts.Restrict {
		flags |= shareFlagsRestrictFileAccess
	}

	// Generate a unique share name from the nameCounter.
	name := strconv.FormatUint(m.nameCounter, 10)
	m.nameCounter++

	ctx, _ = log.WithContext(ctx, logrus.WithField("shareName", name))

	log.G(ctx).WithFields(logrus.Fields{
		logfields.HostPath:   opts.HostPath,
		logfields.ReadOnly:   opts.ReadOnly,
		"RestrictFileAccess": opts.Restrict,
		"AllowedFiles":       opts.AllowedNames,
	}).Tracef("adding plan9 share to host VM")

	// Call into HCS to add the Plan9 share to the VM.
	if err := m.vmPlan9Mgr.AddPlan9(ctx, hcsschema.Plan9Share{
		Name:         name,
		AccessName:   name,
		Path:         opts.HostPath,
		Port:         vmutils.Plan9Port,
		Flags:        flags,
		AllowedFiles: opts.AllowedNames,
	}); err != nil {
		return "", fmt.Errorf("add plan9 share %s to host: %w", name, err)
	}

	m.shares[name] = struct{}{}

	log.G(ctx).Info("plan9 share added to host VM")

	return name, nil
}

// RemoveFromVM removes the Plan9 share identified by shareName from the host VM.
func (m *Manager) RemoveFromVM(ctx context.Context, shareName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx, _ = log.WithContext(ctx, logrus.WithField("shareName", shareName))

	if _, ok := m.shares[shareName]; !ok {
		log.G(ctx).Debug("plan9 share not found, skipping removal")
		return nil
	}

	log.G(ctx).Debug("starting plan9 share removal")

	// Call into HCS to remove the share from the VM.
	if err := m.vmPlan9Mgr.RemovePlan9(ctx, hcsschema.Plan9Share{
		Name:       shareName,
		AccessName: shareName,
		Port:       vmutils.Plan9Port,
	}); err != nil {
		return fmt.Errorf("remove plan9 share %s from host: %w", shareName, err)
	}

	delete(m.shares, shareName)

	log.G(ctx).Info("plan9 share removed from host VM")

	return nil
}
