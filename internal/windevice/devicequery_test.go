//go:build windows

package windevice

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Microsoft/go-winio/vhd"
)

// validateAgainstPnPUtil gets a list of disk interface instances via `getDeviceInterfaceInstancesByClass` method and then
// also gets the same via pnputil.exe and compares that those lists are identical.
func validateAgainstPnPUtil(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	initialInterfaces, err := getDeviceInterfaceInstancesByClass(ctx, &devClassDiskGUID, false)
	if err != nil {
		t.Fatalf("Failed to get initial disk interfaces: %v", err)
	}

	initialPnpDevices, err := getDiskDevicesFromPnpUtil()
	if err != nil {
		t.Fatalf("Failed to get initial disk devices from pnputil: %v", err)
	}

	slices.SortFunc(initialInterfaces, func(a, b string) int { return strings.Compare(a, b) })
	slices.SortFunc(initialPnpDevices, func(a, b string) int { return strings.Compare(a, b) })

	if !reflect.DeepEqual(initialInterfaces, initialPnpDevices) {
		t.Logf("interface list retrieved via API: %+v\n", initialInterfaces)
		t.Logf("interface list returned by pnputil: %+v\n", initialPnpDevices)
		t.Fatalf("interfaces retrieved via API doesn't match with the list returned by pnputil")
	}

}

func TestDeviceInterfaceInstancesWithPnpUtil(t *testing.T) {
	// do first validation against pnp util
	validateAgainstPnPUtil(t)

	// make a fake VHD and attach it.
	tempDir := t.TempDir()
	vhdxPath := filepath.Join(tempDir, "test.vhdx")
	if err := vhd.CreateVhdx(vhdxPath, 1, 1); err != nil {
		t.Fatalf("failed to create vhd: %s", err)
	}
	if err := vhd.AttachVhd(vhdxPath); err != nil {
		t.Fatalf("failed to attach vhd: %s", err)
	}
	t.Cleanup(func() {
		vhd.DetachVhd(vhdxPath)
	})

	// check if we still match with pnputil
	validateAgainstPnPUtil(t)

	vhd.DetachVhd(vhdxPath)

	// last check if we still match with pnputil
	validateAgainstPnPUtil(t)
}

// Get disk devices from pnputil
func getDiskDevicesFromPnpUtil() ([]string, error) {
	// Run pnputil to get all disk devices
	cmd := exec.Command("pnputil", "/enum-interfaces", "/enabled", "/class", fmt.Sprintf("{%s}", devClassDiskGUID.String()))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	// Parse the output to extract device IDs
	var devices []string
	lines := strings.Split(string(output), "\n")
	idRegex := regexp.MustCompile(`Interface Path:\s+(.+)`)

	for _, line := range lines {
		match := idRegex.FindStringSubmatch(line)
		if len(match) > 1 {
			devices = append(devices, strings.TrimSpace(match[1]))
		}
	}

	return devices, nil
}
