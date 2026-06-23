//go:build windows && (lcow || wcow)

package guestmanager

import (
	"context"
	"errors"
)

// ErrGuestConnectionUnavailable is returned when the guest connection is nil
// (not yet established, or already closed by [Guest.CloseConnection]).
var ErrGuestConnectionUnavailable = errors.New("guest connection not initialized")

// modify sends a guest modification request via the guest connection.
// This is a helper method to avoid having to check for a nil guest connection in every method that needs to send a request.
func (gm *Guest) modify(ctx context.Context, req interface{}) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.gc == nil {
		return ErrGuestConnectionUnavailable
	}
	return gm.gc.Modify(ctx, req)
}
