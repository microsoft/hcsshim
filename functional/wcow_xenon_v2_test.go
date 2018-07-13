// xxxxbuild functional wcow wcowv2 wcowv2xenon

package functional

//import (
//	"os"
//	"testing"

//	"github.com/Microsoft/hcsshim/functional/utilities"
//	"github.com/Microsoft/hcsshim/internal/guid"
//	"github.com/Microsoft/hcsshim/internal/hcsoci"
//	"github.com/Microsoft/hcsshim/internal/osversion"
//	"github.com/Microsoft/hcsshim/internal/schemaversion"
//	"github.com/Microsoft/hcsshim/internal/uvm"
//	"github.com/Microsoft/hcsshim/internal/uvmfolder"
//	"github.com/Microsoft/hcsshim/internal/wclayer"
//	"github.com/Microsoft/hcsshim/internal/wcow"
//	specs "github.com/opencontainers/runtime-spec/specs-go"
//)

//// TODO: This is just copied out of the old wcow_xenon_test.go under hcsoci. Needs re-implementing.
//// TODO: Also need to copy out the wcow v1
//// TODO: Also need to copy of the wcow argon (v1/v2)

//// Helper to create a utility VM. Returns the UtilityVM and folder used as its scratch
//func createv2WCOWUVM(t *testing.T, uvmLayers []string, uvmId string, resources *specs.WindowsResources) (*uvm.UtilityVM, string) {
//	if uvmId == "" {
//		uvmId = guid.New().String()
//	}

//	uvmImageFolder, err := uvmfolder.LocateUVMFolder(uvmLayers)
//	if err != nil {
//		t.Fatal("Failed to locate UVM folder from layers")
//	}

//	scratchFolder := testutilities.CreateTempDir(t)
//	if err := wcow.CreateUVMScratch(uvmImageFolder, scratchFolder, uvmId); err != nil {
//		t.Fatalf("failed to create scratch: %s", err)
//	}

//	wcowUVM, err := uvm.Create(
//		&uvm.UVMOptions{
//			ID:              uvmId,
//			OperatingSystem: "windows",
//			LayerFolders:    append(uvmLayers, scratchFolder),
//			Resources:       resources,
//		})
//	if err != nil {
//		t.Fatalf("Failed create WCOW v2 UVM: %s", err)
//	}
//	if err := wcowUVM.Start(); err != nil {
//		wcowUVM.Terminate()
//		t.Fatalf("Failed start WCOW v2 UVM: %s", err)

//	}
//	return wcowUVM, scratchFolder
//}

//// TODO What about mount. Test with the client doing the mount.
//// TODO Test as above, but where sandbox for UVM is entirely created by a client to show how it's done.

//// Two v2 WCOW containers in the same UVM, each with a single base layer
//func TestV2XenonWCOWTwoContainers(t *testing.T) {
//	t.Skip("Skipping for now")
//	uvm, uvmScratchDir := createv2WCOWUVM(t, layersNanoserver, "TestV2XenonWCOWTwoContainers_UVM", nil)
//	defer os.RemoveAll(uvmScratchDir)
//	defer uvm.Terminate()

//	// First hosted container
//	firstContainerScratchDir := createWCOWTempDirWithSandbox(t)
//	defer os.RemoveAll(firstContainerScratchDir)
//	firstLayerFolders := append(layersNanoserver, firstContainerScratchDir)
//	firstHostedContainer, err := CreateContainer(&CreateOptions{
//		Id:            "FirstContainer",
//		HostingSystem: uvm,
//		SchemaVersion: schemaversion.SchemaV21(),
//		Spec:          &specs.Spec{Windows: &specs.Windows{LayerFolders: firstLayerFolders}},
//	})
//	if err != nil {
//		t.Fatalf("CreateContainer failed: %s", err)
//	}
//	defer unmountContainerLayers(firstLayerFolders, uvm, unmountOperationAll)

//	// Second hosted container
//	secondContainerScratchDir := createWCOWTempDirWithSandbox(t)
//	defer os.RemoveAll(firstContainerScratchDir)
//	secondLayerFolders := append(layersNanoserver, secondContainerScratchDir)
//	secondHostedContainer, err := CreateContainer(&CreateOptions{
//		Id:            "SecondContainer",
//		HostingSystem: uvm,
//		SchemaVersion: schemaversion.SchemaV21(),
//		Spec:          &specs.Spec{Windows: &specs.Windows{LayerFolders: secondLayerFolders}},
//	})
//	if err != nil {
//		t.Fatalf("CreateContainer failed: %s", err)
//	}
//	defer unmountContainerLayers(secondLayerFolders, uvm, unmountOperationAll)

//	startContainer(t, firstHostedContainer)
//	runCommand(t, firstHostedContainer, "cmd /s /c echo FirstContainer", `c:\`, "FirstContainer")
//	startContainer(t, secondHostedContainer)
//	runCommand(t, secondHostedContainer, "cmd /s /c echo SecondContainer", `c:\`, "SecondContainer")
//	stopContainer(t, firstHostedContainer)
//	stopContainer(t, secondHostedContainer)
//	firstHostedContainer.Terminate()
//	secondHostedContainer.Terminate()
//}

//// Lots of v2 WCOW containers in the same UVM, each with a single base layer. Containers aren't
//// actually started, but it stresses the SCSI controller hot-add logic.
//func TestV2XenonWCOWCreateLots(t *testing.T) {
//	t.Skip("Skipping for now")
//	uvm, uvmScratchDir := createv2WCOWUVM(t, layersNanoserver, "TestV2XenonWCOWCreateLots", nil)
//	defer os.RemoveAll(uvmScratchDir)
//	defer uvm.Terminate()

//	// 63 as 0:0 is already taken as the UVMs scratch. So that leaves us with 64-1 left for container scratches on SCSI
//	for i := 0; i < 63; i++ {
//		containerScratchDir := createWCOWTempDirWithSandbox(t)
//		defer os.RemoveAll(containerScratchDir)
//		layerFolders := append(layersNanoserver, containerScratchDir)
//		hostedContainer, err := CreateContainer(&CreateOptions{
//			Id:            fmt.Sprintf("container%d", i),
//			HostingSystem: uvm,
//			SchemaVersion: schemaversion.SchemaV21(),
//			Spec:          &specs.Spec{Windows: &specs.Windows{LayerFolders: layerFolders}},
//		})
//		if err != nil {
//			t.Fatalf("CreateContainer failed: %s", err)
//		}
//		defer hostedContainer.Terminate()
//		defer unmountContainerLayers(layerFolders, uvm, unmountOperationAll)
//	}

//	// TODO: Should check the internal structures here for VSMB and SCSI

//	// TODO: Push it over 63 now and will get a failure.
//}
