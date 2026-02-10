//go:build windows

package guestmanager

import (
	"context"
	"errors"
)

var errGuestConnectionUnavailable = errors.New("guest connection not initialized")

// modify sends a guest modification request via the guest connection.
// This is a helper method to avoid having to check for a nil guest connection in every method that needs to send a request.
func (gm *Guest) modify(ctx context.Context, req interface{}) error {
	if gm.gc == nil {
		return errGuestConnectionUnavailable
	}
	return gm.gc.Modify(ctx, req)
}
