// +build windows

package uvm

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func init() {
	//if os.Getenv("HCSSHIM_TEST_DEBUG") != "" {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		//.		TimestampFormat: "2006-01-02T15:04:05.000000000Z07:00",
		FullTimestamp: true,
	})
	//}
}

// Returns
// - Container object
// - Containers scratch file host-path (added on SCSI - use RemoveSCSI to remove)
//func createV2LCOWUvm(t *testing.T, addScratch bool) (*UtilityVM, string) {

//	v2uvm := UtilityVM{
//		Id:              "v2LCOWuvm",
//		OperatingSystem: "linux",
//	}
//	if err := v2uvm.Create(); err != nil {
//		t.Fatalf("Failed create: %s", err)
//	}

//	startUVM(t, &v2uvm)

//	if addScratch {
//		scratchFile = filepath.Join(uvmScratchDir, "sandbox.vhdx")
//		if err := GrantVmAccess("uvm", scratchFile); err != nil {
//			t.Fatalf("Failed grantvmaccess: %s", err)
//		}
//		controller, lun, err := v2uvm.AddSCSI(scratchFile, "/tmp/scratch")
//		if err != nil {
//			t.Fatalf("Failed to add UVM scratch: %s", err)
//		}
//		if controller != 0 || lun != 0 {
//			t.Fatalf("expected 0:0")
//		}
//	}
//	return &v2uvm, scratchFile
//}

//// UVMOptions are the set of options passed to Create() to create a utility vm.
//type UVMOptions struct {
//	Id                      string                  // Identifier for the uvm. Defaults to generated GUID.
//	Owner                   string                  // Specifies the owner. Defaults to executable name.
//	OperatingSystem         string                  // "windows" or "linux".
//	Resources               *specs.WindowsResources // Optional resources for the utility VM. Supports Memory.limit and CPU.Count only currently. // TODO consider extending?
//	AdditionHCSDocumentJSON string                  // Optional additional JSON to merge into the HCS document prior

//	// WCOW specific parameters
//	LayerFolders []string // Set of folders for base layers and sandbox. Ordered from top most read-only through base read-only layer, followed by sandbox

//	// LCOW specific parameters
//	KirdPath               string // Folder in which kernel and initrd reside. Defaults to \Program Files\Linux Containers
//	KernelFile             string // Filename under KirdPath for the kernel. Defaults to bootx64.efi
//	InitrdFile             string // Filename under KirdPath for the initrd image. Defaults to initrd.img
//	KernelBootOptions      string // Additional boot options for the kernel
//	KernelDebugMode        bool   // Configures the kernel in debug mode using sane defaults
//	KernelDebugComPortPipe string // If kernel is in debug mode, can override the pipe here. Defaults to `\\.\pipe\vmpipe`
//}

func TestCreateBadOS(t *testing.T) {
	opts := &UVMOptions{
		OperatingSystem: "foobar",
	}
	_, err := Create(opts)
	if err == nil || (err != nil && err.Error() != `unsupported operating system "foobar"`) {
		t.Fatal(err)
	}
}

func TestCreateBadKirdPath(t *testing.T) {
	opts := &UVMOptions{
		OperatingSystem: "linux",
		KirdPath:        `c:\does\not\exist\I\hope`,
	}
	_, err := Create(opts)
	if err == nil || (err != nil && err.Error() != `kernel 'c:\does\not\exist\I\hope\bootx64.efi' not found`) {
		t.Fatal(err)
	}
}

func TestCreateLCOW(t *testing.T) {
	opts := &UVMOptions{
		OperatingSystem: "linux",
	}
	_, err := Create(opts)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateWCOWBadLayerFolders(t *testing.T) {
	opts := &UVMOptions{
		OperatingSystem: "windows",
	}
	_, err := Create(opts)
	if err == nil || (err != nil && err.Error() != `at least 2 LayerFolders must be supplied`) {
		t.Fatal(err)
	}
}

//	startUVM(t, &v2uvm)

//	if addScratch {
//		scratchFile = filepath.Join(uvmScratchDir, "sandbox.vhdx")
//		if err := GrantVmAccess("uvm", scratchFile); err != nil {
//			t.Fatalf("Failed grantvmaccess: %s", err)
//		}
//		controller, lun, err := v2uvm.AddSCSI(scratchFile, "/tmp/scratch")
//		if err != nil {
//			t.Fatalf("Failed to add UVM scratch: %s", err)
//		}
//		if controller != 0 || lun != 0 {
//			t.Fatalf("expected 0:0")
//		}
//	}
//	return &v2uvm, scratchFile
