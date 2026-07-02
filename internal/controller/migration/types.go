//go:build windows && lcow

package migration

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi"
	"github.com/Microsoft/hcsshim/internal/controller/device/vpci"
	"github.com/Microsoft/hcsshim/internal/controller/network"
	"github.com/Microsoft/hcsshim/internal/controller/pod"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	hcs "github.com/Microsoft/hcsshim/internal/hcs/v2"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"

	"google.golang.org/protobuf/types/known/anypb"
)

// vmController is the subset of the VM controller that the migration controller
// drives. It is declared as an interface so tests can substitute a mock; it is
// implemented by [vm.Controller].
type vmController interface {
	State() vm.State
	SandboxOptions() *lcow.SandboxOptions
	InitializeLiveMigrationOnSource(ctx context.Context, options *hcsschema.MigrationInitializeOptions) error
	Save(ctx context.Context) (*anypb.Any, error)
	Import(ctx context.Context, env *anypb.Any) error
	CreateVM(ctx context.Context, opts *vm.CreateOptions) error
	Patch(ctx context.Context) error
	StartLiveMigrationOnSource(ctx context.Context, config *hcs.MigrationConfig) error
	StartWithMigrationOptions(ctx context.Context, config *hcs.MigrationConfig) error
	StartLiveMigrationTransfer(ctx context.Context, options *hcsschema.MigrationTransferOptions) error
	FinalizeLiveMigration(ctx context.Context, options *hcsschema.MigrationFinalizedOptions) error
	Resume(ctx context.Context, rebuildBridge bool) error
	MigrationNotifications() (<-chan hcsschema.OperationSystemMigrationNotificationInfo, error)

	RuntimeID() string
	VM() *vmmanager.UtilityVM
	Guest() *guestmanager.Guest
	SCSIController(ctx context.Context) (*scsi.Controller, error)
	VPCIController() *vpci.Controller
	Plan9Controller() *plan9.Controller
	NetworkController(networkNamespaceID string) *network.Controller
}

// InitOptions carries the fields common to every migration entry point that
// binds a controller to a session.
type InitOptions struct {
	// SessionID correlates all calls belonging to a single migration session.
	SessionID string

	// Origin selects whether this side acts as the migration source or destination.
	Origin hcsschema.MigrationOrigin

	// VMController is the VM driven by the session: saved on the source, rehydrated
	// on the destination. Owned by the service; the controller only borrows it.
	VMController vmController

	// PodControllers are the sandbox's pods, keyed by pod ID, migrated alongside the
	// VM. Owned by the service; the controller only borrows the map.
	PodControllers map[string]*pod.Controller
}

// PrepareSourceOptions configures the source side of a migration session.
type PrepareSourceOptions struct {
	InitOptions

	// MigrationOpts tunes the HCS migration workflow; optional, defaulted when nil.
	MigrationOpts *hcsschema.MigrationInitializeOptions
}

// ImportStateOptions configures rehydrating a source snapshot on the destination side.
type ImportStateOptions struct {
	InitOptions

	// SandboxID is the destination sandbox ID the snapshot is imported into.
	SandboxID string

	// SavedState is the opaque snapshot produced by the source's ExportState.
	SavedState *anypb.Any

	// ContainerPodMapping is the service-owned containerID -> podID index.
	// ImportState populates it and PatchResourcePaths renames its entries
	// in place; the service continues to own its lifetime.
	ContainerPodMapping map[string]string
}
