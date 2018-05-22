// +build windows

//,functional,lcow

// To run: go test -v -tags "functional lcow"

package hcsoci

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/uvm"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func getDefaultLinuxSpec(t *testing.T) *specs.Spec {
	content, err := ioutil.ReadFile(`..\..\test\assets\defaultlinuxspec.json`)
	if err != nil {
		t.Fatalf("failed to read defaultlinuxspec.json: %s", err.Error())
	}
	spec := specs.Spec{}
	if err := json.Unmarshal(content, &spec); err != nil {
		t.Fatalf("failed to unmarshal contents of defaultlinuxspec.json: %s", err.Error())
	}
	return &spec
}

//// A v1 LCOW
//// TODO LCOW doesn't work currently
//func TestV1XenonLCOW(t *testing.T) {
//	t.Skip("for now")
//	tempDir, _ := createLCOWTempDirWithSandboxv2(t)
//	defer os.RemoveAll(tempDir)

//	spec := getDefaultLinuxSpec(t)
//	//	spec.Windows.LayerFolders = append(layersAlpine, tempDir)
//	c, err := CreateContainer(&CreateOptions{
//		Id:            "TextV1XenonLCOW",
//		SchemaVersion: schemaversion.SchemaV10(),
//		Spec:          spec,
//	})
//	if err != nil {
//		t.Fatalf("Failed create: %s", err)
//	}

//	startContainer(t, c)
//	time.Sleep(5 * time.Second)
//	runCommand(t, c, "echo Hello", `/bin`, "Hello")
//	stopContainer(t, c)
//	c.Terminate()
//}

//// Returns
//// - Container object
//// - Containers scratch file host-path (added on SCSI - use RemoveSCSI to remove)
//func createV2LCOWUvm(t *testing.T, addScratch bool) (*UtilityVM, string) {
//	uvmScratchDir, _ := createLCOWTempDirWithSandboxv2(t)
//	//defer os.RemoveAll(uvmScratchDir)

//	scratchFile := ""
//	v2uvm := UtilityVM{
//		Id:              "v2LCOWuvm",
//		OperatingSystem: "linux",
//	}
//	if err := v2uvm.Create(); err != nil {
//		t.Fatalf("Failed create: %s", err)
//	}
//
//	startUVM(t, &v2uvm)
//
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

func TestV2XenonLCOW(t *testing.T) {
	cacheDir := createTempDir(t)
	defer os.RemoveAll(cacheDir)
	cacheFile := filepath.Join(cacheDir, "cache.vhdx")

	// This is what gets mounted into /tmp/scratch
	uvmScratchDir := createTempDir(t)
	defer os.RemoveAll(uvmScratchDir)
	uvmScratchFile := filepath.Join(uvmScratchDir, "uvmscratch.vhdx")

	// Sandbox for the first container
	c1SandboxDir := createTempDir(t)
	defer os.RemoveAll(c1SandboxDir)
	c1SandboxFile := filepath.Join(c1SandboxDir, "sandbox.vhdx")

	// Sandbox for the second container
	c2SandboxDir := createTempDir(t)
	defer os.RemoveAll(c2SandboxDir)
	c2SandboxFile := filepath.Join(c2SandboxDir, "sandbox.vhdx")

	opts := &uvm.UVMOptions{
		OperatingSystem: "linux",
		ID:              "uvm",
	}
	lcowUVM, err := uvm.Create(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := lcowUVM.Start(); err != nil {
		t.Fatal(err)
	}
	defer lcowUVM.Terminate()

	// Populate the cache and generate the scratch file for /tmp/scratch
	if err := lcowUVM.CreateLCOWSandbox(uvmScratchFile, uvm.DefaultLCOWSandboxSizeGB, cacheFile, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lcowUVM.AddSCSI(uvmScratchFile, `/tmp/scratch`); err != nil {
		t.Fatal(err)
	}

	// Now create the first containers sandbox, populate a spec
	if err := lcowUVM.CreateLCOWSandbox(c1SandboxFile, uvm.DefaultLCOWSandboxSizeGB, cacheFile, ""); err != nil {
		t.Fatal(err)
	}
	c1Spec := getDefaultLinuxSpec(t)
	c1Folders := append(layersAlpine, c1SandboxDir)
	c1Spec.Windows.LayerFolders = c1Folders
	c1Spec.Process = &specs.Process{Args: []string{"echo", "hello", "lcow", "container", "one"}}
	c1Opts := &CreateOptions{
		Spec:          c1Spec,
		HostingSystem: lcowUVM,
	}

	// Now create the second containers sandbox, populate a spec
	if err := lcowUVM.CreateLCOWSandbox(c2SandboxFile, uvm.DefaultLCOWSandboxSizeGB, cacheFile, ""); err != nil {
		t.Fatal(err)
	}
	c2Spec := getDefaultLinuxSpec(t)
	c2Folders := append(layersAlpine, c2SandboxDir)
	c2Spec.Windows.LayerFolders = c2Folders
	c2Spec.Process = &specs.Process{Args: []string{"echo", "hello", "lcow", "container", "two"}}
	c2Opts := &CreateOptions{
		Spec:          c2Spec,
		HostingSystem: lcowUVM,
	}

	// Create the first container
	c1, c1Resources, err := CreateContainer(c1Opts)
	fmt.Println(c1, c1Resources, err)
	if err != nil {
		t.Fatal(err)
	}

	// Create the second container
	c2, c2Resources, err := CreateContainer(c2Opts)
	fmt.Println(c2, c2Resources, err)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)

}

//// A v2 LCOW
//func TestV2XenonLCOW(t *testing.T) {
//	t.Skip("for now")
//	v2uvm, v2uvmScratchFile := createV2LCOWUvm(t, false)
//	if v2uvmScratchFile != "" {
//		defer v2uvm.RemoveSCSI(v2uvmScratchFile)
//		defer os.RemoveAll(filepath.Dir(v2uvmScratchFile))
//	}
//	defer v2uvm.Terminate()

//	containerScratchDir, _ := createLCOWTempDirWithSandboxv2(t)
//	defer os.RemoveAll(containerScratchDir)
//	if err := GrantVmAccess(v2uvm.Id, filepath.Join(containerScratchDir, "sandbox.vhdx")); err != nil {
//		t.Fatalf("Failed GrantVmAccess on sandbox.vhdx: %s", err)
//	}

//	spec := getDefaultLinuxSpec(t)
//	spec.Windows.LayerFolders = append(layersAlpine, containerScratchDir)
//	hostedContainer, err := CreateContainer(&CreateOptions{
//		Id:            "TextV2XenonLCOW",
//		SchemaVersion: schemaversion.SchemaV20(),
//		Spec:          spec,
//		HostingSystem: v2uvm,
//	})
//	if err != nil {
//		t.Fatalf("Failed create: %s", err)
//	}

//	startContainer(t, hostedContainer)
//	stopContainer(t, hostedContainer)

//	//	pmid, uvmpath, err := AddVPMEM(v2uvm, filepath.Join(layersAlpine[0], "layer.vhd"), "", true)
//	//	fmt.Println(pmid, uvmpath, err)
//	//	RemoveVPMEM(v2uvm, filepath.Join(layersAlpine[0], "layer.vhd"))

//	//	runCommand(t, v2uvm, "echo Hello", `/bin`, "Hello")
//	terminateUtilityVM(t, v2uvm)
//}
