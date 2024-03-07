//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"strings"
	"testing"

	ctrdoci "github.com/containerd/containerd/oci"

	"github.com/Microsoft/hcsshim/osversion"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	testcontainer "github.com/Microsoft/hcsshim/test/internal/container"
	testlayers "github.com/Microsoft/hcsshim/test/internal/layers"
	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func TestLCOW_ContainerLifecycle(t *testing.T) {
	requireFeatures(t, featureLCOW, featureUVM, featureContainer)
	require.Build(t, osversion.RS5)

	ctx := util.Context(namespacedContext(context.Background()), t)
	ls := linuxImageLayers(ctx, t)
	opts := defaultLCOWOptions(ctx, t)
	opts.ID += util.RandNameSuffix()
	vm := testuvm.CreateAndStartLCOWFromOpts(ctx, t, opts)

	scratch, _ := testlayers.ScratchSpace(ctx, t, vm, "", "", "")

	spec := testoci.CreateLinuxSpec(ctx, t, t.Name()+util.RandNameSuffix(),
		testoci.DefaultLinuxSpecOpts("",
			ctrdoci.WithProcessArgs("/bin/sh", "-c", testoci.TailNullArgs),
			testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

	c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, t.Name(), hcsOwner)
	t.Cleanup(cleanup)

	init := testcontainer.Start(ctx, t, c, nil)
	t.Cleanup(func() {
		testcontainer.Kill(ctx, t, c)
		testcontainer.Wait(ctx, t, c)
	})
	testcmd.Kill(ctx, t, init)
	testcmd.WaitExitCode(ctx, t, init, testcmd.ForcedKilledExitCode)
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
	requireFeatures(t, featureLCOW, featureUVM, featureContainer)
	require.Build(t, osversion.RS5)

	ctx := util.Context(namespacedContext(context.Background()), t)
	ls := linuxImageLayers(ctx, t)
	opts := defaultLCOWOptions(ctx, t)
	opts.ID += util.RandNameSuffix()
	cache := testlayers.CacheFile(ctx, t, "")
	vm := testuvm.CreateAndStartLCOWFromOpts(ctx, t, opts)

	for _, tt := range ioTests {
		t.Run(tt.name, func(t *testing.T) {
			id := strings.ReplaceAll(t.Name(), "/", "") + util.RandNameSuffix()
			scratch, _ := testlayers.ScratchSpace(ctx, t, vm, "", "", cache)
			spec := testoci.CreateLinuxSpec(ctx, t, id,
				testoci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs(tt.args...),
					testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, id, hcsOwner)
			t.Cleanup(cleanup)

			io := testcmd.NewBufferedIO()
			if tt.in != "" {
				io = testcmd.NewBufferedIOFromString(tt.in)
			}
			init := testcontainer.Start(ctx, t, c, io)

			t.Cleanup(func() {
				testcontainer.Kill(ctx, t, c)
				testcontainer.Wait(ctx, t, c)
			})

			if e := testcmd.Wait(ctx, t, init); e != 0 {
				t.Fatalf("got exit code %d, wanted %d", e, 0)
			}

			io.TestOutput(t, tt.want, nil)
		})
	}
}

func TestLCOW_ContainerExec(t *testing.T) {
	requireFeatures(t, featureLCOW, featureUVM, featureContainer)
	require.Build(t, osversion.RS5)

	ctx := util.Context(namespacedContext(context.Background()), t)
	ls := linuxImageLayers(ctx, t)
	opts := defaultLCOWOptions(ctx, t)
	opts.ID += util.RandNameSuffix()
	vm := testuvm.CreateAndStartLCOWFromOpts(ctx, t, opts)

	id := strings.ReplaceAll(t.Name(), "/", "") + util.RandNameSuffix()
	scratch, _ := testlayers.ScratchSpace(ctx, t, vm, "", "", "")
	spec := testoci.CreateLinuxSpec(ctx, t, id,
		testoci.DefaultLinuxSpecOpts(id,
			ctrdoci.WithProcessArgs("/bin/sh", "-c", testoci.TailNullArgs),
			testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

	c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, id, hcsOwner)
	t.Cleanup(cleanup)
	init := testcontainer.Start(ctx, t, c, nil)
	t.Cleanup(func() {
		testcmd.Kill(ctx, t, init)
		testcmd.Wait(ctx, t, init)
		testcontainer.Kill(ctx, t, c)
		testcontainer.Wait(ctx, t, c)
	})

	for _, tt := range ioTests {
		t.Run(tt.name, func(t *testing.T) {
			ps := testoci.CreateLinuxSpec(ctx, t, id,
				testoci.DefaultLinuxSpecOpts(id,
					// oci.WithTTY,
					ctrdoci.WithDefaultPathEnv,
					ctrdoci.WithProcessArgs(tt.args...))...,
			).Process
			io := testcmd.NewBufferedIO()
			if tt.in != "" {
				io = testcmd.NewBufferedIOFromString(tt.in)
			}
			p := testcmd.Create(ctx, t, c, ps, io)
			testcmd.Start(ctx, t, p)

			if e := testcmd.Wait(ctx, t, p); e != 0 {
				t.Fatalf("got exit code %d, wanted %d", e, 0)
			}

			io.TestOutput(t, tt.want, nil)
		})
	}
}
