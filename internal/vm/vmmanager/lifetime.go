//go:build windows

package vmmanager

import (
	"context"

	"github.com/Microsoft/go-winio/pkg/guid"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm"
	"github.com/pkg/errors"
)

type LifetimeManager interface {
	// ID will return a string identifier for the Utility VM.
	ID() string

	// RuntimeID will return the Hyper-V VM GUID for the Utility VM.
	//
	// Only valid after the utility VM has been created.
	RuntimeID() guid.GUID

	// OS will return the operating system type of the Utility VM. This is typically either "windows" or "linux".
	OS() vm.GuestOS

	// Start will power on the Utility VM and put it into a running state. This will boot the guest OS and start all of the
	// devices configured on the machine.
	Start(ctx context.Context) error

	// Terminate will forcefully power off the Utility VM.
	Terminate(ctx context.Context) error

	// Close terminates and releases resources associated with the utility VM.
	Close(ctx context.Context) error

	// Pause will place the Utility VM into a paused state. The guest OS will be halted and any devices will have be in a
	// a suspended state. Save can be used to snapshot the current state of the virtual machine, and Resume can be used to
	// place the virtual machine back into a running state.
	Pause(ctx context.Context) error

	// Resume will put a previously paused Utility VM back into a running state. The guest OS will resume operation from the point
	// in time it was paused and all devices should be un-suspended.
	Resume(ctx context.Context) error

	// Save will snapshot the state of the Utility VM at the point in time when the VM was paused.
	Save(ctx context.Context, options hcsschema.SaveOptions) error

	// Wait synchronously waits for the Utility VM to terminate.
	Wait(ctx context.Context) error

	// PropertiesV2 returns the properties of the Utility VM.
	PropertiesV2(ctx context.Context, types ...hcsschema.PropertyType) (*hcsschema.Properties, error)

	// ExitError will return any error if the Utility VM exited unexpectedly, or if the Utility VM experienced an
	// error after Wait returned, it will return the wait error.
	ExitError() error
}

var _ LifetimeManager = (*UtilityVM)(nil)

// ID returns the ID of the utility VM.
func (uvm *UtilityVM) ID() string {
	return uvm.id
}

// RuntimeID returns the runtime ID of the utility VM.
func (uvm *UtilityVM) RuntimeID() guid.GUID {
	return uvm.vmID
}

// OS returns the operating system of the utility VM.
func (uvm *UtilityVM) OS() vm.GuestOS {
	return uvm.guestOS
}

// Start starts the utility VM.
func (uvm *UtilityVM) Start(ctx context.Context) (err error) {
	if err := uvm.cs.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start utility VM")
	}
	return nil
}

// Terminate terminates the utility VM.
func (uvm *UtilityVM) Terminate(ctx context.Context) error {
	if err := uvm.cs.Terminate(ctx); err != nil {
		return errors.Wrap(err, "failed to terminate utility VM")
	}
	return nil
}

// Close closes the utility VM and releases all associated resources.
func (uvm *UtilityVM) Close(ctx context.Context) error {
	if err := uvm.cs.CloseCtx(ctx); err != nil {
		return errors.Wrap(err, "failed to close utility VM")
	}
	return nil
}

// Pause pauses the utility VM.
func (uvm *UtilityVM) Pause(ctx context.Context) error {
	if err := uvm.cs.Pause(ctx); err != nil {
		return errors.Wrap(err, "failed to pause utility VM")
	}
	return nil
}

// Resume resumes the utility VM.
func (uvm *UtilityVM) Resume(ctx context.Context) error {
	if err := uvm.cs.Resume(ctx); err != nil {
		return errors.Wrap(err, "failed to resume utility VM")
	}
	return nil
}

// Save saves the state of the utility VM as a template.
func (uvm *UtilityVM) Save(ctx context.Context, options hcsschema.SaveOptions) error {
	if err := uvm.cs.Save(ctx, options); err != nil {
		return errors.Wrap(err, "failed to save utility VM state")
	}
	return nil
}

// Wait waits for the utility VM to exit and returns any error that occurred during execution.
func (uvm *UtilityVM) Wait(ctx context.Context) error {
	return uvm.cs.WaitCtx(ctx)
}

// PropertiesV2 returns the properties of the utility VM from HCS.
func (uvm *UtilityVM) PropertiesV2(ctx context.Context, types ...hcsschema.PropertyType) (*hcsschema.Properties, error) {
	props, err := uvm.cs.PropertiesV2(ctx, types...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get properties from HCS")
	}

	return props, nil
}

// ExitError returns the exit error of the utility VM, if it has exited.
func (uvm *UtilityVM) ExitError() error {
	return uvm.cs.ExitError()
}
