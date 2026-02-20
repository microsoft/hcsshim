//go:build windows

package builder

import (
	"reflect"
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func TestVSMB(t *testing.T) {
	b, cs := newBuilder(t)
	var devices DeviceOptions = b
	opts := &hcsschema.VirtualSmbShareOptions{ReadOnly: true}
	share1 := hcsschema.VirtualSmbShare{
		Name:         "data",
		Path:         "C:\\share",
		AllowedFiles: []string{"a.txt"},
		Options:      opts,
	}
	share2 := hcsschema.VirtualSmbShare{
		Name:         "data2",
		Path:         "C:\\share2",
		AllowedFiles: []string{"b.txt"},
		Options:      opts,
	}

	devices.AddVSMB(hcsschema.VirtualSmb{DirectFileMappingInMB: 1024})

	if err := devices.AddVSMBShare(share1); err != nil {
		t.Fatalf("AddVSMBShare error = %v", err)
	}
	if err := devices.AddVSMBShare(share2); err != nil {
		t.Fatalf("AddVSMBShare error = %v", err)
	}

	vsmb := cs.VirtualMachine.Devices.VirtualSmb
	if vsmb == nil || len(vsmb.Shares) != 2 {
		t.Fatal("VSMB not configured as expected")
	}
	if vsmb.DirectFileMappingInMB != 1024 {
		t.Fatalf("DirectFileMappingInMB = %d, want 1024", vsmb.DirectFileMappingInMB)
	}
	if !reflect.DeepEqual(vsmb.Shares[0], share1) || !reflect.DeepEqual(vsmb.Shares[1], share2) {
		t.Fatal("VSMB shares not applied as expected")
	}
}
