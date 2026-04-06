//go:build windows

package vmmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/Microsoft/go-winio/pkg/guid"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// ID returns the ID of the utility VM.
func (uvm *UtilityVM) ID() string {
	return uvm.id
}

// RuntimeID returns the runtime ID of the utility VM.
func (uvm *UtilityVM) RuntimeID() guid.GUID {
	return uvm.vmID
}

// Start starts the utility VM.
func (uvm *UtilityVM) Start(ctx context.Context) (err error) {
	if err := uvm.cs.Start(ctx); err != nil {
		return fmt.Errorf("failed to start utility VM: %w", err)
	}
	return nil
}

// Terminate terminates the utility VM and waits for it to exit.
func (uvm *UtilityVM) Terminate(ctx context.Context) error {
	if err := uvm.cs.Terminate(ctx); err != nil {
		return fmt.Errorf("failed to terminate utility VM: %w", err)
	}
	if err := uvm.Wait(ctx); err != nil {
		return fmt.Errorf("failed to wait for utility VM termination: %w", err)
	}
	return nil
}

// Close closes the utility VM and releases all associated resources.
func (uvm *UtilityVM) Close(ctx context.Context) error {
	if err := uvm.cs.CloseCtx(ctx); err != nil {
		return fmt.Errorf("failed to close utility VM: %w", err)
	}
	return nil
}

// Pause pauses the utility VM.
func (uvm *UtilityVM) Pause(ctx context.Context) error {
	if err := uvm.cs.Pause(ctx); err != nil {
		return fmt.Errorf("failed to pause utility VM: %w", err)
	}
	return nil
}

// Resume resumes the utility VM.
func (uvm *UtilityVM) Resume(ctx context.Context) error {
	if err := uvm.cs.Resume(ctx); err != nil {
		return fmt.Errorf("failed to resume utility VM: %w", err)
	}
	return nil
}

// Save saves the state of the utility VM as a template.
func (uvm *UtilityVM) Save(ctx context.Context, options hcsschema.SaveOptions) error {
	if err := uvm.cs.Save(ctx, options); err != nil {
		return fmt.Errorf("failed to save utility VM state: %w", err)
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
		return nil, fmt.Errorf("failed to get properties from HCS: %w", err)
	}

	return props, nil
}

// StartedTime returns the time when the utility VM entered the running state.
func (uvm *UtilityVM) StartedTime() time.Time {
	return uvm.cs.StartedTime()
}

// StoppedTime returns the time when the utility VM entered the stopped state.
func (uvm *UtilityVM) StoppedTime() time.Time {
	return uvm.cs.StoppedTime()
}

// ExitError returns the exit error of the utility VM, if it has exited.
func (uvm *UtilityVM) ExitError() error {
	return uvm.cs.ExitError()
}
