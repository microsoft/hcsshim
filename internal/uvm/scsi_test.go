package uvm

//import (
//	"fmt"
//	"os"
//	"path/filepath"
//	"testing"
//	"time"

//	"github.com/Microsoft/hcsshim/functional/utilities"
//	"github.com/Microsoft/hcsshim/internal/osversion"
//	"github.com/Microsoft/hcsshim/uvm"
//	"github.com/sirupsen/logrus"
//)

// TODO: Should be able to make this a unit-test
//// TestAllocateSCSI tests allocateSCSI/deallocateSCSI/findSCSIAttachment
//func TestAllocateSCSI(t *testing.T) {
//	imageName := "microsoft/nanoserver"
//	uvm, uvmScratchDir := createWCOWUVM(t, framework.LayerFolders(t, imageName), "", nil)
//	defer os.RemoveAll(uvmScratchDir)
//	defer uvm.Terminate()

//	c, l, _, err := uvm.findSCSIAttachment(filepath.Join(uvmScratchDir, `sandbox.vhdx`))
//	if err != nil {
//		t.Fatalf("failed to find sandbox %s", err)
//	}
//	if c != 0 && l != 0 {
//		t.Fatalf("sandbox at %d:%d", c, l)
//	}

//	for i := 0; i <= (4*64)-2; i++ { // 4 controllers, each with 64 slots but 0:0 is the UVM scratch
//		controller, lun, err := uvm.allocateSCSI(`anything`)
//		if err != nil {
//			t.Fatalf("unexpected error %s", err)
//		}
//		if lun != (i+1)%64 {
//			t.Fatalf("unexpected LUN:%d i=%d", lun, i)
//		}
//		if controller != (i+1)/64 {
//			t.Fatalf("unexpected controller:%d i=%d", controller, i)
//		}
//	}
//	_, _, err = uvm.allocateSCSI(`shouldfail`)
//	if err == nil {
//		t.Fatalf("expected error")
//	}
//	if err.Error() != "no free SCSI locations" {
//		t.Fatalf("expected to have run out of SCSI slots")
//	}

//	for c := 0; c < 4; c++ {
//		for l := 0; l < 64; l++ {
//			if !(c == 0 && l == 0) {
//				uvm.deallocateSCSI(c, l)
//			}
//		}
//	}
//	if uvm.scsiLocations[0][0].hostPath == "" {
//		t.Fatalf("0:0 should still be taken")
//	}
//	c, l, _, err = uvm.findSCSIAttachment(filepath.Join(uvmScratchDir, `sandbox.vhdx`))
//	if err != nil {
//		t.Fatalf("failed to find scratch %s", err)
//	}
//	if c != 0 && l != 0 {
//		t.Fatalf("scratch at %d:%d", c, l)
//	}
//}
