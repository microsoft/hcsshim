//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"fmt"
	"path"
	"testing"

	ctrdoci "github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/pkg/annotations"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	testcontainer "github.com/Microsoft/hcsshim/test/internal/container"
	testlayers "github.com/Microsoft/hcsshim/test/internal/layers"
	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func TestLCOW_LogPath(t *testing.T) {
	requireFeatures(t, featureUVM, featureContainer, featureLCOW)
	require.Build(t, osversion.RS5)

	ctx := util.Context(namespacedContext(context.Background()), t)

	ls := linuxImageLayers(ctx, t)
	cache := testlayers.CacheFile(ctx, t, "")

	const (
		helloWorld = "hello world"
		teeInput   = `please copy me
this is a line on a new line.
look at these characters: sdfasd09fc-32r42	3;.er "k🧪3112=3-🧪po4\asdfpas9difasdck s
another new line, with more letters`
	)

	t.Run("tee", func(t *testing.T) {
		const want = helloWorld + "\n" + teeInput

		opts := defaultLCOWOptions(ctx, t)
		vm := testuvm.CreateAndStart(ctx, t, opts)

		cID := testName(t, "container")

		logFile := path.Join("/run/dira/b/clogs/", util.CleanName(t))

		scratch, _ := testlayers.ScratchSpace(ctx, t, vm, "", "", cache)
		spec := testoci.CreateLinuxSpec(ctx, t, cID,
			testoci.DefaultLinuxSpecOpts(cID,
				ctrdoci.WithProcessArgs("/bin/sh", "-c", fmt.Sprintf("echo %s; tee", helloWorld)),
				ctrdoci.WithAnnotations(map[string]string{annotations.LCOWTeeLogPath: logFile}),
				testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

		c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cID, hcsOwner)
		t.Cleanup(cleanup)

		io := testcmd.NewBufferedIOFromString(teeInput)
		init := testcontainer.Start(ctx, t, c, io)
		t.Cleanup(func() {
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		testcmd.WaitExitCode(ctx, t, init, 0)

		// validate stdout
		io.TestOutput(t, want, nil)

		logIO := testcmd.NewBufferedIO()
		cmdArgs := testcmd.Create(ctx, t, vm, &specs.Process{Args: []string{"cat", logFile}}, logIO)
		testcmd.Start(ctx, t, cmdArgs)
		testcmd.WaitExitCode(ctx, t, cmdArgs, 0)

		// validate the log file
		logIO.TestOutput(t, want, nil)
	})

	t.Run("exec", func(t *testing.T) {
		const (
			want     = helloWorld + "\n" + helloWorld
			execWant = helloWorld + "\n" + teeInput
		)

		opts := defaultLCOWOptions(ctx, t)
		vm := testuvm.CreateAndStart(ctx, t, opts)

		cID := testName(t, "container")

		logFile := path.Join("/run/clogs/", util.CleanName(t))

		scratch, _ := testlayers.ScratchSpace(ctx, t, vm, "", "", cache)
		spec := testoci.CreateLinuxSpec(ctx, t, cID,
			testoci.DefaultLinuxSpecOpts(cID,
				ctrdoci.WithProcessArgs("/bin/sh", "-c", fmt.Sprintf("echo %s; sleep 10s; echo %s", helloWorld, helloWorld)),
				ctrdoci.WithAnnotations(map[string]string{annotations.LCOWTeeLogPath: logFile}),
				testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

		c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cID, hcsOwner)
		t.Cleanup(cleanup)

		io := testcmd.NewBufferedIO()
		init := testcontainer.Start(ctx, t, c, io)
		t.Cleanup(func() {
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		ps := testoci.CreateLinuxSpec(ctx, t, cID,
			testoci.DefaultLinuxSpecOpts(cID,
				ctrdoci.WithDefaultPathEnv,
				ctrdoci.WithProcessArgs("/bin/sh", "-c", fmt.Sprintf("echo %s; tee", helloWorld)),
			)...,
		).Process
		execIO := testcmd.NewBufferedIOFromString(teeInput)
		execCmd := testcmd.Create(ctx, t, c, ps, execIO)
		testcmd.Start(ctx, t, execCmd)

		testcmd.WaitExitCode(ctx, t, execCmd, 0)
		testcmd.WaitExitCode(ctx, t, init, 0)

		// validate stdout
		execIO.TestOutput(t, execWant, nil)
		io.TestOutput(t, want, nil)

		logIO := testcmd.NewBufferedIO()
		cmdArgs := testcmd.Create(ctx, t, vm, &specs.Process{Args: []string{"cat", logFile}}, logIO)
		testcmd.Start(ctx, t, cmdArgs)
		testcmd.WaitExitCode(ctx, t, cmdArgs, 0)

		// validate the log file
		logIO.TestOutput(t, want, nil)
	})
}
