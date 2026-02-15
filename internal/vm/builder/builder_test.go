//go:build windows

package builder

import (
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm"

	"github.com/pkg/errors"
)

func newBuilder(t *testing.T, guestOS vm.GuestOS) (*UtilityVM, *hcsschema.ComputeSystem) {
	t.Helper()
	b, err := New("owner", guestOS)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return b, b.Get()
}

func TestNewBuilder_InvalidGuestOS(t *testing.T) {
	if _, err := New("owner", vm.GuestOS("unknown")); !errors.Is(err, errUnknownGuestOS) {
		t.Fatalf("New() error = %v, want %v", err, errUnknownGuestOS)
	}
}

func TestNewBuilder_DefaultDevices(t *testing.T) {
	_, cs := newBuilder(t, vm.Windows)
	if cs.VirtualMachine.Devices.VirtualSmb == nil {
		t.Fatal("VirtualSmb should be initialized for Windows")
	}
	if cs.VirtualMachine.Devices.Plan9 != nil {
		t.Fatal("Plan9 should be nil for Windows")
	}

	_, cs = newBuilder(t, vm.Linux)
	if cs.VirtualMachine.Devices.Plan9 == nil {
		t.Fatal("Plan9 should be initialized for Linux")
	}
	if cs.VirtualMachine.Devices.VirtualSmb != nil {
		t.Fatal("VirtualSmb should be nil for Linux")
	}
}
