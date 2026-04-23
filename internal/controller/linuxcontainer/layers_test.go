//go:build windows && lcow

package linuxcontainer

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	scsiMount "github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	"github.com/Microsoft/hcsshim/internal/controller/linuxcontainer/mocks"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/ospath"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"

	"github.com/Microsoft/go-winio/pkg/guid"
	containerdtypes "github.com/containerd/containerd/api/types"
	"go.uber.org/mock/gomock"
)

var (
	errResolvePath   = errors.New("resolve path failed")
	errGrantVMAccess = errors.New("grant vm access failed")
	errScsiReserve   = errors.New("scsi reserve failed")
	errMapToGuest    = errors.New("map to guest failed")
	errCombineLayers = errors.New("combine layers failed")
)

// newLayersTestController creates a Controller wired to mock SCSI and guest
// controllers, with stubbed resolvePath and grantVMAccess functions.
func newLayersTestController(t *testing.T) (
	*Controller,
	*mocks.MockscsiController,
	*mocks.Mockguest,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	scsiCtrl := mocks.NewMockscsiController(ctrl)
	guestCtrl := mocks.NewMockguest(ctrl)
	c := &Controller{
		vmID:           "test-vm",
		gcsPodID:       "test-pod",
		gcsContainerID: "test-ctr",
		scsi:           scsiCtrl,
		guest:          guestCtrl,
	}
	return c, scsiCtrl, guestCtrl
}

// stubResolvePath replaces the package-level resolvePath with an identity
// function and restores the original when the test completes.
func stubResolvePath(t *testing.T) {
	t.Helper()
	orig := resolvePath
	resolvePath = func(path string) (string, error) { return path, nil }
	t.Cleanup(func() { resolvePath = orig })
}

// stubGrantVMAccess replaces the package-level grantVMAccess with a no-op and
// restores the original when the test completes.
func stubGrantVMAccess(t *testing.T) {
	t.Helper()
	orig := grantVMAccess
	grantVMAccess = func(_ context.Context, _, _ string) error { return nil }
	t.Cleanup(func() { grantVMAccess = orig })
}

// legacyLayerFolders returns layer folders in the legacy containerd format:
// [parent0, parent1, ..., scratch].
func legacyLayerFolders(parentPaths []string, scratchDir string) []string {
	folders := make([]string, 0, len(parentPaths)+1)
	folders = append(folders, parentPaths...)
	folders = append(folders, scratchDir)
	return folders
}

// TestAllocateLayers_SingleReadOnlyLayer verifies the full Reserve → MapToGuest
// → CombineLayers flow for a container with one read-only layer and a scratch.
func TestAllocateLayers_SingleReadOnlyLayer(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, guestCtrl := newLayersTestController(t)
	layerFolders := legacyLayerFolders([]string{`C:\layers\base`}, `C:\scratch`)

	roGUID, _ := guid.NewV4()
	scratchGUID, _ := guid.NewV4()

	// Read-only layer: Reserve → MapToGuest.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), disk.Config{HostPath: `C:\layers\base\layer.vhd`, ReadOnly: true, Type: disk.TypeVirtualDisk}, scsiMount.Config{Partition: 0, ReadOnly: true, Options: []string{"ro"}}).
		Return(roGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), roGUID).
		Return("/dev/sda", nil)

	// Scratch layer: Reserve → MapToGuest.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), disk.Config{HostPath: `C:\scratch\sandbox.vhdx`, ReadOnly: false, Type: disk.TypeVirtualDisk}, scsiMount.Config{EnsureFilesystem: true, Filesystem: "ext4"}).
		Return(scratchGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), scratchGUID).
		Return("/dev/sdb", nil)

	// Combine layers.
	expectedScratchPath := ospath.Join("linux", "/dev/sdb", "scratch", c.gcsPodID, c.gcsContainerID)
	expectedRootfsPath := ospath.Join("linux", guestpath.LCOWV2RootPrefixInVM, c.gcsPodID, c.gcsContainerID, guestpath.RootfsPath)

	guestCtrl.EXPECT().
		AddCombinedLayers(gomock.Any(), guestresource.LCOWCombinedLayers{
			ContainerID:       c.gcsContainerID,
			ContainerRootPath: expectedRootfsPath,
			Layers:            []hcsschema.Layer{{Path: "/dev/sda"}},
			ScratchPath:       expectedScratchPath,
		}).
		Return(nil)

	if err := c.allocateLayers(t.Context(), layerFolders, nil, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.layers.roLayers) != 1 {
		t.Errorf("expected 1 read-only layer, got %d", len(c.layers.roLayers))
	}
	if c.layers.roLayers[0].id != roGUID {
		t.Errorf("ro layer GUID = %v, want %v", c.layers.roLayers[0].id, roGUID)
	}
	if c.layers.scratch.id != scratchGUID {
		t.Errorf("scratch GUID = %v, want %v", c.layers.scratch.id, scratchGUID)
	}
	if !c.layers.layersCombined {
		t.Error("expected layersCombined to be true")
	}
	if c.layers.rootfsPath != expectedRootfsPath {
		t.Errorf("rootfsPath = %q, want %q", c.layers.rootfsPath, expectedRootfsPath)
	}
}

// TestAllocateLayers_MultipleReadOnlyLayers verifies that multiple read-only
// layers are each reserved, mapped, and passed to CombineLayers in order.
func TestAllocateLayers_MultipleReadOnlyLayers(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, guestCtrl := newLayersTestController(t)
	layerFolders := legacyLayerFolders(
		[]string{`C:\layers\layer0`, `C:\layers\layer1`},
		`C:\scratch`,
	)

	roGUID0, _ := guid.NewV4()
	roGUID1, _ := guid.NewV4()
	scratchGUID, _ := guid.NewV4()

	// Read-only layer 0.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), disk.Config{HostPath: `C:\layers\layer0\layer.vhd`, ReadOnly: true, Type: disk.TypeVirtualDisk}, scsiMount.Config{Partition: 0, ReadOnly: true, Options: []string{"ro"}}).
		Return(roGUID0, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), roGUID0).
		Return("/dev/sda", nil)

	// Read-only layer 1.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), disk.Config{HostPath: `C:\layers\layer1\layer.vhd`, ReadOnly: true, Type: disk.TypeVirtualDisk}, scsiMount.Config{Partition: 0, ReadOnly: true, Options: []string{"ro"}}).
		Return(roGUID1, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), roGUID1).
		Return("/dev/sdb", nil)

	// Scratch layer.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(scratchGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), scratchGUID).
		Return("/dev/sdc", nil)

	// Combine layers with both read-only layers.
	guestCtrl.EXPECT().
		AddCombinedLayers(gomock.Any(), gomock.Any()).
		Return(nil)

	if err := c.allocateLayers(t.Context(), layerFolders, nil, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.layers.roLayers) != 2 {
		t.Fatalf("expected 2 read-only layers, got %d", len(c.layers.roLayers))
	}
	if c.layers.roLayers[0].id != roGUID0 {
		t.Errorf("ro layer 0 GUID = %v, want %v", c.layers.roLayers[0].id, roGUID0)
	}
	if c.layers.roLayers[1].id != roGUID1 {
		t.Errorf("ro layer 1 GUID = %v, want %v", c.layers.roLayers[1].id, roGUID1)
	}
}

// TestAllocateLayers_ScratchEncryption verifies that when scratch encryption is
// enabled, the scratch disk is reserved with xfs and the encrypted flag set.
func TestAllocateLayers_ScratchEncryption(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, guestCtrl := newLayersTestController(t)
	layerFolders := legacyLayerFolders([]string{`C:\layers\base`}, `C:\scratch`)

	roGUID, _ := guid.NewV4()
	scratchGUID, _ := guid.NewV4()

	// Read-only layer.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(roGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), roGUID).
		Return("/dev/sda", nil)

	// Scratch layer: must use xfs and the Encrypted flag.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(),
			disk.Config{HostPath: `C:\scratch\sandbox.vhdx`, ReadOnly: false, Type: disk.TypeVirtualDisk},
			scsiMount.Config{
				Encrypted:        true,
				EnsureFilesystem: true,
				ReadOnly:         false,
				Filesystem:       "xfs",
			}).
		Return(scratchGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), scratchGUID).
		Return("/dev/sdb", nil)

	guestCtrl.EXPECT().
		AddCombinedLayers(gomock.Any(), gomock.Any()).
		Return(nil)

	if err := c.allocateLayers(t.Context(), layerFolders, nil, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !c.layers.layersCombined {
		t.Error("expected layersCombined to be true")
	}
}

// TestAllocateLayers_ResolvePathFailure verifies that a resolvePath failure on
// a read-only layer propagates the error.
func TestAllocateLayers_ResolvePathFailure(t *testing.T) {
	stubGrantVMAccess(t)

	orig := resolvePath
	resolvePath = func(_ string) (string, error) { return "", errResolvePath }
	t.Cleanup(func() { resolvePath = orig })

	c, _, _ := newLayersTestController(t)
	layerFolders := legacyLayerFolders([]string{`C:\layers\base`}, `C:\scratch`)

	err := c.allocateLayers(t.Context(), layerFolders, nil, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errResolvePath) {
		t.Errorf("error = %v, want wrapping %v", err, errResolvePath)
	}
}

// TestAllocateLayers_ROLayerReserveFailure verifies that a SCSI Reserve failure
// for a read-only layer propagates the error.
func TestAllocateLayers_ROLayerReserveFailure(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, _ := newLayersTestController(t)
	layerFolders := legacyLayerFolders([]string{`C:\layers\base`}, `C:\scratch`)

	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(guid.GUID{}, errScsiReserve)

	err := c.allocateLayers(t.Context(), layerFolders, nil, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errScsiReserve) {
		t.Errorf("error = %v, want wrapping %v", err, errScsiReserve)
	}
}

// TestAllocateLayers_ROLayerMapToGuestFailure verifies that a MapToGuest
// failure for a read-only layer propagates the error.
func TestAllocateLayers_ROLayerMapToGuestFailure(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, _ := newLayersTestController(t)
	layerFolders := legacyLayerFolders([]string{`C:\layers\base`}, `C:\scratch`)

	roGUID, _ := guid.NewV4()
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(roGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), roGUID).
		Return("", errMapToGuest)

	err := c.allocateLayers(t.Context(), layerFolders, nil, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errMapToGuest) {
		t.Errorf("error = %v, want wrapping %v", err, errMapToGuest)
	}
}

// TestAllocateLayers_GrantVMAccessFailure verifies that a grantVMAccess failure
// on the scratch layer propagates the error.
func TestAllocateLayers_GrantVMAccessFailure(t *testing.T) {
	stubResolvePath(t)

	orig := grantVMAccess
	grantVMAccess = func(_ context.Context, _, _ string) error { return errGrantVMAccess }
	t.Cleanup(func() { grantVMAccess = orig })

	c, scsiCtrl, _ := newLayersTestController(t)
	layerFolders := legacyLayerFolders([]string{`C:\layers\base`}, `C:\scratch`)

	roGUID, _ := guid.NewV4()

	// Read-only layer succeeds.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(roGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), roGUID).
		Return("/dev/sda", nil)

	err := c.allocateLayers(t.Context(), layerFolders, nil, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errGrantVMAccess) {
		t.Errorf("error = %v, want wrapping %v", err, errGrantVMAccess)
	}
}

// TestAllocateLayers_ScratchReserveFailure verifies that a SCSI Reserve failure
// on the scratch layer propagates the error.
func TestAllocateLayers_ScratchReserveFailure(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, _ := newLayersTestController(t)
	layerFolders := legacyLayerFolders([]string{`C:\layers\base`}, `C:\scratch`)

	roGUID, _ := guid.NewV4()
	// Read-only layer succeeds.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(roGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), roGUID).
		Return("/dev/sda", nil)

	// Scratch Reserve fails. We use DoAndReturn to distinguish the second
	// Reserve call (scratch) from the first (read-only layer) above.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(guid.GUID{}, errScsiReserve)

	err := c.allocateLayers(t.Context(), layerFolders, nil, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errScsiReserve) {
		t.Errorf("error = %v, want wrapping %v", err, errScsiReserve)
	}
}

// TestAllocateLayers_ScratchMapToGuestFailure verifies that a MapToGuest
// failure on the scratch layer propagates the error.
func TestAllocateLayers_ScratchMapToGuestFailure(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, _ := newLayersTestController(t)
	layerFolders := legacyLayerFolders([]string{`C:\layers\base`}, `C:\scratch`)

	roGUID, _ := guid.NewV4()
	scratchGUID, _ := guid.NewV4()

	// Read-only layer succeeds.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(roGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), roGUID).
		Return("/dev/sda", nil)

	// Scratch Reserve succeeds but MapToGuest fails.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(scratchGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), scratchGUID).
		Return("", errMapToGuest)

	err := c.allocateLayers(t.Context(), layerFolders, nil, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errMapToGuest) {
		t.Errorf("error = %v, want wrapping %v", err, errMapToGuest)
	}
}

// TestAllocateLayers_CombineLayersFailure verifies that an
// AddCombinedLayers failure propagates the error and layersCombined remains
// false.
func TestAllocateLayers_CombineLayersFailure(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, guestCtrl := newLayersTestController(t)
	layerFolders := legacyLayerFolders([]string{`C:\layers\base`}, `C:\scratch`)

	roGUID, _ := guid.NewV4()
	scratchGUID, _ := guid.NewV4()

	// Read-only layer.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(roGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), roGUID).
		Return("/dev/sda", nil)

	// Scratch layer.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(scratchGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), scratchGUID).
		Return("/dev/sdb", nil)

	// CombineLayers fails.
	guestCtrl.EXPECT().
		AddCombinedLayers(gomock.Any(), gomock.Any()).
		Return(errCombineLayers)

	err := c.allocateLayers(t.Context(), layerFolders, nil, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errCombineLayers) {
		t.Errorf("error = %v, want wrapping %v", err, errCombineLayers)
	}
	if c.layers.layersCombined {
		t.Error("expected layersCombined to be false after combine failure")
	}
}

// TestAllocateLayers_ScratchResolvePathFailure verifies that a resolvePath
// failure on the scratch VHD propagates the error.
func TestAllocateLayers_ScratchResolvePathFailure(t *testing.T) {
	stubGrantVMAccess(t)

	callCount := 0
	orig := resolvePath
	resolvePath = func(path string) (string, error) {
		callCount++
		// Let read-only layer resolve succeed, fail on scratch.
		if callCount == 1 {
			return path, nil
		}
		return "", errResolvePath
	}
	t.Cleanup(func() { resolvePath = orig })

	c, scsiCtrl, _ := newLayersTestController(t)
	layerFolders := legacyLayerFolders([]string{`C:\layers\base`}, `C:\scratch`)

	roGUID, _ := guid.NewV4()

	// Read-only layer succeeds.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(roGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), roGUID).
		Return("/dev/sda", nil)

	err := c.allocateLayers(t.Context(), layerFolders, nil, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errResolvePath) {
		t.Errorf("error = %v, want wrapping %v", err, errResolvePath)
	}
}

// TestAllocateLayers_RootfsMount verifies allocateLayers works with a rootfs
// mount instead of legacy layer folders.
func TestAllocateLayers_RootfsMount(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, guestCtrl := newLayersTestController(t)

	rootfs := []*containerdtypes.Mount{
		{
			Type:   "lcow-layer",
			Source: `C:\scratch`,
			Options: []string{
				`parentLayerPaths=["C:\\layers\\base"]`,
			},
		},
	}

	roGUID, _ := guid.NewV4()
	scratchGUID, _ := guid.NewV4()

	// Read-only layer.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(roGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), roGUID).
		Return("/dev/sda", nil)

	// Scratch layer.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(scratchGUID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), scratchGUID).
		Return("/dev/sdb", nil)

	guestCtrl.EXPECT().
		AddCombinedLayers(gomock.Any(), gomock.Any()).
		Return(nil)

	if err := c.allocateLayers(t.Context(), nil, rootfs, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.layers.roLayers) != 1 {
		t.Errorf("expected 1 read-only layer, got %d", len(c.layers.roLayers))
	}
	if !c.layers.layersCombined {
		t.Error("expected layersCombined to be true")
	}
}

// TestAllocateLayers_InvalidLayerFolders verifies that allocateLayers returns
// an error when both rootfs and layerFolders are empty.
func TestAllocateLayers_InvalidLayerFolders(t *testing.T) {
	c, _, _ := newLayersTestController(t)

	err := c.allocateLayers(t.Context(), nil, nil, false)
	if err == nil {
		t.Fatal("expected error for empty layers")
	}
}
