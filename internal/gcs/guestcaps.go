//go:build windows

package gcs

import (
	"encoding/json"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
)

// GuestDefinedCapabilities is an interface for different guest defined capabilities.
// This allows us to define different capabilities by OS.
//
// When adding new fields to the implementations of type GuestDefinedCapabilities, a
// new interface function should only be added if that capability describes a feature
// available for both WCOW and LCOW. Otherwise, you can use the helper functions
// GetWCOWCapabilities or GetLCOWCapabilities to check for capabilities specific to
// one implementation.
type GuestDefinedCapabilities interface {
	IsSignalProcessSupported() bool
	IsDeleteContainerStateSupported() bool
	IsDumpStacksSupported() bool
	IsNamespaceAddRequestSupported() bool
}

var _ GuestDefinedCapabilities = &schema1.GuestDefinedCapabilities{}
var _ GuestDefinedCapabilities = &prot.GcsGuestCapabilities{}

func GetWCOWCapabilities(gdc GuestDefinedCapabilities) *schema1.GuestDefinedCapabilities {
	g, ok := gdc.(*schema1.GuestDefinedCapabilities)
	if !ok {
		return nil
	}
	return g
}

func GetLCOWCapabilities(gdc GuestDefinedCapabilities) *prot.GcsGuestCapabilities {
	g, ok := gdc.(*prot.GcsGuestCapabilities)
	if !ok {
		return nil
	}
	return g
}

func unmarshalGuestCapabilities(os string, data json.RawMessage) (GuestDefinedCapabilities, error) {
	if os == "windows" {
		gdc := &schema1.GuestDefinedCapabilities{}
		if err := json.Unmarshal(data, gdc); err != nil {
			return nil, fmt.Errorf("unmarshal returned GuestDefinedCapabilities for windows: %w", err)
		}
		return gdc, nil
	}
	// linux
	gdc := &prot.GcsGuestCapabilities{}
	if err := json.Unmarshal(data, gdc); err != nil {
		return nil, fmt.Errorf("unmarshal returned GuestDefinedCapabilities for lcow: %w", err)
	}
	return gdc, nil
}
