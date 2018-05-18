// +build windows,functional

package hcsoci

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/schemaversion"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// createLCOWTempDirWithSandboxv2 uses a v2 LCOW utility VM to create a blank
// VHDX and format it ext4.
func createLCOWTempDirWithSandboxv2(t *testing.T) (string, string) {
	if lcowServiceContainerV2 == nil {
		cacheSandboxDir = createTempDir(t)

		lcowServiceContainerV2 = &UtilityVM{
			Id:              "v2global",
			OperatingSystem: "linux",
			SchemaVersion:   schemaversion.SchemaV20(),
		}

		if err := lcowServiceContainerV2.Create(); err != nil {
			t.Fatalf("Failed create: %s", err)
		}

		if err := lcowServiceContainerV2.Start(); err != nil {
			t.Fatalf("Failed to start service container: %s", err)
		}
	}
	tempDir := createTempDir(t)
	cacheSandboxFile = filepath.Join(cacheSandboxDir, "sandbox.vhdx")
	if err := lcowServiceContainerV2.CreateLCOWScratch(filepath.Join(tempDir, "sandbox.vhdx"), DefaultLCOWScratchSizeGB, cacheSandboxFile); err != nil {
		t.Fatalf("failed to create EXT4 sandbox for LCOW test cases: %s", err)
	}
	return tempDir, filepath.Base(tempDir)
}

func getDefaultLinuxSpec(t *testing.T) *specs.Spec {
	content, err := ioutil.ReadFile(`.\testassets\defaultlinuxspec.json`)
	if err != nil {
		t.Fatalf("failed to read defaultlinuxspec.json: %s", err.Error())
	}
	spec := specs.Spec{}
	if err := json.Unmarshal(content, &spec); err != nil {
		t.Fatalf("failed to unmarshal contents of defaultlinuxspec.json: %s", err.Error())
	}
	return &spec
}

//// createLCOWTempDirWithSandbox uses an LCOW utility VM to create a blank
//// VHDX and format it ext4.
//func TestCreateLCOWScratch(t *testing.T) {
//	t.Skip("for now")
//	cacheDir := createTempDir(t)
//	cacheFile := filepath.Join(cacheDir, "cache.vhdx")
//	uvm, err := CreateContainerEx(&CreateOptionsEx{Spec: getDefaultLinuxSpec(t)})
//	if err != nil {
//		t.Fatalf("Failed create: %s", err)
//	}
//	defer uvm.Terminate()
//	if err := uvm.Start(); err != nil {
//		t.Fatalf("Failed to start service container: %s", err)
//	}

//	// 1: Default size, cache doesn't exist, but no UVM passed. Cannot be created
//	err = CreateLCOWScratch(nil, filepath.Join(cacheDir, "default.vhdx"), DefaultLCOWScratchSizeGB, cacheFile)
//	if err == nil {
//		t.Fatalf("expected an error creating LCOW scratch")
//	}
//	if err.Error() != "cannot create scratch disk as cache is not present and no utility VM supplied" {
//		t.Fatalf("Not expecting error %s", err)
//	}

//	// 2: Default size, no cache supplied and no UVM
//	err = CreateLCOWScratch(nil, filepath.Join(cacheDir, "default.vhdx"), DefaultLCOWScratchSizeGB, "")
//	if err == nil {
//		t.Fatalf("expected an error creating LCOW scratch")
//	}
//	if err.Error() != "cannot create scratch disk as cache is not present and no utility VM supplied" {
//		t.Fatalf("Not expecting error %s", err)
//	}

//	// 3: Default size. This should work and the cache should be created.
//	err = CreateLCOWScratch(uvm, filepath.Join(cacheDir, "default.vhdx"), DefaultLCOWScratchSizeGB, cacheFile)
//	if err != nil {
//		t.Fatalf("should succeed creating default size cache file: %s", err)
//	}
//	if _, err = os.Stat(cacheFile); err != nil {
//		t.Fatalf("failed to stat cache file after created: %s", err)
//	}
//	if _, err = os.Stat(filepath.Join(cacheDir, "default.vhdx")); err != nil {
//		t.Fatalf("failed to stat default.vhdx after created: %s", err)
//	}

//	// 4: Non-defaultsize. This should work and the cache should be created.
//	err = CreateLCOWScratch(uvm, filepath.Join(cacheDir, "nondefault.vhdx"), DefaultLCOWScratchSizeGB+1, cacheFile)
//	if err != nil {
//		t.Fatalf("should succeed creating default size cache file: %s", err)
//	}
//	if _, err = os.Stat(cacheFile); err != nil {
//		t.Fatalf("failed to stat cache file after created: %s", err)
//	}
//	if _, err = os.Stat(filepath.Join(cacheDir, "nondefault.vhdx")); err != nil {
//		t.Fatalf("failed to stat default.vhdx after created: %s", err)
//	}

//}

// A v1 LCOW
// TODO LCOW doesn't work currently
func TestV1XenonLCOW(t *testing.T) {
	t.Skip("for now")
	tempDir, _ := createLCOWTempDirWithSandboxv2(t)
	defer os.RemoveAll(tempDir)

	spec := getDefaultLinuxSpec(t)
	//	spec.Windows.LayerFolders = append(layersAlpine, tempDir)
	c, err := CreateContainerEx(&CreateOptionsEx{
		Id:            "TextV1XenonLCOW",
		SchemaVersion: schemaversion.SchemaV10(),
		Spec:          spec,
	})
	if err != nil {
		t.Fatalf("Failed create: %s", err)
	}

	startContainer(t, c)
	time.Sleep(5 * time.Second)
	runCommand(t, c, "echo Hello", `/bin`, "Hello")
	stopContainer(t, c)
	c.Terminate()
}

// Returns
// - Container object
// - Containers scratch file host-path (added on SCSI - use RemoveSCSI to remove)
func createV2LCOWUvm(t *testing.T, addScratch bool) (*UtilityVM, string) {
	uvmScratchDir, _ := createLCOWTempDirWithSandboxv2(t)
	//defer os.RemoveAll(uvmScratchDir)

	scratchFile := ""
	v2uvm := UtilityVM{
		Id:              "v2LCOWuvm",
		OperatingSystem: "linux",
	}
	if err := v2uvm.Create(); err != nil {
		t.Fatalf("Failed create: %s", err)
	}

	startUVM(t, &v2uvm)

	if addScratch {
		scratchFile = filepath.Join(uvmScratchDir, "sandbox.vhdx")
		if err := GrantVmAccess("uvm", scratchFile); err != nil {
			t.Fatalf("Failed grantvmaccess: %s", err)
		}
		controller, lun, err := v2uvm.AddSCSI(scratchFile, "/tmp/scratch")
		if err != nil {
			t.Fatalf("Failed to add UVM scratch: %s", err)
		}
		if controller != 0 || lun != 0 {
			t.Fatalf("expected 0:0")
		}
	}
	return &v2uvm, scratchFile
}

// A v2 LCOW
func TestV2XenonLCOW(t *testing.T) {
	t.Skip("for now")
	v2uvm, v2uvmScratchFile := createV2LCOWUvm(t, false)
	if v2uvmScratchFile != "" {
		defer v2uvm.RemoveSCSI(v2uvmScratchFile)
		defer os.RemoveAll(filepath.Dir(v2uvmScratchFile))
	}
	defer v2uvm.Terminate()

	containerScratchDir, _ := createLCOWTempDirWithSandboxv2(t)
	defer os.RemoveAll(containerScratchDir)
	if err := GrantVmAccess(v2uvm.Id, filepath.Join(containerScratchDir, "sandbox.vhdx")); err != nil {
		t.Fatalf("Failed GrantVmAccess on sandbox.vhdx: %s", err)
	}

	spec := getDefaultLinuxSpec(t)
	spec.Windows.LayerFolders = append(layersAlpine, containerScratchDir)
	hostedContainer, err := CreateContainerEx(&CreateOptionsEx{
		Id:            "TextV2XenonLCOW",
		SchemaVersion: schemaversion.SchemaV20(),
		Spec:          spec,
		HostingSystem: v2uvm,
	})
	if err != nil {
		t.Fatalf("Failed create: %s", err)
	}

	startContainer(t, hostedContainer)
	stopContainer(t, hostedContainer)

	//	pmid, uvmpath, err := AddVPMEM(v2uvm, filepath.Join(layersAlpine[0], "layer.vhd"), "", true)
	//	fmt.Println(pmid, uvmpath, err)
	//	RemoveVPMEM(v2uvm, filepath.Join(layersAlpine[0], "layer.vhd"))

	//	runCommand(t, v2uvm, "echo Hello", `/bin`, "Hello")
	terminateUtilityVM(t, v2uvm)
}
