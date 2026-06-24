//go:build windows && lcow

package pod

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/controller/device/plan9"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi"
	"github.com/Microsoft/hcsshim/internal/controller/device/vpci"
	"github.com/Microsoft/hcsshim/internal/controller/network"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"

	"google.golang.org/protobuf/types/known/anypb"
)

// vmController exposes the subset of the VM manager that the pod controller
// needs: identity, guest access, device controllers, and the network controller.
// Implemented by the VM controller.
type vmController interface {
	// RuntimeID returns the unique runtime identifier for the VM.
	RuntimeID() string

	// VM returns the vm manager used for UVM host side operations.
	VM() *vmmanager.UtilityVM

	// Guest returns the guest manager used for guest-side operations.
	Guest() *guestmanager.Guest

	// SCSIController returns the SCSI device controller for the VM.
	SCSIController(ctx context.Context) (*scsi.Controller, error)

	// VPCIController returns the vPCI device controller for the VM.
	VPCIController() *vpci.Controller

	// Plan9Controller returns the Plan9 share controller for the VM.
	Plan9Controller() *plan9.Controller

	// NetworkController returns the network controller for the VM.
	NetworkController(networkNamespaceID string) *network.Controller
}

// networkController is the narrow interface used by the pod to set up and
// tear down the network namespace. Implemented by [network.Controller].
type networkController interface {
	// Setup performs network setup for the pod.
	Setup(ctx context.Context) error

	// Teardown performs network teardown for the pod.
	Teardown(ctx context.Context) error

	// Save returns the network controller's migration payload as an
	// [anypb.Any] envelope owned by the network controller package.
	Save(ctx context.Context) (*anypb.Any, error)

	// Patch supplies the destination host's network namespace ID, which is used
	// later to attach endpoints when migration completes.
	Patch(ctx context.Context, networkNamespaceID string)

	// Resume binds live host/guest dependencies during destination-side
	// migration rehydration.
	Resume(ctx context.Context, vm *vmmanager.UtilityVM, guest *guestmanager.Guest)

	// ResetAfterMigration detaches the stale source endpoints and wires up the
	// destination namespace's endpoints in the guest.
	ResetAfterMigration(ctx context.Context) error
}
