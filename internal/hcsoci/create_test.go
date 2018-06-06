// +build windows,functional

//
// These unit tests must run on a system setup to run both Argons and Xenons,
// have docker installed, and have the nanoserver (WCOW) and alpine (LCOW)
// base images installed. The nanoserver image MUST match the build of the
// host.
//
// We rely on docker as the tools to extract a container image aren't
// open source. We use it to find the location of the base image on disk.
//

package hcsoci

//import (
//	"bytes"
//	"encoding/json"
//	"io/ioutil"
//	"os"
//	"os/exec"
//	"path/filepath"
//	"strings"
//	"testing"

//	"github.com/Microsoft/hcsshim/internal/schemaversion"
//	_ "github.com/Microsoft/hcsshim/test/assets"
//	specs "github.com/opencontainers/runtime-spec/specs-go"
//	"github.com/sirupsen/logrus"
//)

//func startContainer(t *testing.T, c Container) {
//	if err := c.Start(); err != nil {
//		t.Fatalf("Failed start: %s", err)
//	}
//}

//func startUVM(t *testing.T, uvm *UtilityVM) {
//	if err := uvm.Start(); err != nil {
//		t.Fatalf("UVM %s Failed start: %s", uvm.Id, err)
//	}
//}

//// Helper to launch a process in it. At the
//// point of calling, the container must have been successfully created.
//// TODO Convert to CreateProcessEx using full OCI spec.
//func runCommand(t *testing.T, c Container, command, workdir, expectedOutput string) {
//	if c == nil {
//		t.Fatalf("requested container to start is nil!")
//	}
//	p, err := c.CreateProcess(&ProcessConfig{
//		CommandLine:      command,
//		WorkingDirectory: workdir,
//		CreateStdInPipe:  true,
//		CreateStdOutPipe: true,
//		CreateStdErrPipe: true,
//	})
//	if err != nil {
//		//		c.DebugLCOWGCS()
//		//		time.Sleep(60 * time.Minute)
//		t.Fatalf("Failed Create Process: %s", err)

//	}
//	defer p.Close()
//	if err := p.Wait(); err != nil {
//		t.Fatalf("Failed Wait Process: %s", err)
//	}
//	exitCode, err := p.ExitCode()
//	if err != nil {
//		t.Fatalf("Failed to obtain process exit code: %s", err)
//	}
//	if exitCode != 0 {
//		t.Fatalf("Non-zero exit code from process %s (%d)", command, exitCode)
//	}
//	_, o, _, err := p.Stdio()
//	if err != nil {
//		t.Fatalf("Failed to get Stdio handles for process: %s", err)
//	}
//	buf := new(bytes.Buffer)
//	buf.ReadFrom(o)
//	out := strings.TrimSpace(buf.String())
//	if out != expectedOutput {
//		t.Fatalf("Failed to get %q from process: %q", expectedOutput, out)
//	}
//}

//// Helper to stop a container
//func stopContainer(t *testing.T, c Container) {
//	if err := c.Shutdown(); err != nil {
//		if IsPending(err) {
//			if err := c.Wait(); err != nil {
//				t.Fatalf("Failed Wait shutdown: %s", err)
//			}
//		} else {
//			t.Fatalf("Failed shutdown: %s", err)
//		}
//	}
//	//c.Terminate()
//}

//// Helper to shoot a utility VM
//func terminateUtilityVM(t *testing.T, uvm *UtilityVM) {
//	if err := uvm.Terminate(); err != nil {
//		t.Fatalf("Failed terminate utility VM %s", err)
//	}
//}

//// TODO: Test UVMResourcesFromContainerSpec
//func TestUVMSizing(t *testing.T) {
//	t.Skip("for now - not implemented at all")
//}

//// TestID validates that the requested ID is retrieved
//func TestID(t *testing.T) {
//	t.Skip("fornow")
//	tempDir := createWCOWTempDirWithSandbox(t)
//	defer os.RemoveAll(tempDir)

//	layers := append(layersNanoserver, tempDir)
//	mountPath, err := mountContainerLayers(layers, nil)
//	if err != nil {
//		t.Fatalf("failed to mount container storage: %s", err)
//	}
//	defer unmountContainerLayers(layers, nil, unmountOperationAll)

//	c, err := CreateContainer(&CreateOptions{
//		Id:            "gruntbuggly",
//		SchemaVersion: schemaversion.SchemaV20(),
//		Spec: &specs.Spec{
//			Windows: &specs.Windows{LayerFolders: layers},
//			Root:    &specs.Root{Path: mountPath.(string)},
//		},
//	})
//	if err != nil {
//		t.Fatalf("Failed create: %s", err)
//	}
//	if c.ID() != "gruntbuggly" {
//		t.Fatalf("id not set correctly: %s", c.ID())
//	}

//	c.Terminate()
//}
