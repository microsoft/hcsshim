//go:build windows

package builder

import (
	"testing"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

func TestSCSI(t *testing.T) {
	b, cs := newBuilder(t)
	var devices DeviceOptions = b

	if err := devices.AddSCSIDisk("0", "1", hcsschema.Attachment{Path: "disk.vhdx", Type_: "VirtualDisk", ReadOnly: false}); err == nil {
		t.Fatal("AddSCSIDisk should fail when controller missing")
	}

	devices.AddSCSIController("0")
	if err := devices.AddSCSIDisk("0", "1", hcsschema.Attachment{Path: "disk.vhdx", Type_: "VirtualDisk", ReadOnly: true}); err != nil {
		t.Fatalf("AddSCSIDisk error = %v", err)
	}

	// Verify the attachment is reflected directly in the document (map reference semantics -
	// no write-back of the Scsi struct copy is needed).
	att := cs.VirtualMachine.Devices.Scsi["0"].Attachments["1"]
	if att.Path != "disk.vhdx" || att.Type_ != "VirtualDisk" || !att.ReadOnly {
		t.Fatal("SCSI attachment not applied as expected")
	}

	// Add a second disk and confirm both attachments are present in the document,
	// verifying that multiple calls also work correctly.
	if err := devices.AddSCSIDisk("0", "2", hcsschema.Attachment{Path: "disk2.vhdx", Type_: "VirtualDisk", ReadOnly: false}); err != nil {
		t.Fatalf("AddSCSIDisk (lun 2) error = %v", err)
	}
	if len(cs.VirtualMachine.Devices.Scsi["0"].Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(cs.VirtualMachine.Devices.Scsi["0"].Attachments))
	}
	att2 := cs.VirtualMachine.Devices.Scsi["0"].Attachments["2"]
	if att2.Path != "disk2.vhdx" || att2.ReadOnly {
		t.Fatal("second SCSI attachment not applied as expected")
	}

	if err := devices.AddSCSIDisk("missing", "1", hcsschema.Attachment{Path: "disk.vhdx", Type_: "VirtualDisk", ReadOnly: false}); err == nil {
		t.Fatal("AddSCSIDisk should fail when controller does not exist")
	}
}
