//go:build windows && !wcow && !lcow

package vpci

import (
	"context"
	"fmt"

	"github.com/Microsoft/go-winio/pkg/guid"
)

// waitGuestDeviceReady is a not supported for unsupported guests.
func (c *Controller) waitGuestDeviceReady(_ context.Context, _ guid.GUID) error {
	return fmt.Errorf("unsupported guest")
}
