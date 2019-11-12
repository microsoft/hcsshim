// +build functional

package functional

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/uvm"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
)

// findTestDevices returns the first pcip device on the host
func findTestVirtualDevice() (string, error) {
	out, err := exec.Command(
		"powershell",
		`(Get-PnpDevice -presentOnly | where-object {$_.InstanceID -Match 'PCIP.*'})[0].InstanceId`,
	).Output()
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

func TestVirtualDevice(t *testing.T) {
	testutilities.RequiresBuild(t, 19516)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	testDeviceInstanceID, err := findTestVirtualDevice()
	if err != nil {
		t.Skipf("skipping test, failed to find assignable device on host with: %v", err)
	}
	if testDeviceInstanceID == "" {
		t.Skipf("skipping test, host has no assignable PCIP devices")
	}

	// update opts needed to assign a hyper-v pci device
	opts := uvm.NewDefaultOptionsLCOW(t.Name(), "")
	opts.VPCIEnabled = true
	opts.AllowOvercommit = false
	opts.KernelDirect = false
	opts.VPMemDeviceCount = 0
	opts.KernelFile = uvm.KernelFile
	opts.RootFSFile = uvm.InitrdFile
	opts.PreferredRootFSType = uvm.PreferredRootFSTypeInitRd

	// create test uvm and ensure we can assign and remove the device
	vm := testutilities.CreateLCOWUVMFromOpts(ctx, t, opts)
	dev := hcsschema.VirtualPciDevice{
		Functions: []hcsschema.VirtualPciFunction{
			{
				DeviceInstancePath: testDeviceInstanceID,
			},
		},
	}
	busGUID, err := vm.AssignDevice(ctx, dev)
	if err != nil {
		t.Fatalf("failed to assign device %s with %v", testDeviceInstanceID, err)
	}
	if err := vm.RemoveDevice(ctx, busGUID); err != nil {
		t.Fatalf("failed to remove device %s with %v", testDeviceInstanceID, err)
	}
}
