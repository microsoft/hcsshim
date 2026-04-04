//go:build windows && !wcow && !lcow

package network

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/hcn"
)

// guestNetwork is not supported for unsupported guests.
type guestNetwork interface{}

// addNetNSInsideGuest is not supported for unsupported guests.
func (c *Controller) addNetNSInsideGuest(_ context.Context, _ *hcn.HostComputeNamespace) error {
	return fmt.Errorf("unsupported guest")
}

// removeNetNSInsideGuest is not supported for unsupported guests.
func (c *Controller) removeNetNSInsideGuest(_ context.Context, _ string) error {
	return fmt.Errorf("unsupported guest")
}

// addEndpointToGuestNamespace is not supported for unsupported guests.
func (c *Controller) addEndpointToGuestNamespace(_ context.Context, _ string, _ *hcn.HostComputeEndpoint, _ bool) error {
	return fmt.Errorf("unsupported guest")
}

// removeEndpointFromGuestNamespace is not supported for unsupported guests.
func (c *Controller) removeEndpointFromGuestNamespace(_ context.Context, _ string, _ *hcn.HostComputeEndpoint) error {
	return fmt.Errorf("unsupported guest")
}
