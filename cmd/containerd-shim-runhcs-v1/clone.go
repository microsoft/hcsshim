package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/regstate"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

const (
	configRoot = "croot"
	configKey  = "ckey"
)

type PersistedUVMConfig struct {
	// actual information related to template / clone
	RawData []byte
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

// When encoding interfaces gob requires us to register the struct types that we will be using under those
// interfaces. This registration needs to happen on both sides i.e the side which encodes the data and the
// side which decodes the data.
func gobInit() {
	// Register the pointer to structs because that is what is being stored.
	gob.Register(&uvm.VSMBShare{})
	gob.Register(&uvm.SCSIMount{})
}

func encodeTemplateConfig(utc *uvm.UVMTemplateConfig) ([]byte, error) {
	var buf bytes.Buffer

	gobInit()
	encoder := gob.NewEncoder(&buf)
	err := encoder.Encode(utc)
	if err != nil {
		return nil, fmt.Errorf("Error while encoding template config: %s", err)
	}
	return buf.Bytes(), nil
}

func decodeTemplateConfig(encodedBytes []byte) (*uvm.UVMTemplateConfig, error) {
	var utc uvm.UVMTemplateConfig

	gobInit()
	reader := bytes.NewReader(encodedBytes)
	decoder := gob.NewDecoder(reader)
	err := decoder.Decode(&utc)
	if err != nil {
		return nil, fmt.Errorf("Error while decoding template config: %s", err)
	}
	return &utc, nil
}

// Saves all the information required to create a clone from the template
// of this container into the registry.
func SaveTemplateConfig(ctx context.Context, hostUVM *uvm.UtilityVM) error {
	_, err := loadPersistedUVMConfig(hostUVM.ID())
	if !regstate.IsNotFoundError(err) {
		return fmt.Errorf("Parent VM(ID: %s) config shouldn't exit in registry (%s) \n", hostUVM.ID(), err.Error())
	}

	utc := hostUVM.GenerateTemplateConfig()

	encodedBytes, err := encodeTemplateConfig(utc)
	if err != nil {
		return err
	}

	puc := &PersistedUVMConfig{
		RawData: encodedBytes,
		Stored:  false,
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

	utc, err := decodeTemplateConfig(puc.RawData)
	if err != nil {
		return nil, err
	}
	return utc, nil
}

// SaveAsTemplate saves the host as a template. It is assumed that SaveTemplateConfig is
// called on this host before calling SaveAsTemplate. SaveTemplateConfig saves the important
// information that is required to create copies from this template. SaveAsTemplate actually
// pauses this VM and saves it.
// Saving is done in following steps:
// - First remove namespaces associated with the host.
// - Close the GCS connection.
// - Save the information about the templtae that will be needed during cloning
// - Save the host as a template.
func SaveAsTemplate(ctx context.Context, host *uvm.UtilityVM) (err error) {
	if err = host.RemoveAllNICs(ctx); err != nil {
		return err
	}

	if err = host.CloseGCConnection(); err != nil {
		return err
	}

	if err = SaveTemplateConfig(ctx, host); err != nil {
		return err
	}

	if err = host.SaveAsTemplate(ctx); err != nil {
		return err
	}
	return nil
}
