//go:build windows

package builder

import (
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm"
)

func TestStorageQoS(t *testing.T) {
	b, cs := newBuilder(t, vm.Linux)
	var storage StorageQoSOptions = b

	storage.SetStorageQoS(&hcsschema.StorageQoS{
		IopsMaximum:      1000,
		BandwidthMaximum: 2000,
	})
	if cs.VirtualMachine.StorageQoS == nil {
		t.Fatal("StorageQoS should be initialized")
	}
	if cs.VirtualMachine.StorageQoS.IopsMaximum != 1000 || cs.VirtualMachine.StorageQoS.BandwidthMaximum != 2000 {
		t.Fatal("StorageQoS not applied as expected")
	}
}
