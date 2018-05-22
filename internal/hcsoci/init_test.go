//functional lcow

package hcsoci

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/Microsoft/hcsshim/uvm"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

var (
	// Obtained from docker - for the base images used in the tests
	layersNanoserver []string // Nanoserver matching the build
	layersAlpine     []string

	lcowGlobalSVM        *uvm.UtilityVM
	lcowCacheSandboxFile string
)

const lcowGlobalSVMID = "test.lcowglobalsvm"

func init() {
	if len(os.Getenv("HCSSHIM_FUNCTIONAL_TESTS_DEBUG")) > 0 {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	}
	layersNanoserver = getLayers("microsoft/nanoserver:latest")
	layersAlpine = getLayers("alpine")

	// Delete the global LCOW service VM if it exists.
	foo, err := hcs.OpenComputeSystem(lcowGlobalSVMID)
	if err == nil {
		foo.Terminate()
	}
}

func getLayers(imageName string) []string {
	cmd := exec.Command("docker", "inspect", imageName, "-f", `"{{.GraphDriver.Data.dir}}"`)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		panic("failed to get layers. Is the daemon running?")
	}
	imagePath := strings.Replace(strings.TrimSpace(out.String()), `"`, ``, -1)
	layers := getLayerChain(imagePath)
	return append([]string{imagePath}, layers...)
}

func getLayerChain(layerFolder string) []string {
	jPath := filepath.Join(layerFolder, "layerchain.json")
	content, err := ioutil.ReadFile(jPath)
	if os.IsNotExist(err) {
		panic("layerchain not found")
	} else if err != nil {
		panic("failed to read layerchain")
	}

	var layerChain []string
	err = json.Unmarshal(content, &layerChain)
	if err != nil {
		panic("failed to unmarshal layerchain")
	}
	return layerChain
}

// createTempDir creates a temporary directory
func createTempDir(t *testing.T) string {
	tempDir, err := ioutil.TempDir("", "hcsshimtestcase")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %s", err)
	}
	return tempDir
}

// Helper to create a WCOW utility VM. Returns the UtilityVM object; folder used as its scratch
func createWCOWUVM(t *testing.T, uvmLayers []string, uvmId string, resources *specs.WindowsResources) (*uvm.UtilityVM, string) {
	scratchDir := createTempDir(t)

	opts := &uvm.UVMOptions{
		OperatingSystem: "windows",
		LayerFolders:    append(uvmLayers, scratchDir),
		Resources:       resources,
	}
	uvm, err := uvm.Create(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := uvm.Start(); err != nil {
		t.Fatal(err)
	}

	return uvm, scratchDir
}

// Helper to create an LCOW utility VM.
func createLCOWUVM(t *testing.T, id string) *uvm.UtilityVM {
	opts := &uvm.UVMOptions{OperatingSystem: "linux"}
	if id != "" {
		opts.ID = id
	}
	uvm, err := uvm.Create(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := uvm.Start(); err != nil {
		t.Fatal(err)
	}
	return uvm
}

// createWCOWTempDirWithSandbox uses HCS to create a sandbox with VM group access
// in a temporary directory. Returns the directory where created.
// TODO: This is wrong. Need to search the folders.
func createWCOWTempDirWithSandbox(t *testing.T) string {
	tempDir := createTempDir(t)
	if err := wclayer.CreateSandboxLayer(tempDir, layersNanoserver); err != nil {
		t.Fatalf("Failed CreateSandboxLayer: %s", err)
	}
	return tempDir
}

// createLCOWTempDirWithSandbox uses an LCOW utility VM to create a blank
// VHDX and format it ext4. If vmID is supplied, it grants access to the
// destination file
func createLCOWTempDirWithSandbox(t *testing.T, vmID string) string {
	if lcowGlobalSVM == nil {
		lcowGlobalSVM = createLCOWUVM(t, lcowGlobalSVMID)
		lcowCacheSandboxFile = filepath.Join(createTempDir(t), "sandbox.vhdx")
	}
	tempDir := createTempDir(t)
	if err := lcowGlobalSVM.CreateLCOWSandbox(filepath.Join(tempDir, "sandbox.vhdx"), uvm.DefaultLCOWSandboxSizeGB, lcowCacheSandboxFile, vmID); err != nil {
		t.Fatalf("failed to create EXT4 sandbox for LCOW test cases: %s", err)
	}
	return tempDir
}
