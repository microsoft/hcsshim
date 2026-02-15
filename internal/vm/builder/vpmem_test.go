//go:build windows

package builder

import (
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm"
)

func TestVPMem(t *testing.T) {
	b, cs := newBuilder(t, vm.Linux)
	var devices DeviceOptions = b
	if err := devices.AddVPMemDevice("0", hcsschema.VirtualPMemDevice{HostPath: "pmem.img", ReadOnly: true, ImageFormat: "raw"}); err == nil {
		t.Fatal("AddVPMemDevice should fail when controller missing")
	}

	devices.AddVPMemController(2, 1024)
	if err := devices.AddVPMemDevice("0", hcsschema.VirtualPMemDevice{HostPath: "pmem.img", ReadOnly: true, ImageFormat: "raw"}); err != nil {
		t.Fatalf("AddVPMemDevice error = %v", err)
	}

	controller := cs.VirtualMachine.Devices.VirtualPMem
	if controller.MaximumCount != 2 || controller.MaximumSizeBytes != 1024 {
		t.Fatal("VPMem controller not applied as expected")
	}
	device := controller.Devices["0"]
	if device.HostPath != "pmem.img" || !device.ReadOnly || device.ImageFormat != "raw" {
		t.Fatal("VPMem device not applied as expected")
	}
}
