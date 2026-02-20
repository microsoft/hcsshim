//go:build windows

package builder

import (
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func newBuilder(t *testing.T) (*UtilityVM, *hcsschema.ComputeSystem) {
	t.Helper()
	b, err := New("owner")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return b, b.Get()
}

func TestNewBuilder_DefaultFields(t *testing.T) {
	_, cs := newBuilder(t)
	if cs.VirtualMachine == nil {
		t.Fatal("VirtualMachine should be initialized")
	}
	if cs.VirtualMachine.Devices == nil {
		t.Fatal("Devices should be initialized")
	}
	if cs.VirtualMachine.Devices.HvSocket == nil {
		t.Fatal("HvSocket should be initialized")
	}
}
