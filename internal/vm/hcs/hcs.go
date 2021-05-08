package hcs

import (
	"context"
	"sync"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

type utilityVM struct {
	id           string
	state        vm.State
	guestOS      vm.GuestOS
	cs           *hcs.System
	backingType  vm.MemoryBackingType
	vmmemProcess windows.Handle
	vmmemErr     error
	vmmemOnce    sync.Once
	vmID         guid.GUID
}

func (uvm *utilityVM) ID() string {
	return uvm.id
}

func (uvm *utilityVM) Start(ctx context.Context) (err error) {
	if err := uvm.cs.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start utility VM")
	}
	return nil
}

func (uvm *utilityVM) Stop(ctx context.Context) error {
	if err := uvm.cs.Terminate(ctx); err != nil {
		return errors.Wrap(err, "failed to terminate utility VM")
	}
	return nil
}

func (uvm *utilityVM) Pause(ctx context.Context) error {
	if err := uvm.cs.Pause(ctx); err != nil {
		return errors.Wrap(err, "failed to pause utility VM")
	}
	return nil
}

func (uvm *utilityVM) Resume(ctx context.Context) error {
	if err := uvm.cs.Resume(ctx); err != nil {
		return errors.Wrap(err, "failed to resume utility VM")
	}
	return nil
}

func (uvm *utilityVM) Save(ctx context.Context) error {
	saveOptions := hcsschema.SaveOptions{
		SaveType: "AsTemplate",
	}
	if err := uvm.cs.Save(ctx, saveOptions); err != nil {
		return errors.Wrap(err, "failed to save utility VM state")
	}
	return nil
}

func (uvm *utilityVM) Wait() error {
	return uvm.cs.Wait()
}

func (uvm *utilityVM) ExitError() error {
	return uvm.cs.ExitError()
}
