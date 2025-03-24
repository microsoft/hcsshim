//go:build windows && functional

package functional

import (
	"context"
	"testing"

	ctrdoci "github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/osversion"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	testcontainer "github.com/Microsoft/hcsshim/test/internal/container"
	testlayers "github.com/Microsoft/hcsshim/test/internal/layers"
	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

// TestSandboxMountBacking checks that sandbox mounts are properly backed by the uVM's scratch space.
//
// LCOW only, since:
//   - LCOW uVMs have their rootfs backed by either (1) a ramfs filesystem created from the initrd; or (2)
//     a readonly mount of the rootfs VHD.
//     In either case, the scratch VHD is mounted as `/run/gcs/c/<pod id>/`, and not over the entire rootfs.
//     For WCOW uVMs, however, the scratch VHD is a difference VHD on top of the uVM's entire C: drive,
//     and, since sandbox mounts are under ["internal/hcsoci".wcowSandboxMountPath] ("C:\\SandboxMounts"),
//     they should always be within the sandbox mount.
//   - There isnt a very straight-forward equivalent of `df -h` or `mount` for Windows.
func TestSandboxMountBacking(t *testing.T) {
	requireFeatures(t, featureLCOW, featureUVM, featureContainer)
	require.Build(t, osversion.RS5)

	ctx := util.Context(context.Background(), t)
	opts := defaultLCOWOptions(ctx, t)
	vm := testuvm.CreateAndStart(ctx, t, opts)

	ls := linuxImageLayers(ctx, t)
	scratch, _ := testlayers.ScratchSpace(ctx, t, vm, "", "", "")

	cID := vm.ID() + "-container"
	spec := testoci.CreateLinuxSpec(ctx, t, cID,
		testoci.DefaultLinuxSpecOpts(cID,
			ctrdoci.WithProcessArgs("/bin/sh", "-c", testoci.TailNullArgs),
			ctrdoci.WithMounts([]specs.Mount{
				{
					Destination: "sandbox/mount",
					Type:        "bind",
					Source:      "sandbox://container1/mount/dir",
					Options:     []string{"rw", "rbind", "rshared"},
				},
			}),
			testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

	c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cID, hcsOwner)
	t.Cleanup(cleanup)
	init := testcontainer.Start(ctx, t, c, nil)
	t.Cleanup(func() {
		testcmd.Kill(ctx, t, init)
		testcmd.Wait(ctx, t, init)
		testcontainer.Kill(ctx, t, c)
		testcontainer.Wait(ctx, t, c)
	})

}

func TestSandboxMountTraversal(t *testing.T) {
	requireFeatures(t, featureUVM, featureContainer)
	requireAnyFeature(t, featureLCOW, featureWCOW)
	require.Build(t, osversion.RS5)

	// TODO: sandbox mount with traversal (e.g., C:\path or ..\path or )
}
