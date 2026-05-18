//go:build windows && lcow

package pod

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/controller/device/plan9"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi"
	"github.com/Microsoft/hcsshim/internal/controller/device/vpci"
	"github.com/Microsoft/hcsshim/internal/controller/network"
	"github.com/Microsoft/hcsshim/internal/vm/guestmanager"
)

// vmController exposes the subset of the VM manager that the pod controller
// needs: identity, guest access, device controllers, and the network controller.
// Implemented by the VM controller.
type vmController interface {
	// RuntimeID returns the unique runtime identifier for the VM.
	RuntimeID() string

	// Guest returns the guest manager used for guest-side operations.
	Guest() *guestmanager.Guest

	// SCSIController returns the SCSI device controller for the VM.
	SCSIController() *scsi.Controller

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
}
