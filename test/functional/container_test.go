//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"fmt"
	"testing"

	ctrdoci "github.com/containerd/containerd/oci"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/jobcontainers"
	"github.com/Microsoft/hcsshim/osversion"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	testcontainer "github.com/Microsoft/hcsshim/test/internal/container"
	testlayers "github.com/Microsoft/hcsshim/test/internal/layers"
	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func TestContainerLifecycle(t *testing.T) {
	requireFeatures(t, featureContainer)
	requireAnyFeature(t, featureUVM, featureLCOW, featureWCOW, featureHostProcess)
	require.Build(t, osversion.RS5)

	ctx := util.Context(namespacedContext(context.Background()), t)

	t.Run("LCOW", func(t *testing.T) {
		requireFeatures(t, featureLCOW, featureUVM)

		ls := linuxImageLayers(ctx, t)
		vm := testuvm.CreateAndStart(ctx, t, defaultLCOWOptions(ctx, t))

		scratch, _ := testlayers.ScratchSpace(ctx, t, vm, "", "", "")
		cID := vm.ID() + "-container"
		spec := testoci.CreateLinuxSpec(ctx, t, cID,
			testoci.DefaultLinuxSpecOpts(cID,
				ctrdoci.WithProcessArgs("/bin/sh", "-c", testoci.TailNullArgs),
				testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

		c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cID, hcsOwner)
		t.Cleanup(cleanup)

		init := testcontainer.Start(ctx, t, c, nil)
		t.Cleanup(func() {
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		testcmd.Kill(ctx, t, init)
		testcmd.WaitExitCode(ctx, t, init, testcmd.ForcedKilledExitCode)
	}) // LCOW

	t.Run("WCOW Hyper-V", func(t *testing.T) {
		requireFeatures(t, featureWCOW, featureUVM)

		ls := windowsImageLayers(ctx, t)
		vm := testuvm.CreateAndStart(ctx, t, defaultWCOWOptions(ctx, t))

		cID := vm.ID() + "-container"
		scratch := testlayers.WCOWScratchDir(ctx, t, "")
		spec := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
				testoci.WithWindowsLayerFolders(append(ls, scratch)),
			)...)

		c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cID, hcsOwner)
		t.Cleanup(cleanup)

		init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, nil)
		t.Cleanup(func() {
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		testcmd.Kill(ctx, t, init)
		testcmd.WaitExitCode(ctx, t, init, int(windows.ERROR_PROCESS_ABORTED))
	}) // WCOW Hyper-V

	t.Run("WCOW Process", func(t *testing.T) {
		requireFeatures(t, featureWCOW)

		cID := testName(t, "container")
		scratch := testlayers.WCOWScratchDir(ctx, t, "")
		spec := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
				testoci.WithWindowsLayerFolders(append(windowsImageLayers(ctx, t), scratch)),
			)...)

		c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
		t.Cleanup(cleanup)

		init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, nil)
		t.Cleanup(func() {
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		testcmd.Kill(ctx, t, init)
		testcmd.WaitExitCode(ctx, t, init, int(windows.ERROR_PROCESS_ABORTED))
	}) // WCOW Process

	t.Run("WCOW HostProcess", func(t *testing.T) {
		requireFeatures(t, featureWCOW, featureHostProcess)

		cID := testName(t, "container")
		scratch := testlayers.WCOWScratchDir(ctx, t, "")
		spec := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
				testoci.WithWindowsLayerFolders(append(windowsImageLayers(ctx, t), scratch)),
				testoci.AsHostProcessContainer(),
				testoci.HostProcessInheritUser(),
			)...)

		c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
		t.Cleanup(cleanup)

		if _, ok := c.(*jobcontainers.JobContainer); !ok {
			t.Fatalf("expected type JobContainer; got %T", c)
		}

		init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, nil)
		t.Cleanup(func() {
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		testcmd.Kill(ctx, t, init)
		testcmd.WaitExitCode(ctx, t, init, 1)
	}) // WCOW HostProcess
}

var ioTests = []struct {
	name     string
	lcowArgs []string
	wcowCmd  string
	in       string
	want     string
}{
	{
		name:     "true",
		lcowArgs: []string{"/bin/sh", "-c", "true"},
		wcowCmd:  "cmd /c (exit 0)",
		want:     "",
	},
	{
		name:     "echo",
		lcowArgs: []string{"/bin/sh", "-c", `echo -n "hi y'all"`},
		wcowCmd:  `cmd /c echo hi y'all`,
		want:     "hi y'all",
	},
	{
		name:     "tee",
		lcowArgs: []string{"/bin/sh", "-c", "tee"},
		wcowCmd:  "", // TODO: figure out cmd.exe equivalent
		in:       "are you copying me?",
		want:     "are you copying me?",
	},
}

func TestContainerIO(t *testing.T) {
	requireFeatures(t, featureContainer)
	requireAnyFeature(t, featureUVM, featureLCOW, featureWCOW, featureHostProcess)
	require.Build(t, osversion.RS5)

	ctx := util.Context(namespacedContext(context.Background()), t)

	t.Run("LCOW", func(t *testing.T) {
		requireFeatures(t, featureLCOW, featureUVM)

		opts := defaultLCOWOptions(ctx, t)
		vm := testuvm.CreateAndStart(ctx, t, opts)

		ls := linuxImageLayers(ctx, t)
		cache := testlayers.CacheFile(ctx, t, "")

		for _, tt := range ioTests {
			if len(tt.lcowArgs) == 0 {
				continue
			}

			t.Run(tt.name, func(t *testing.T) {
				cID := testName(t, "container")

				scratch, _ := testlayers.ScratchSpace(ctx, t, vm, "", "", cache)
				spec := testoci.CreateLinuxSpec(ctx, t, cID,
					testoci.DefaultLinuxSpecOpts(cID,
						ctrdoci.WithProcessArgs(tt.lcowArgs...),
						testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

				c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cID, hcsOwner)
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

				testcmd.WaitExitCode(ctx, t, init, 0)
				io.TestOutput(t, tt.want, nil)
			})
		}
	}) // LCOW

	t.Run("WCOW Hyper-V", func(t *testing.T) {
		requireFeatures(t, featureWCOW, featureUVM)

		ls := windowsImageLayers(ctx, t)
		vm := testuvm.CreateAndStart(ctx, t, defaultWCOWOptions(ctx, t))

		for _, tt := range ioTests {
			if tt.wcowCmd == "" {
				continue
			}

			t.Run(tt.name, func(t *testing.T) {
				cID := vm.ID() + "-container"
				scratch := testlayers.WCOWScratchDir(ctx, t, "")
				spec := testoci.CreateWindowsSpec(ctx, t, cID,
					testoci.DefaultWindowsSpecOpts(cID,
						ctrdoci.WithProcessCommandLine(tt.wcowCmd),
						testoci.WithWindowsLayerFolders(append(ls, scratch)),
					)...)

				c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cID, hcsOwner)
				t.Cleanup(cleanup)

				io := testcmd.NewBufferedIO()
				if tt.in != "" {
					io = testcmd.NewBufferedIOFromString(tt.in)
				}
				init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, io)

				t.Cleanup(func() {
					testcontainer.Kill(ctx, t, c)
					testcontainer.Wait(ctx, t, c)
				})

				testcmd.WaitExitCode(ctx, t, init, 0)
				io.TestOutput(t, tt.want, nil)
			})
		}
	}) // WCOW Hyper-V

	t.Run("WCOW Process", func(t *testing.T) {
		requireFeatures(t, featureWCOW)

		ls := windowsImageLayers(ctx, t)

		for _, tt := range ioTests {
			if tt.wcowCmd == "" {
				continue
			}

			t.Run(tt.name, func(t *testing.T) {
				cID := testName(t, "container")
				scratch := testlayers.WCOWScratchDir(ctx, t, "")
				spec := testoci.CreateWindowsSpec(ctx, t, cID,
					testoci.DefaultWindowsSpecOpts(cID,
						ctrdoci.WithProcessCommandLine(tt.wcowCmd),
						testoci.WithWindowsLayerFolders(append(ls, scratch)),
					)...)

				c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
				t.Cleanup(cleanup)

				io := testcmd.NewBufferedIO()
				if tt.in != "" {
					io = testcmd.NewBufferedIOFromString(tt.in)
				}
				init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, io)
				t.Cleanup(func() {
					testcontainer.Kill(ctx, t, c)
					testcontainer.Wait(ctx, t, c)
				})

				testcmd.WaitExitCode(ctx, t, init, 0)
				io.TestOutput(t, tt.want, nil)
			})
		}
	}) // WCOW Process

	t.Run("WCOW HostProcess", func(t *testing.T) {
		requireFeatures(t, featureWCOW, featureHostProcess)

		ls := windowsImageLayers(ctx, t)

		for _, tt := range ioTests {
			if tt.wcowCmd == "" {
				continue
			}

			t.Run(tt.name, func(t *testing.T) {
				cID := testName(t, "container")
				scratch := testlayers.WCOWScratchDir(ctx, t, "")
				spec := testoci.CreateWindowsSpec(ctx, t, cID,
					testoci.DefaultWindowsSpecOpts(cID,
						ctrdoci.WithProcessCommandLine(tt.wcowCmd),
						testoci.WithWindowsLayerFolders(append(ls, scratch)),
						testoci.AsHostProcessContainer(),
						testoci.HostProcessInheritUser(),
					)...)

				c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
				t.Cleanup(cleanup)

				io := testcmd.NewBufferedIO()
				if tt.in != "" {
					io = testcmd.NewBufferedIOFromString(tt.in)
				}
				init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, io)
				t.Cleanup(func() {
					testcontainer.Kill(ctx, t, c)
					testcontainer.Wait(ctx, t, c)
				})

				testcmd.WaitExitCode(ctx, t, init, 0)
				io.TestOutput(t, tt.want, nil)
			})
		}
	}) // WCOW HostProcess
}

func TestContainerExec(t *testing.T) {
	requireFeatures(t, featureContainer)
	requireAnyFeature(t, featureUVM, featureLCOW, featureWCOW, featureHostProcess)
	require.Build(t, osversion.RS5)

	ctx := util.Context(namespacedContext(context.Background()), t)

	t.Run("LCOW", func(t *testing.T) {
		requireFeatures(t, featureLCOW, featureUVM)

		opts := defaultLCOWOptions(ctx, t)
		vm := testuvm.CreateAndStart(ctx, t, opts)

		ls := linuxImageLayers(ctx, t)
		scratch, _ := testlayers.ScratchSpace(ctx, t, vm, "", "", "")

		cID := vm.ID() + "-container"
		spec := testoci.CreateLinuxSpec(ctx, t, cID,
			testoci.DefaultLinuxSpecOpts(cID,
				ctrdoci.WithProcessArgs("/bin/sh", "-c", testoci.TailNullArgs),
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

		for _, tt := range ioTests {
			if len(tt.lcowArgs) == 0 {
				continue
			}

			t.Run(tt.name, func(t *testing.T) {
				ps := testoci.CreateLinuxSpec(ctx, t, cID,
					testoci.DefaultLinuxSpecOpts(cID,
						ctrdoci.WithDefaultPathEnv,
						ctrdoci.WithProcessArgs(tt.lcowArgs...))...,
				).Process
				io := testcmd.NewBufferedIO()
				if tt.in != "" {
					io = testcmd.NewBufferedIOFromString(tt.in)
				}
				p := testcmd.Create(ctx, t, c, ps, io)
				testcmd.Start(ctx, t, p)

				testcmd.WaitExitCode(ctx, t, p, 0)
				io.TestOutput(t, tt.want, nil)
			})
		}
	}) // LCOW

	t.Run("WCOW Hyper-V", func(t *testing.T) {
		requireFeatures(t, featureWCOW, featureUVM)

		ls := windowsImageLayers(ctx, t)
		vm := testuvm.CreateAndStart(ctx, t, defaultWCOWOptions(ctx, t))

		cID := vm.ID() + "-container"
		scratch := testlayers.WCOWScratchDir(ctx, t, "")
		spec := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
				testoci.WithWindowsLayerFolders(append(ls, scratch)),
			)...)

		c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cID, hcsOwner)
		t.Cleanup(cleanup)
		init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, nil)
		t.Cleanup(func() {
			testcmd.Kill(ctx, t, init)
			testcmd.Wait(ctx, t, init)
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		for _, tt := range ioTests {
			if tt.wcowCmd == "" {
				continue
			}

			t.Run(tt.name, func(t *testing.T) {
				ps := testoci.CreateWindowsSpec(ctx, t, cID,
					testoci.DefaultWindowsSpecOpts(cID,
						ctrdoci.WithProcessCommandLine(tt.wcowCmd),
					)...).Process

				io := testcmd.NewBufferedIO()
				if tt.in != "" {
					io = testcmd.NewBufferedIOFromString(tt.in)
				}
				p := testcmd.Create(ctx, t, c, ps, io)
				testcmd.Start(ctx, t, p)

				testcmd.WaitExitCode(ctx, t, p, 0)
				io.TestOutput(t, tt.want, nil)
			})
		}
	}) // WCOW Hyper-V

	t.Run("WCOW Process", func(t *testing.T) {
		requireFeatures(t, featureWCOW)

		ls := windowsImageLayers(ctx, t)

		cID := testName(t, "container")
		scratch := testlayers.WCOWScratchDir(ctx, t, "")
		spec := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
				testoci.WithWindowsLayerFolders(append(ls, scratch)),
			)...)

		c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
		t.Cleanup(cleanup)
		init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, nil)
		t.Cleanup(func() {
			testcmd.Kill(ctx, t, init)
			testcmd.Wait(ctx, t, init)
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		for _, tt := range ioTests {
			if tt.wcowCmd == "" {
				continue
			}

			t.Run(tt.name, func(t *testing.T) {
				ps := testoci.CreateWindowsSpec(ctx, t, cID,
					testoci.DefaultWindowsSpecOpts(cID,
						ctrdoci.WithProcessCommandLine(tt.wcowCmd),
					)...).Process

				io := testcmd.NewBufferedIO()
				if tt.in != "" {
					io = testcmd.NewBufferedIOFromString(tt.in)
				}
				p := testcmd.Create(ctx, t, c, ps, io)
				testcmd.Start(ctx, t, p)

				testcmd.WaitExitCode(ctx, t, p, 0)
				io.TestOutput(t, tt.want, nil)
			})
		}
	}) // WCOW Process

	t.Run("WCOW HostProcess", func(t *testing.T) {
		requireFeatures(t, featureWCOW, featureHostProcess)

		ls := windowsImageLayers(ctx, t)

		cID := testName(t, "container")
		scratch := testlayers.WCOWScratchDir(ctx, t, "")
		spec := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
				testoci.WithWindowsLayerFolders(append(ls, scratch)),
				testoci.AsHostProcessContainer(),
				testoci.HostProcessInheritUser(),
			)...)

		c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
		t.Cleanup(cleanup)
		init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, nil)
		t.Cleanup(func() {
			testcmd.Kill(ctx, t, init)
			testcmd.Wait(ctx, t, init)
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		for _, tt := range ioTests {
			if tt.wcowCmd == "" {
				continue
			}

			t.Run(tt.name, func(t *testing.T) {
				ps := testoci.CreateWindowsSpec(ctx, t, cID,
					testoci.DefaultWindowsSpecOpts(cID,
						ctrdoci.WithProcessCommandLine(tt.wcowCmd),
					)...).Process

				io := testcmd.NewBufferedIO()
				if tt.in != "" {
					io = testcmd.NewBufferedIOFromString(tt.in)
				}
				p := testcmd.Create(ctx, t, c, ps, io)
				testcmd.Start(ctx, t, p)

				testcmd.WaitExitCode(ctx, t, p, 0)
				io.TestOutput(t, tt.want, nil)
			})
		}
	}) // WCOW HostProcess
}

func TestContainerExec_DoubleQuotes(t *testing.T) {
	requireFeatures(t, featureContainer, featureWCOW)
	requireAnyFeature(t, featureUVM, featureHostProcess)
	require.Build(t, osversion.RS5)

	ctx := util.Context(namespacedContext(context.Background()), t)

	dir := `C:\hcsshim test temp dir with spaces`
	acl := "CREATOR OWNER:(OI)(CI)(IO)(F)"
	cmdLine := fmt.Sprintf(`cmd /C mkdir "%s" && icacls "%s" /grant "%s" /T && icacls "%s"`, dir, dir, acl, dir)
	t.Logf("command line:\n%s", cmdLine)

	t.Run("WCOW Hyper-V", func(t *testing.T) {
		requireFeatures(t, featureWCOW, featureUVM)

		ls := windowsImageLayers(ctx, t)
		vm := testuvm.CreateAndStart(ctx, t, defaultWCOWOptions(ctx, t))

		cID := vm.ID() + "-container"
		scratch := testlayers.WCOWScratchDir(ctx, t, "")
		spec := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
				testoci.WithWindowsLayerFolders(append(ls, scratch)),
			)...)

		c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cID, hcsOwner)
		t.Cleanup(cleanup)
		init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, nil)
		t.Cleanup(func() {
			testcmd.Kill(ctx, t, init)
			testcmd.Wait(ctx, t, init)
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		ps := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine(cmdLine),
			)...).Process

		io := testcmd.NewBufferedIO()
		p := testcmd.Create(ctx, t, c, ps, io)
		testcmd.Start(ctx, t, p)

		testcmd.WaitExitCode(ctx, t, p, 0)
		io.TestStdOutContains(t, []string{acl}, nil)
	}) // WCOW Hyper-V

	t.Run("WCOW Process", func(t *testing.T) {
		requireFeatures(t, featureWCOW)

		ls := windowsImageLayers(ctx, t)

		cID := testName(t, "container")
		scratch := testlayers.WCOWScratchDir(ctx, t, "")
		spec := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
				testoci.WithWindowsLayerFolders(append(ls, scratch)),
			)...)

		c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
		t.Cleanup(cleanup)
		init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, nil)
		t.Cleanup(func() {
			testcmd.Kill(ctx, t, init)
			testcmd.Wait(ctx, t, init)
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		ps := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine(cmdLine),
			)...).Process

		io := testcmd.NewBufferedIO()
		p := testcmd.Create(ctx, t, c, ps, io)
		testcmd.Start(ctx, t, p)

		testcmd.WaitExitCode(ctx, t, p, 0)
		io.TestStdOutContains(t, []string{acl}, nil)
	}) // WCOW Process

	t.Run("WCOW HostProcess", func(t *testing.T) {
		requireFeatures(t, featureWCOW, featureHostProcess)

		ls := windowsImageLayers(ctx, t)

		// the directory will be created on the host from inside the HPC, so remove it
		// this is mostly to avoid test failures, since `mkdir` errors if the directory already exists
		t.Cleanup(func() { _ = util.RemoveAll(dir) })

		cID := testName(t, "container")
		scratch := testlayers.WCOWScratchDir(ctx, t, "")
		spec := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
				testoci.WithWindowsLayerFolders(append(ls, scratch)),
				testoci.AsHostProcessContainer(),
				testoci.HostProcessInheritUser(),
			)...)

		c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
		t.Cleanup(cleanup)
		init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, nil)
		t.Cleanup(func() {
			testcmd.Kill(ctx, t, init)
			testcmd.Wait(ctx, t, init)
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		ps := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine(cmdLine),
			)...).Process

		io := testcmd.NewBufferedIO()
		p := testcmd.Create(ctx, t, c, ps, io)
		testcmd.Start(ctx, t, p)

		testcmd.WaitExitCode(ctx, t, p, 0)
		io.TestStdOutContains(t, []string{acl}, nil)
	}) // WCOW HostProcess
}
