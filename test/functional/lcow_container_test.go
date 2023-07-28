//go:build windows && functional
// +build windows,functional

package functional

import (
	"strings"
	"testing"

	ctrdoci "github.com/containerd/containerd/oci"

	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/internal/cmd"
	"github.com/Microsoft/hcsshim/test/internal/container"
	"github.com/Microsoft/hcsshim/test/internal/layers"
	"github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	"github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func TestLCOW_ContainerLifecycle(t *testing.T) {
	requireFeatures(t, featureLCOW, featureContainer)
	require.Build(t, osversion.RS5)

	ctx := namespacedContext()
	ls := linuxImageLayers(ctx, t)
	opts := defaultLCOWOptions(t)
	vm := uvm.CreateAndStartLCOWFromOpts(ctx, t, opts)

	scratch, _ := layers.ScratchSpace(ctx, t, vm, "", "", "")

	spec := oci.CreateLinuxSpec(ctx, t, t.Name(),
		oci.DefaultLinuxSpecOpts("",
			ctrdoci.WithProcessArgs("/bin/sh", "-c", oci.TailNullArgs),
			oci.WithWindowsLayerFolders(append(ls, scratch)))...)

	c, _, cleanup := container.Create(ctx, t, vm, spec, t.Name(), hcsOwner)
	t.Cleanup(cleanup)

	init := container.Start(ctx, t, c, nil)
	t.Cleanup(func() {
		container.Kill(ctx, t, c)
		container.Wait(ctx, t, c)
	})
	cmd.Kill(ctx, t, init)
	cmd.WaitExitCode(ctx, t, init, cmd.ForcedKilledExitCode)
}

var ioTests = []struct {
	name string
	args []string
	in   string
	want string
}{
	{
		name: "true",
		args: []string{"/bin/sh", "-c", "true"},
		want: "",
	},
	{
		name: "echo",
		args: []string{"/bin/sh", "-c", `echo -n "hi y'all"`},
		want: "hi y'all",
	},
	{
		name: "tee",
		args: []string{"/bin/sh", "-c", "tee"},
		in:   "are you copying me?",
		want: "are you copying me?",
	},
}

func TestLCOW_ContainerIO(t *testing.T) {
	requireFeatures(t, featureLCOW, featureContainer)
	require.Build(t, osversion.RS5)

	ctx := namespacedContext()
	ls := linuxImageLayers(ctx, t)
	opts := defaultLCOWOptions(t)
	cache := layers.CacheFile(ctx, t, "")
	vm := uvm.CreateAndStartLCOWFromOpts(ctx, t, opts)

	for _, tt := range ioTests {
		t.Run(tt.name, func(t *testing.T) {
			id := strings.ReplaceAll(t.Name(), "/", "")
			scratch, _ := layers.ScratchSpace(ctx, t, vm, "", "", cache)
			spec := oci.CreateLinuxSpec(ctx, t, id,
				oci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs(tt.args...),
					oci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := container.Create(ctx, t, vm, spec, id, hcsOwner)
			t.Cleanup(cleanup)

			io := cmd.NewBufferedIO()
			if tt.in != "" {
				io = cmd.NewBufferedIOFromString(tt.in)
			}
			init := container.Start(ctx, t, c, io)

			t.Cleanup(func() {
				container.Kill(ctx, t, c)
				container.Wait(ctx, t, c)
			})

			if e := cmd.Wait(ctx, t, init); e != 0 {
				t.Fatalf("got exit code %d, wanted %d", e, 0)
			}

			io.TestOutput(t, tt.want, nil)
		})
	}
}

func TestLCOW_ContainerExec(t *testing.T) {
	requireFeatures(t, featureLCOW, featureContainer)
	require.Build(t, osversion.RS5)

	ctx := namespacedContext()
	ls := linuxImageLayers(ctx, t)
	opts := defaultLCOWOptions(t)
	vm := uvm.CreateAndStartLCOWFromOpts(ctx, t, opts)

	id := strings.ReplaceAll(t.Name(), "/", "")
	scratch, _ := layers.ScratchSpace(ctx, t, vm, "", "", "")
	spec := oci.CreateLinuxSpec(ctx, t, id,
		oci.DefaultLinuxSpecOpts(id,
			ctrdoci.WithProcessArgs("/bin/sh", "-c", oci.TailNullArgs),
			oci.WithWindowsLayerFolders(append(ls, scratch)))...)

	c, _, cleanup := container.Create(ctx, t, vm, spec, id, hcsOwner)
	t.Cleanup(cleanup)
	init := container.Start(ctx, t, c, nil)
	t.Cleanup(func() {
		cmd.Kill(ctx, t, init)
		cmd.Wait(ctx, t, init)
		container.Kill(ctx, t, c)
		container.Wait(ctx, t, c)
	})

	for _, tt := range ioTests {
		t.Run(tt.name, func(t *testing.T) {
			ps := oci.CreateLinuxSpec(ctx, t, id,
				oci.DefaultLinuxSpecOpts(id,
					// oci.WithTTY,
					ctrdoci.WithDefaultPathEnv,
					ctrdoci.WithProcessArgs(tt.args...))...,
			).Process
			io := cmd.NewBufferedIO()
			if tt.in != "" {
				io = cmd.NewBufferedIOFromString(tt.in)
			}
			p := cmd.Create(ctx, t, c, ps, io)
			cmd.Start(ctx, t, p)

			if e := cmd.Wait(ctx, t, p); e != 0 {
				t.Fatalf("got exit code %d, wanted %d", e, 0)
			}

			io.TestOutput(t, tt.want, nil)
		})
	}
}
