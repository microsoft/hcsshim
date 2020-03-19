package uvm

import (
	"context"

	"github.com/Microsoft/go-winio/pkg/security"
	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/regstate"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
)

var err error

const (
	hcsSaveOptions = "{\"SaveType\": \"AsTemplate\"}"
	templateRoot   = "troot"
	templateKey    = "tkey"
)

type PersistedUVMConfig struct {
	ID     string
	Stored bool
	Config hcsschema.ComputeSystem
}

func NewPersistedUVMConfig(ID string, config hcsschema.ComputeSystem) *PersistedUVMConfig {
	return &PersistedUVMConfig{
		ID:     ID,
		Stored: false,
		Config: config,
	}
}

// LoadTemplateConfig loads a persisted template config from the registry that matches
// `templateID`. If not found returns `regstate.NotFoundError`
func LoadPersistedUVMConfig(ID string) (*PersistedUVMConfig, error) {
	sk, err := regstate.Open(templateRoot, false)
	if err != nil {
		return nil, err
	}
	defer sk.Close()

	var puc PersistedUVMConfig
	if err := sk.Get(ID, templateKey, &puc); err != nil {
		return nil, err
	}
	return &puc, nil
}

// Store stores or updates the in-memory config to its registry state. If the
// store fails returns the store error.
func StorePersistedUVMConfig(puc *PersistedUVMConfig) error {
	sk, err := regstate.Open(templateRoot, false)
	if err != nil {
		return err
	}
	defer sk.Close()

	if puc.Stored {
		if err := sk.Set(puc.ID, templateKey, puc); err != nil {
			return err
		}
	} else {
		if err := sk.Create(puc.ID, templateKey, puc); err != nil {
			return err
		}
	}
	puc.Stored = true
	return nil
}

// TODO(ambarve): Hook this up with the pod removal functions.
// Remove removes any persisted state associated with this config. If the config
// is not found in the registery `Remove` returns no error.
func RemovePersistedUVMConfig(ID string) error {
	sk, err := regstate.Open(templateRoot, false)
	if err != nil {
		if regstate.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	defer sk.Close()

	if err := sk.Remove(ID); err != nil {
		if regstate.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	return nil
}

// Store the current UVM as a template which can be later used for cloning.
// Note: Once this UVM is stored as a template it can not be resumed. It will
// permananetly stay in Saved as template state.
func (uvm *UtilityVM) SaveAsTemplate(ctx context.Context) error {
	err := uvm.hcsSystem.Pause(ctx)
	if err != nil {
		return err
	}

	err = uvm.hcsSystem.Save(ctx, hcsSaveOptions)
	if err != nil {
		return err
	}

	err = StorePersistedUVMConfig(NewPersistedUVMConfig(uvm.ID(), *uvm.configDoc))
	if err != nil {
		return err
	}
	return nil
}

// Get the config of the UVM with given ID
func getUVMConfig(ctx context.Context, uvmID string) (*hcsschema.ComputeSystem, error) {
	puc, err := LoadPersistedUVMConfig(uvmID)
	if err != nil {
		return nil, err
	}
	return &puc.Config, nil
}

func (uvm *UtilityVM) clone(ctx context.Context, doc *hcsschema.ComputeSystem, opts *OptionsWCOW) error {
	doc.VirtualMachine.RestoreState = &hcsschema.RestoreState{}
	doc.VirtualMachine.RestoreState.TemplateSystemId = opts.TemplateID

	templateConfig, err := getUVMConfig(ctx, opts.TemplateID)
	if err != nil {
		return err
	}

	srcVhdPath := templateConfig.VirtualMachine.Devices.Scsi["0"].Attachments["0"].Path
	dstVhdPath := doc.VirtualMachine.Devices.Scsi["0"].Attachments["0"].Path

	// copy the VHDX of source VM
	err = copyfile.CopyFile(ctx, srcVhdPath, dstVhdPath, true)
	if err != nil {
		return err
	}

	// Guest connection will be done externally for clones
	doc.VirtualMachine.GuestConnection = &hcsschema.GuestConnection{}

	// original VHD has VM group access but it is overwritten in the copyFile op above
	err = security.GrantVmGroupAccess(dstVhdPath)
	if err != nil {
		return err
	}

	err = uvm.create(ctx, uvm.configDoc)
	if err != nil {
		return err
	}

	return nil
}
