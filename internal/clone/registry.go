//go:build windows

package clone

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/regstate"
	"github.com/Microsoft/hcsshim/internal/uvm"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

const (
	configRoot                           = "LateClone"
	configKey                            = "UVMConfig"
	templateConfigCurrentSerialVersionID = 1
)

// TemplateConfig struct maintains all of the information about a template.  This includes
// the information for both the template container and the template UVM.  This struct is
// serialized and stored in the registry and hence is version controlled.
// Note: Update the `templateConfigCurrentSerialVersionID` when this structure definition
// is changed.
type TemplateConfig struct {
	SerialVersionID       uint32
	TemplateUVMID         string
	TemplateUVMResources  []uvm.Cloneable
	TemplateUVMCreateOpts uvm.OptionsWCOW
	TemplateContainerID   string
	// Below we store the container spec for the template container so that when
	// cloning containers we can verify that a different spec is not provided for the
	// cloned container.
	TemplateContainerSpec specs.Spec
}

// When encoding interfaces gob requires us to register the struct types that we will be
// using under those interfaces. This registration needs to happen on both sides i.e the
// side which encodes the data (i.e the shim process of the template) and the side which
// decodes the data (i.e the shim process of the clone).
// Go init function: https://golang.org/doc/effective_go.html#init
func init() {
	// Register the pointer to structs because that is what is being stored.
	gob.Register(&uvm.VSMBShare{})
	gob.Register(&uvm.SCSIAttachment{})
}

func encodeTemplateConfig(templateConfig *TemplateConfig) ([]byte, error) {
	var buf bytes.Buffer

	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(templateConfig); err != nil {
		return nil, fmt.Errorf("error while encoding template config: %s", err)
	}
	return buf.Bytes(), nil
}

func decodeTemplateConfig(encodedBytes []byte) (*TemplateConfig, error) {
	var templateConfig TemplateConfig

	reader := bytes.NewReader(encodedBytes)
	decoder := gob.NewDecoder(reader)
	if err := decoder.Decode(&templateConfig); err != nil {
		return nil, fmt.Errorf("error while decoding template config: %s", err)
	}
	return &templateConfig, nil
}

// loadPersistedUVMConfig loads a persisted config from the registry that matches the given ID
// If not found returns `regstate.NotFoundError`
func loadPersistedUVMConfig(id string) ([]byte, error) {
	sk, err := regstate.Open(configRoot, false)
	if err != nil {
		return nil, err
	}
	defer sk.Close()

	var encodedConfig []byte
	if err := sk.Get(id, configKey, &encodedConfig); err != nil {
		return nil, err
	}
	return encodedConfig, nil
}

// storePersistedUVMConfig stores the given config to the registry.
// If the store fails returns the store error.
func storePersistedUVMConfig(id string, encodedConfig []byte) error {
	sk, err := regstate.Open(configRoot, false)
	if err != nil {
		return err
	}
	defer sk.Close()

	if err := sk.Create(id, configKey, encodedConfig); err != nil {
		return err
	}
	return nil
}

// removePersistedUVMConfig removes any persisted state associated with this config. If the config
// is not found in the registry `Remove` returns no error.
func removePersistedUVMConfig(id string) error {
	sk, err := regstate.Open(configRoot, false)
	if err != nil {
		if regstate.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	defer sk.Close()

	if err := sk.Remove(id); err != nil {
		if regstate.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	return nil
}

// Saves all the information required to create a clone from the template
// of this container into the registry.
func SaveTemplateConfig(ctx context.Context, templateConfig *TemplateConfig) error {
	_, err := loadPersistedUVMConfig(templateConfig.TemplateUVMID)
	if !regstate.IsNotFoundError(err) {
		return fmt.Errorf("parent VM(ID: %s) config shouldn't exit in registry (%s)", templateConfig.TemplateUVMID, err)
	}

	// set the serial version before encoding
	templateConfig.SerialVersionID = templateConfigCurrentSerialVersionID

	encodedBytes, err := encodeTemplateConfig(templateConfig)
	if err != nil {
		return fmt.Errorf("failed to encode template config: %s", err)
	}

	if err := storePersistedUVMConfig(templateConfig.TemplateUVMID, encodedBytes); err != nil {
		return fmt.Errorf("failed to store encoded template config: %s", err)
	}

	return nil
}

// Removes all the state associated with the template with given ID
// If there is no state associated with this ID then the function simply returns without
// doing anything.
func RemoveSavedTemplateConfig(id string) error {
	return removePersistedUVMConfig(id)
}

// Retrieves the UVMTemplateConfig for the template with given ID from the registry.
func FetchTemplateConfig(ctx context.Context, id string) (*TemplateConfig, error) {
	encodedBytes, err := loadPersistedUVMConfig(id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch encoded template config: %s", err)
	}

	templateConfig, err := decodeTemplateConfig(encodedBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode template config: %s", err)
	}

	if templateConfig.SerialVersionID != templateConfigCurrentSerialVersionID {
		return nil, fmt.Errorf("serialized version of TemplateConfig: %d doesn't match with the current version: %d", templateConfig.SerialVersionID, templateConfigCurrentSerialVersionID)
	}

	return templateConfig, nil
}
