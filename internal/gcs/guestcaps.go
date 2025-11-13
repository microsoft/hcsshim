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

func GetWCOWCapabilities(gdc GuestDefinedCapabilities) *WCOWGuestDefinedCapabilities {
	g, ok := gdc.(*WCOWGuestDefinedCapabilities)
	if !ok {
		return nil
	}
	return g
}

func GetLCOWCapabilities(gdc GuestDefinedCapabilities) *LCOWGuestDefinedCapabilities {
	g, ok := gdc.(*LCOWGuestDefinedCapabilities)
	if !ok {
		return nil
	}
	return g
}

func unmarshalGuestCapabilities(os string, data json.RawMessage) (GuestDefinedCapabilities, error) {
	if os == "windows" {
		gdc := &WCOWGuestDefinedCapabilities{}
		if err := json.Unmarshal(data, gdc); err != nil {
			return nil, fmt.Errorf("unmarshal returned GuestDefinedCapabilities for windows: %w", err)
		}
		return gdc, nil
	}
	// linux
	gdc := &LCOWGuestDefinedCapabilities{}
	if err := json.Unmarshal(data, gdc); err != nil {
		return nil, fmt.Errorf("unmarshal returned GuestDefinedCapabilities for lcow: %w", err)
	}
	return gdc, nil
}

var _ GuestDefinedCapabilities = &LCOWGuestDefinedCapabilities{}

type LCOWGuestDefinedCapabilities struct {
	prot.GcsGuestCapabilities
}

func (l *LCOWGuestDefinedCapabilities) IsNamespaceAddRequestSupported() bool {
	return l.NamespaceAddRequestSupported
}

func (l *LCOWGuestDefinedCapabilities) IsSignalProcessSupported() bool {
	return l.SignalProcessSupported
}

func (l *LCOWGuestDefinedCapabilities) IsDumpStacksSupported() bool {
	return l.DumpStacksSupported
}

func (l *LCOWGuestDefinedCapabilities) IsDeleteContainerStateSupported() bool {
	return l.DeleteContainerStateSupported
}

var _ GuestDefinedCapabilities = &WCOWGuestDefinedCapabilities{}

type WCOWGuestDefinedCapabilities struct {
	schema1.GuestDefinedCapabilities
}

func (w *WCOWGuestDefinedCapabilities) IsNamespaceAddRequestSupported() bool {
	return w.NamespaceAddRequestSupported
}

func (w *WCOWGuestDefinedCapabilities) IsSignalProcessSupported() bool {
	return w.SignalProcessSupported
}

func (w *WCOWGuestDefinedCapabilities) IsDumpStacksSupported() bool {
	return w.DumpStacksSupported
}

func (w *WCOWGuestDefinedCapabilities) IsDeleteContainerStateSupported() bool {
	return w.DeleteContainerStateSupported
}

func (w *WCOWGuestDefinedCapabilities) IsLogForwardingSupported() bool {
	return w.LogForwardingSupported
}
