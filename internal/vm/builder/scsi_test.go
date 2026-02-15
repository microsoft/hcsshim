//go:build windows

package builder

import (
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm"
)

func TestSCSI(t *testing.T) {
	b, cs := newBuilder(t, vm.Linux)
	var devices DeviceOptions = b

	if err := devices.AddSCSIDisk("0", "1", hcsschema.Attachment{Path: "disk.vhdx", Type_: "VirtualDisk", ReadOnly: false}); err == nil {
		t.Fatal("AddSCSIDisk should fail when controller missing")
	}

	devices.AddSCSIController("0")
	if err := devices.AddSCSIDisk("0", "1", hcsschema.Attachment{Path: "disk.vhdx", Type_: "VirtualDisk", ReadOnly: true}); err != nil {
		t.Fatalf("AddSCSIDisk error = %v", err)
	}

	ctrl := cs.VirtualMachine.Devices.Scsi["0"]
	att := ctrl.Attachments["1"]
	if att.Path != "disk.vhdx" || att.Type_ != "VirtualDisk" || !att.ReadOnly {
		t.Fatal("SCSI attachment not applied as expected")
	}

	if err := devices.AddSCSIDisk("missing", "1", hcsschema.Attachment{Path: "disk.vhdx", Type_: "VirtualDisk", ReadOnly: false}); err == nil {
		t.Fatal("AddSCSIDisk should fail when controller does not exist")
	}
}
