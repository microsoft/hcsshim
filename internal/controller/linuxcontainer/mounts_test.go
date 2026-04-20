//go:build windows && lcow

package linuxcontainer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	plan9Mount "github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/share"
	"github.com/Microsoft/hcsshim/internal/controller/device/scsi/disk"
	scsiMount "github.com/Microsoft/hcsshim/internal/controller/device/scsi/mount"
	"github.com/Microsoft/hcsshim/internal/controller/linuxcontainer/mocks"
	"github.com/Microsoft/hcsshim/internal/guestpath"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
)

var (
	errPlan9Reserve = errors.New("plan9 reserve failed")
	errPlan9Map     = errors.New("plan9 map to guest failed")
)

// newMountsTestController creates a Controller wired to mock SCSI and Plan9
// controllers alongside stubbed resolvePath and grantVMAccess functions.
func newMountsTestController(t *testing.T) (
	*Controller,
	*mocks.MockscsiController,
	*mocks.Mockplan9Controller,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	scsiCtrl := mocks.NewMockscsiController(ctrl)
	plan9Ctrl := mocks.NewMockplan9Controller(ctrl)
	c := &Controller{
		vmID:  "test-vm",
		scsi:  scsiCtrl,
		plan9: plan9Ctrl,
	}
	return c, scsiCtrl, plan9Ctrl
}

// --- allocateMounts: SCSI mount tests ---

// TestAllocateMounts_VirtualDisk verifies the full Reserve → MapToGuest flow
// for a virtual-disk mount, including source rewrite and type change.
func TestAllocateMounts_VirtualDisk(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      `C:\disks\data.vhdx`,
				Destination: "/mnt/data",
				Type:        mountTypeVirtualDisk,
				Options:     []string{"ro"},
			},
		},
	}

	id, _ := guid.NewV4()
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), disk.Config{
			HostPath: `C:\disks\data.vhdx`,
			ReadOnly: true,
			Type:     disk.TypeVirtualDisk,
		}, scsiMount.Config{
			ReadOnly: true,
			Options:  []string{"ro"},
		}).
		Return(id, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), id).
		Return("/dev/sda", nil)

	if err := c.allocateMounts(t.Context(), spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Mounts[0].Source != "/dev/sda" {
		t.Errorf("mount source = %q, want %q", spec.Mounts[0].Source, "/dev/sda")
	}
	if spec.Mounts[0].Type != mountTypeNone {
		t.Errorf("mount type = %q, want %q", spec.Mounts[0].Type, mountTypeNone)
	}
	if len(c.scsiResources) != 1 || c.scsiResources[0] != id {
		t.Errorf("scsiResources = %v, want [%v]", c.scsiResources, id)
	}
}

// TestAllocateMounts_PhysicalDisk verifies that a physical-disk mount uses
// PassThru disk type and correctly rewrites the spec.
func TestAllocateMounts_PhysicalDisk(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      `\\.\PhysicalDrive1`,
				Destination: "/mnt/disk",
				Type:        mountTypePhysicalDisk,
			},
		},
	}

	id, _ := guid.NewV4()
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), disk.Config{
			HostPath: `\\.\PhysicalDrive1`,
			ReadOnly: false,
			Type:     disk.TypePassThru,
		}, scsiMount.Config{}).
		Return(id, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), id).
		Return("/dev/sdb", nil)

	if err := c.allocateMounts(t.Context(), spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Mounts[0].Source != "/dev/sdb" {
		t.Errorf("mount source = %q, want %q", spec.Mounts[0].Source, "/dev/sdb")
	}
	if spec.Mounts[0].Type != mountTypeNone {
		t.Errorf("mount type = %q, want %q", spec.Mounts[0].Type, mountTypeNone)
	}
}

// TestAllocateMounts_ExtensibleVirtualDisk verifies EVD path parsing and SCSI
// reservation for an extensible-virtual-disk mount.
func TestAllocateMounts_ExtensibleVirtualDisk(t *testing.T) {
	stubResolvePath(t)

	c, scsiCtrl, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      `evd://provider-type/C:\disks\data.vhdx`,
				Destination: "/mnt/evd",
				Type:        mountTypeExtensibleVirtualDisk,
			},
		},
	}

	id, _ := guid.NewV4()
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), disk.Config{
			HostPath: `C:\disks\data.vhdx`,
			ReadOnly: false,
			Type:     disk.TypeExtensibleVirtualDisk,
			EVDType:  "provider-type",
		}, scsiMount.Config{}).
		Return(id, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), id).
		Return("/dev/sdc", nil)

	if err := c.allocateMounts(t.Context(), spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Mounts[0].Source != "/dev/sdc" {
		t.Errorf("mount source = %q, want %q", spec.Mounts[0].Source, "/dev/sdc")
	}
}

// TestAllocateMounts_BlockDevMount verifies that a block-device mount (indicated
// by a "blockdev://" destination prefix) keeps the bind type.
func TestAllocateMounts_BlockDevMount(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      `C:\disks\block.vhdx`,
				Destination: guestpath.BlockDevMountPrefix + "/dev/sda",
				Type:        mountTypeVirtualDisk,
			},
		},
	}

	id, _ := guid.NewV4()
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), scsiMount.Config{
			BlockDev: true,
		}).
		Return(id, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), id).
		Return("/dev/sda", nil)

	if err := c.allocateMounts(t.Context(), spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Block dev mounts retain bind type.
	if spec.Mounts[0].Type != mountTypeBind {
		t.Errorf("mount type = %q, want %q", spec.Mounts[0].Type, mountTypeBind)
	}
}

// TestAllocateMounts_SCSIReserveFailure verifies that a SCSI Reserve failure
// propagates the error.
func TestAllocateMounts_SCSIReserveFailure(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      `C:\disks\data.vhdx`,
				Destination: "/mnt/data",
				Type:        mountTypeVirtualDisk,
			},
		},
	}

	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(guid.GUID{}, errScsiReserve)

	err := c.allocateMounts(t.Context(), spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errScsiReserve) {
		t.Errorf("error = %v, want wrapping %v", err, errScsiReserve)
	}
}

// TestAllocateMounts_SCSIMapToGuestFailure verifies that a SCSI MapToGuest
// failure propagates the error.
func TestAllocateMounts_SCSIMapToGuestFailure(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      `C:\disks\data.vhdx`,
				Destination: "/mnt/data",
				Type:        mountTypeVirtualDisk,
			},
		},
	}

	id, _ := guid.NewV4()
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(id, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), id).
		Return("", errMapToGuest)

	err := c.allocateMounts(t.Context(), spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errMapToGuest) {
		t.Errorf("error = %v, want wrapping %v", err, errMapToGuest)
	}
}

// TestAllocateMounts_VirtualDiskResolvePathFailure verifies that a resolvePath
// failure for a virtual-disk mount propagates the error.
func TestAllocateMounts_VirtualDiskResolvePathFailure(t *testing.T) {
	stubGrantVMAccess(t)

	orig := resolvePath
	resolvePath = func(_ string) (string, error) { return "", errResolvePath }
	t.Cleanup(func() { resolvePath = orig })

	c, _, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      `C:\disks\data.vhdx`,
				Destination: "/mnt/data",
				Type:        mountTypeVirtualDisk,
			},
		},
	}

	err := c.allocateMounts(t.Context(), spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errResolvePath) {
		t.Errorf("error = %v, want wrapping %v", err, errResolvePath)
	}
}

// TestAllocateMounts_VirtualDiskGrantVMAccessFailure verifies that a
// grantVMAccess failure for a virtual-disk mount propagates the error.
func TestAllocateMounts_VirtualDiskGrantVMAccessFailure(t *testing.T) {
	stubResolvePath(t)

	orig := grantVMAccess
	grantVMAccess = func(_ context.Context, _, _ string) error { return errGrantVMAccess }
	t.Cleanup(func() { grantVMAccess = orig })

	c, _, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      `C:\disks\data.vhdx`,
				Destination: "/mnt/data",
				Type:        mountTypeVirtualDisk,
			},
		},
	}

	err := c.allocateMounts(t.Context(), spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errGrantVMAccess) {
		t.Errorf("error = %v, want wrapping %v", err, errGrantVMAccess)
	}
}

// TestAllocateMounts_EVDResolvePathFailure verifies that a resolvePath failure
// for an extensible-virtual-disk mount propagates the error.
func TestAllocateMounts_EVDResolvePathFailure(t *testing.T) {

	orig := resolvePath
	resolvePath = func(_ string) (string, error) { return "", errResolvePath }
	t.Cleanup(func() { resolvePath = orig })

	c, _, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      `evd://mytype/C:\disk.vhdx`,
				Destination: "/mnt/evd",
				Type:        mountTypeExtensibleVirtualDisk,
			},
		},
	}

	err := c.allocateMounts(t.Context(), spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errResolvePath) {
		t.Errorf("error = %v, want wrapping %v", err, errResolvePath)
	}
}

// --- allocateMounts: Plan9 bind mount tests ---

// TestAllocateMounts_Plan9BindDirectory verifies the Reserve → MapToGuest flow
// for a host directory bind mount served via Plan9.
func TestAllocateMounts_Plan9BindDirectory(t *testing.T) {

	c, _, plan9Ctrl := newMountsTestController(t)
	hostDir := t.TempDir()

	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      hostDir,
				Destination: "/mnt/hostdir",
				Type:        mountTypeBind,
			},
		},
	}

	id, _ := guid.NewV4()
	plan9Ctrl.EXPECT().
		Reserve(gomock.Any(), share.Config{
			HostPath: hostDir,
			ReadOnly: false,
		}, plan9Mount.Config{ReadOnly: false}).
		Return(id, nil)
	plan9Ctrl.EXPECT().
		MapToGuest(gomock.Any(), id).
		Return("/mnt/plan9/share0", nil)

	if err := c.allocateMounts(t.Context(), spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Mounts[0].Source != "/mnt/plan9/share0" {
		t.Errorf("mount source = %q, want %q", spec.Mounts[0].Source, "/mnt/plan9/share0")
	}
	if len(c.plan9Resources) != 1 || c.plan9Resources[0] != id {
		t.Errorf("plan9Resources = %v, want [%v]", c.plan9Resources, id)
	}
}

// TestAllocateMounts_Plan9BindReadOnly verifies that the read-only flag
// is propagated to both share and mount configs for Plan9 mounts.
func TestAllocateMounts_Plan9BindReadOnly(t *testing.T) {

	c, _, plan9Ctrl := newMountsTestController(t)
	hostDir := t.TempDir()

	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      hostDir,
				Destination: "/mnt/readonly",
				Type:        mountTypeBind,
				Options:     []string{"ro"},
			},
		},
	}

	id, _ := guid.NewV4()
	plan9Ctrl.EXPECT().
		Reserve(gomock.Any(), share.Config{
			HostPath: hostDir,
			ReadOnly: true,
		}, plan9Mount.Config{ReadOnly: true}).
		Return(id, nil)
	plan9Ctrl.EXPECT().
		MapToGuest(gomock.Any(), id).
		Return("/mnt/plan9/share0", nil)

	if err := c.allocateMounts(t.Context(), spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestAllocateMounts_Plan9BindSingleFile verifies that a single-file bind mount
// shares the parent directory with restrict mode and the file in AllowedNames.
func TestAllocateMounts_Plan9BindSingleFile(t *testing.T) {

	c, _, plan9Ctrl := newMountsTestController(t)

	// Create a real temp file so os.Stat succeeds and reports !IsDir().
	dir := t.TempDir()
	filePath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(filePath, []byte("{}"), 0644); err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      filePath,
				Destination: "/etc/config.json",
				Type:        mountTypeBind,
			},
		},
	}

	id, _ := guid.NewV4()

	// For a single-file mount, the share's HostPath should be the parent
	// directory with Restrict enabled and the filename in AllowedNames.
	plan9Ctrl.EXPECT().
		Reserve(gomock.Any(), share.Config{
			HostPath:     dir + string(filepath.Separator),
			ReadOnly:     false,
			Restrict:     true,
			AllowedNames: []string{"config.json"},
		}, plan9Mount.Config{ReadOnly: false}).
		Return(id, nil)
	plan9Ctrl.EXPECT().
		MapToGuest(gomock.Any(), id).
		Return("/mnt/plan9/file0", nil)

	if err := c.allocateMounts(t.Context(), spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Mounts[0].Source != "/mnt/plan9/file0" {
		t.Errorf("mount source = %q, want %q", spec.Mounts[0].Source, "/mnt/plan9/file0")
	}
}

// TestAllocateMounts_Plan9ReserveFailure verifies that a Plan9 Reserve failure
// propagates the error.
func TestAllocateMounts_Plan9ReserveFailure(t *testing.T) {

	c, _, plan9Ctrl := newMountsTestController(t)
	hostDir := t.TempDir()

	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      hostDir,
				Destination: "/mnt/fail",
				Type:        mountTypeBind,
			},
		},
	}

	plan9Ctrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(guid.GUID{}, errPlan9Reserve)

	err := c.allocateMounts(t.Context(), spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errPlan9Reserve) {
		t.Errorf("error = %v, want wrapping %v", err, errPlan9Reserve)
	}
}

// TestAllocateMounts_Plan9MapToGuestFailure verifies that a Plan9 MapToGuest
// failure propagates the error.
func TestAllocateMounts_Plan9MapToGuestFailure(t *testing.T) {

	c, _, plan9Ctrl := newMountsTestController(t)
	hostDir := t.TempDir()

	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      hostDir,
				Destination: "/mnt/fail",
				Type:        mountTypeBind,
			},
		},
	}

	id, _ := guid.NewV4()
	plan9Ctrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(id, nil)
	plan9Ctrl.EXPECT().
		MapToGuest(gomock.Any(), id).
		Return("", errPlan9Map)

	err := c.allocateMounts(t.Context(), spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errPlan9Map) {
		t.Errorf("error = %v, want wrapping %v", err, errPlan9Map)
	}
}

// TestAllocateMounts_Plan9StatFailure verifies that allocateMounts returns an
// error when os.Stat fails for a bind mount source.
func TestAllocateMounts_Plan9StatFailure(t *testing.T) {

	c, _, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      `C:\nonexistent\path\does\not\exist`,
				Destination: "/mnt/missing",
				Type:        mountTypeBind,
			},
		},
	}

	err := c.allocateMounts(t.Context(), spec)
	if err == nil {
		t.Fatal("expected error for stat of nonexistent path")
	}
}

// --- allocateMounts: skip / passthrough tests ---

// TestAllocateMounts_HugePagesSkipped verifies that hugepages mounts are
// validated but skipped (no Plan9 or SCSI calls).
func TestAllocateMounts_HugePagesSkipped(t *testing.T) {

	c, _, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      guestpath.HugePagesMountPrefix + "2M/hugepages0",
				Destination: "/dev/hugepages",
				Type:        mountTypeBind,
			},
		},
	}

	// No SCSI or Plan9 calls expected.
	if err := c.allocateMounts(t.Context(), spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestAllocateMounts_HugePagesInvalidSize verifies that only 2M hugepages are
// accepted.
func TestAllocateMounts_HugePagesInvalidSize(t *testing.T) {

	c, _, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      guestpath.HugePagesMountPrefix + "1G/hugepages0",
				Destination: "/dev/hugepages",
				Type:        mountTypeBind,
			},
		},
	}

	err := c.allocateMounts(t.Context(), spec)
	if err == nil {
		t.Fatal("expected error for unsupported hugepage size")
	}
}

// TestAllocateMounts_HugePagesInvalidFormat verifies that an improperly
// formatted hugepages path is rejected.
func TestAllocateMounts_HugePagesInvalidFormat(t *testing.T) {

	c, _, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      guestpath.HugePagesMountPrefix + "bad",
				Destination: "/dev/hugepages",
				Type:        mountTypeBind,
			},
		},
	}

	err := c.allocateMounts(t.Context(), spec)
	if err == nil {
		t.Fatal("expected error for invalid hugepages format")
	}
}

// TestAllocateMounts_GuestInternalPathsSkipped verifies that sandbox://, sandbox-tmp://,
// and uvm:// prefixed mounts pass through without host-side allocation.
func TestAllocateMounts_GuestInternalPathsSkipped(t *testing.T) {

	tests := []struct {
		name   string
		source string
	}{
		{name: "sandbox", source: guestpath.SandboxMountPrefix + "/some/path"},
		{name: "sandbox-tmp", source: guestpath.SandboxTmpfsMountPrefix + "/tmp/path"},
		{name: "uvm", source: guestpath.UVMMountPrefix + "/uvm/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _, _ := newMountsTestController(t)
			spec := &specs.Spec{
				Mounts: []specs.Mount{
					{
						Source:      tt.source,
						Destination: "/mnt/guest",
						Type:        mountTypeBind,
					},
				},
			}

			// No SCSI or Plan9 calls expected.
			if err := c.allocateMounts(t.Context(), spec); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Source should remain unchanged.
			if spec.Mounts[0].Source != tt.source {
				t.Errorf("source = %q, want %q", spec.Mounts[0].Source, tt.source)
			}
		})
	}
}

// TestAllocateMounts_UnknownTypesPassThrough verifies that unknown mount types
// (e.g. tmpfs, proc) are passed through without error or host-side allocation.
func TestAllocateMounts_UnknownTypesPassThrough(t *testing.T) {

	tests := []struct {
		name      string
		mountType string
	}{
		{name: "tmpfs", mountType: "tmpfs"},
		{name: "proc", mountType: "proc"},
		{name: "devpts", mountType: "devpts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _, _ := newMountsTestController(t)
			spec := &specs.Spec{
				Mounts: []specs.Mount{
					{
						Source:      "none",
						Destination: "/mnt/" + tt.mountType,
						Type:        tt.mountType,
					},
				},
			}

			if err := c.allocateMounts(t.Context(), spec); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestAllocateMounts_NoMounts verifies that allocateMounts succeeds when the
// spec contains no mounts.
func TestAllocateMounts_NoMounts(t *testing.T) {

	c, _, _ := newMountsTestController(t)
	spec := &specs.Spec{}

	if err := c.allocateMounts(t.Context(), spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.scsiResources) != 0 {
		t.Errorf("expected 0 scsi resources, got %d", len(c.scsiResources))
	}
	if len(c.plan9Resources) != 0 {
		t.Errorf("expected 0 plan9 resources, got %d", len(c.plan9Resources))
	}
}

// TestAllocateMounts_EmptySourceOrDestination verifies that a mount with an
// empty source or destination returns an error.
func TestAllocateMounts_EmptySourceOrDestination(t *testing.T) {

	tests := []struct {
		name string
		src  string
		dst  string
	}{
		{name: "empty-source", src: "", dst: "/mnt/data"},
		{name: "empty-destination", src: `C:\disks\data.vhdx`, dst: ""},
		{name: "both-empty", src: "", dst: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _, _ := newMountsTestController(t)
			spec := &specs.Spec{
				Mounts: []specs.Mount{
					{
						Source:      tt.src,
						Destination: tt.dst,
						Type:        mountTypeVirtualDisk,
					},
				},
			}

			if err := c.allocateMounts(t.Context(), spec); err == nil {
				t.Fatal("expected error for empty source or destination")
			}
		})
	}
}

// TestAllocateMounts_MultipleMixed verifies that allocateMounts correctly
// handles a mix of SCSI, Plan9, and passthrough mounts in a single spec.
func TestAllocateMounts_MultipleMixed(t *testing.T) {
	stubResolvePath(t)
	stubGrantVMAccess(t)

	c, scsiCtrl, plan9Ctrl := newMountsTestController(t)
	hostDir := t.TempDir()

	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      `C:\disks\data.vhdx`,
				Destination: "/mnt/data",
				Type:        mountTypeVirtualDisk,
			},
			{
				Source:      hostDir,
				Destination: "/mnt/hostdir",
				Type:        mountTypeBind,
			},
			{
				Source:      "none",
				Destination: "/proc",
				Type:        "proc",
			},
			{
				Source:      guestpath.SandboxMountPrefix + "/sandbox/dir",
				Destination: "/mnt/sandbox",
				Type:        mountTypeBind,
			},
		},
	}

	scsiID, _ := guid.NewV4()
	plan9ID, _ := guid.NewV4()

	// SCSI mount for virtual disk.
	scsiCtrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(scsiID, nil)
	scsiCtrl.EXPECT().
		MapToGuest(gomock.Any(), scsiID).
		Return("/dev/sda", nil)

	// Plan9 mount for bind directory.
	plan9Ctrl.EXPECT().
		Reserve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(plan9ID, nil)
	plan9Ctrl.EXPECT().
		MapToGuest(gomock.Any(), plan9ID).
		Return("/mnt/plan9/share0", nil)

	if err := c.allocateMounts(t.Context(), spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.scsiResources) != 1 {
		t.Errorf("expected 1 scsi resource, got %d", len(c.scsiResources))
	}
	if len(c.plan9Resources) != 1 {
		t.Errorf("expected 1 plan9 resource, got %d", len(c.plan9Resources))
	}
	// SCSI mount rewritten.
	if spec.Mounts[0].Source != "/dev/sda" {
		t.Errorf("SCSI mount source = %q, want %q", spec.Mounts[0].Source, "/dev/sda")
	}
	// Plan9 mount rewritten.
	if spec.Mounts[1].Source != "/mnt/plan9/share0" {
		t.Errorf("Plan9 mount source = %q, want %q", spec.Mounts[1].Source, "/mnt/plan9/share0")
	}
	// Proc mount unchanged.
	if spec.Mounts[2].Source != "none" {
		t.Errorf("proc mount source = %q, want %q", spec.Mounts[2].Source, "none")
	}
	// Sandbox mount unchanged.
	if spec.Mounts[3].Source != guestpath.SandboxMountPrefix+"/sandbox/dir" {
		t.Errorf("sandbox mount source changed unexpectedly")
	}
}

// --- Helper function tests ---

// TestParseExtensibleVirtualDiskPath verifies parsing of EVD paths.
func TestParseExtensibleVirtualDiskPath(t *testing.T) {

	tests := []struct {
		name     string
		input    string
		wantType string
		wantPath string
		wantErr  bool
	}{
		{
			name:     "valid",
			input:    `evd://mytype/C:\disks\data.vhdx`,
			wantType: "mytype",
			wantPath: `C:\disks\data.vhdx`,
		},
		{
			name:     "valid-nested",
			input:    "evd://provider/some/nested/path",
			wantType: "provider",
			wantPath: "some/nested/path",
		},
		{
			name:    "missing-prefix",
			input:   "/no/evd/prefix",
			wantErr: true,
		},
		{
			name:    "no-type-separator",
			input:   "evd://",
			wantErr: true,
		},
		{
			name:    "empty-type",
			input:   "evd:///path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			evdType, sourcePath, err := parseExtensibleVirtualDiskPath(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if evdType != tt.wantType {
				t.Errorf("evdType = %q, want %q", evdType, tt.wantType)
			}
			if sourcePath != tt.wantPath {
				t.Errorf("sourcePath = %q, want %q", sourcePath, tt.wantPath)
			}
		})
	}
}

// TestValidateHugePageMount verifies the hugepages mount source validation.
func TestValidateHugePageMount(t *testing.T) {

	tests := []struct {
		name    string
		source  string
		wantErr bool
	}{
		{
			name:   "valid-2M",
			source: guestpath.HugePagesMountPrefix + "2M/hugepages0",
		},
		{
			name:    "unsupported-1G",
			source:  guestpath.HugePagesMountPrefix + "1G/hugepages0",
			wantErr: true,
		},
		{
			name:    "missing-location",
			source:  guestpath.HugePagesMountPrefix + "2M",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateHugePageMount(tt.source)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestIsReadOnlyMount verifies read-only flag detection from mount options.
func TestIsReadOnlyMount(t *testing.T) {

	tests := []struct {
		name    string
		options []string
		want    bool
	}{
		{name: "empty-options", options: nil, want: false},
		{name: "rw-only", options: []string{"rw"}, want: false},
		{name: "ro", options: []string{"ro"}, want: true},
		{name: "RO-case-insensitive", options: []string{"RO"}, want: true},
		{name: "ro-among-others", options: []string{"noatime", "ro", "nosuid"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mount := &specs.Mount{Options: tt.options}
			if got := isReadOnlyMount(mount); got != tt.want {
				t.Errorf("isReadOnlyMount = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsGuestInternalPath verifies detection of guest-internal path prefixes.
func TestIsGuestInternalPath(t *testing.T) {

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "sandbox", path: guestpath.SandboxMountPrefix + "/some/path", want: true},
		{name: "sandbox-tmp", path: guestpath.SandboxTmpfsMountPrefix + "/tmp/path", want: true},
		{name: "uvm", path: guestpath.UVMMountPrefix + "/uvm/path", want: true},
		{name: "host-path", path: `C:\some\host\path`, want: false},
		{name: "linux-path", path: "/some/linux/path", want: false},
		{name: "hugepages", path: guestpath.HugePagesMountPrefix + "2M/hp", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isGuestInternalPath(tt.path); got != tt.want {
				t.Errorf("isGuestInternalPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestAllocateMounts_EVDInvalidPath verifies that an invalid EVD source path
// returns an error before any SCSI reservation.
func TestAllocateMounts_EVDInvalidPath(t *testing.T) {

	c, _, _ := newMountsTestController(t)
	spec := &specs.Spec{
		Mounts: []specs.Mount{
			{
				Source:      "not-evd://missing",
				Destination: "/mnt/bad",
				Type:        mountTypeExtensibleVirtualDisk,
			},
		},
	}

	err := c.allocateMounts(t.Context(), spec)
	if err == nil {
		t.Fatal("expected error for invalid EVD path")
	}
}
