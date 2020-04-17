package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/regstate"
	"github.com/Microsoft/hcsshim/internal/uvm"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

const (
	configRoot = "croot"
	configKey  = "ckey"
)

type PersistedUVMConfig struct {
	// actual information related to template / clone
	TemplateConfig uvm.UVMTemplateConfig
	// metadata field used to determine if this config is already started.
	Stored bool
}

// loadPersistedConfig loads a persisted config from the registry that matches the given ID
// If not found returns `regstate.NotFoundError`
func loadPersistedUVMConfig(ID string) (*PersistedUVMConfig, error) {
	sk, err := regstate.Open(configRoot, false)
	if err != nil {
		return nil, err
	}
	defer sk.Close()

	puc := PersistedUVMConfig{
		Stored: true,
	}
	if err := sk.Get(ID, configKey, &puc); err != nil {
		return nil, err
	}
	return &puc, nil
}

// storePersistedUVMConfig stores or updates the in-memory config to its registry state.
// If the store fails returns the store error.
func storePersistedUVMConfig(ID string, puc *PersistedUVMConfig) error {
	sk, err := regstate.Open(configRoot, false)
	if err != nil {
		return err
	}
	defer sk.Close()

	if puc.Stored {
		if err := sk.Set(ID, configKey, puc); err != nil {
			return err
		}
	} else {
		if err := sk.Create(ID, configKey, puc); err != nil {
			return err
		}
	}
	puc.Stored = true
	return nil
}

// Remove removes any persisted state associated with this config. If the config
// is not found in the registery `Remove` returns no error.
func removePersistedUVMConfig(ID string) error {
	sk, err := regstate.Open(configRoot, false)
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

// Saves all the information required to create a clone from the template
// of this container into the registry.
func SaveTemplateConfig(ctx context.Context, hostUVM *uvm.UtilityVM, spec *specs.Spec) error {
	_, err := loadPersistedUVMConfig(hostUVM.ID())
	if !regstate.IsNotFoundError(err) {
		return fmt.Errorf("Parent VM(ID: %s) config shouldn't exit in registry (%s) \n", hostUVM.ID(), err.Error())
	}

	vhdPath, err := hostUVM.GetScratchVHDPath()
	if err != nil {
		return err
	}

	utc := uvm.UVMTemplateConfig{
		UVMID:      hostUVM.ID(),
		UVMVhdPath: vhdPath,
		VSMBShares: []uvm.TemplateVSMBConfig{},
		SCSIMounts: []uvm.TemplateSCSIMountConfig{},
	}

	// save layer information as VSMB mounts
	for _, layer := range spec.Windows.LayerFolders[:len(spec.Windows.LayerFolders)-1] {
		name, err := hostUVM.GetVSMBShareName(ctx, layer)
		if err != nil {
			return err
		}
		utc.VSMBShares = append(utc.VSMBShares, uvm.TemplateVSMBConfig{
			ShareName: name,
			HostPath:  layer,
		})
	}

	scratchFolder := spec.Windows.LayerFolders[len(spec.Windows.LayerFolders)-1]
	scratchVhd := filepath.Join(scratchFolder, "sandbox.vhdx")
	svController, svLUN, err := hostUVM.GetScsiLocationInfo(ctx, scratchVhd)
	if err != nil {
		return err
	}
	utc.SCSIMounts = append(utc.SCSIMounts, uvm.TemplateSCSIMountConfig{
		Controller: svController,
		LUN:        svLUN,
		HostPath:   scratchVhd,
		IsScratch:  true,
		// TODO(ambarve): Get correct type information here
		Type: "VirtualDisk",
	})

	// TODO(ambarve): other mounts specified in the container config should also be
	// captured here. For now, we won't allow such mounts for templates.
	puc := &PersistedUVMConfig{
		TemplateConfig: utc,
		Stored:         false,
	}

	if err := storePersistedUVMConfig(hostUVM.ID(), puc); err != nil {
		return err
	}

	return nil
}

// Removes all the state associated with the template with given ID
// If there is no state associated with this ID then the function simply returns without
// doing anything.
func RemoveSavedTemplateConfig(ID string) error {
	if err := removePersistedUVMConfig(ID); err != nil {
		return err
	}
	return nil
}

func FetchTemplateConfig(ctx context.Context, ID string) (*uvm.UVMTemplateConfig, error) {
	puc, err := loadPersistedUVMConfig(ID)
	if err != nil {
		return nil, err
	}
	return &puc.TemplateConfig, nil
}

// SaveAsTemplate saves the host as a template. It is assumed that SaveTemplateConfig is
// called on this host before calling SaveAsTemplate. SaveTemplateConfig saves the important
// information that is required to create copies from this template. SaveAsTemplate actually
// pauses this VM and saves it.
// Saving is done in following 3 steps:
// 1. First remove namespaces associated with the host.
// 2. Close the GCS connection.
// 3. Save the host as a template.
func SaveAsTemplate(ctx context.Context, host *uvm.UtilityVM) (err error) {
	if err = host.RemoveAllNamespaces(ctx); err != nil {
		return err
	}

	if err = host.CloseGCConnection(); err != nil {
		return err
	}

	if err = host.SaveAsTemplate(ctx); err != nil {
		return err
	}
	return nil
}
