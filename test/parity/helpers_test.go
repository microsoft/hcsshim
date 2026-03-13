//go:build windows && functional

package parity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
)

// setupBootFiles creates a temp directory with the dummy boot files that both
// builders probe for when resolving kernel and rootfs paths.
func setupBootFiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{
		vmutils.KernelFile,
		vmutils.UncompressedKernelFile,
		vmutils.InitrdFile,
		vmutils.VhdFile,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create boot file %s: %v", name, err)
		}
	}
	return dir
}

// normalizeDoc makes both documents comparable by zeroing nondeterministic
// fields.
func normalizeDoc(doc *hcsschema.ComputeSystem) {
	if doc == nil {
		return
	}
	doc.Owner = ""

	vm := doc.VirtualMachine
	if vm == nil {
		return
	}

	// Empty StorageQoS is the same as nil (no QoS configured).
	if vm.StorageQoS != nil && vm.StorageQoS.IopsMaximum == 0 && vm.StorageQoS.BandwidthMaximum == 0 {
		vm.StorageQoS = nil
	}

	// Empty CpuGroup is the same as nil (no CPU group assigned).
	if vm.ComputeTopology != nil && vm.ComputeTopology.Processor != nil {
		if cg := vm.ComputeTopology.Processor.CpuGroup; cg != nil && cg.Id == "" {
			vm.ComputeTopology.Processor.CpuGroup = nil
		}
	}

	if vm.Devices == nil {
		return
	}

	// SCSI and vPCI maps use random GUID keys. Sort and re-index for
	// deterministic comparison.
	if scsi := vm.Devices.Scsi; scsi != nil {
		vm.Devices.Scsi = sortedMapKeys(scsi)
	}
	if vpci := vm.Devices.VirtualPci; vpci != nil {
		vm.Devices.VirtualPci = sortedVPCIKeys(vpci)
	}
}

func sortedMapKeys(m map[string]hcsschema.Scsi) map[string]hcsschema.Scsi {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]hcsschema.Scsi, len(m))
	for i, k := range keys {
		out[string(rune('0'+i))] = m[k]
	}
	return out
}

func sortedVPCIKeys(m map[string]hcsschema.VirtualPciDevice) map[string]hcsschema.VirtualPciDevice {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]hcsschema.VirtualPciDevice, len(m))
	for i, k := range keys {
		out[string(rune('0'+i))] = m[k]
	}
	return out
}

// mustJSON returns indented JSON for logging.
func mustJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(b)
}
