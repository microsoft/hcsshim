//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	ctrdoci "github.com/containerd/containerd/oci"
	criutil "github.com/containerd/containerd/pkg/cri/util"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	testcontainer "github.com/Microsoft/hcsshim/test/internal/container"
	testlayers "github.com/Microsoft/hcsshim/test/internal/layers"
	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func BenchmarkWCOW_Container(b *testing.B) {
	requireFeatures(b, featureWCOW)
	requireAnyFeature(b, featureContainer, featureUVM, featureHostProcess)
	require.Build(b, osversion.RS5)

	pCtx := util.Context(namespacedContext(context.Background()), b)

	for _, tc := range []struct {
		name         string
		createUVM    bool
		extraOpts    []ctrdoci.SpecOpts
		killExitCode int
		features     []string
	}{
		{
			name:         "Hyper-V",
			createUVM:    true,
			killExitCode: int(windows.ERROR_PROCESS_ABORTED),
			features:     []string{featureUVM},
		},
		{
			name:         "Process",
			killExitCode: int(windows.ERROR_PROCESS_ABORTED),
		},
		{
			name: "HostProcess",
			extraOpts: []ctrdoci.SpecOpts{
				testoci.AsHostProcessContainer(),
				testoci.HostProcessInheritUser(),
			},
			// HostProcess containers prepend a `cmd /c` to the command, and killing that returns exit code 1
			killExitCode: 1,
			features:     []string{featureHostProcess},
		},
	} {
		b.Run(tc.name, func(b *testing.B) {
			requireFeatures(b, tc.features...)

			b.StopTimer()
			b.ResetTimer()

			ls := windowsImageLayers(pCtx, b)

			b.Run("Create", func(b *testing.B) {
				b.StopTimer()
				b.ResetTimer()

				var vm *uvm.UtilityVM
				if tc.createUVM {
					vm = testuvm.CreateAndStart(pCtx, b, defaultWCOWOptions(pCtx, b))
				}

				for i := 0; i < b.N; i++ {
					ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

					id := criutil.GenerateID()
					scratch := testlayers.WCOWScratchDir(ctx, b, "")
					spec := testoci.CreateWindowsSpec(ctx, b, id,
						testoci.DefaultWindowsSpecOpts(id,
							append(tc.extraOpts,
								ctrdoci.WithProcessCommandLine("cmd /c (exit 0)"),
								testoci.WithWindowsLayerFolders(append(ls, scratch)),
							)...,
						)...)

					co := &hcsoci.CreateOptions{
						ID:            id,
						HostingSystem: vm,
						Owner:         hcsOwner,
						Spec:          spec,
						// dont create a network namespace on the host side
						NetworkNamespace: "",
					}

					co.LCOWLayers = &layers.LCOWLayers{
						Layers:         make([]*layers.LCOWLayer, 0, len(ls)),
						ScratchVHDPath: filepath.Join(scratch, "sandbox.vhdx"),
					}

					for _, p := range ls {
						co.LCOWLayers.Layers = append(co.LCOWLayers.Layers, &layers.LCOWLayer{VHDPath: filepath.Join(p, "layer.vhd")})
					}

					b.StartTimer()
					c, r, err := hcsoci.CreateContainer(ctx, co)
					if err != nil {
						b.Fatalf("could not create container %q: %v", co.ID, err)
					}
					b.StopTimer()

					// container creation launches go rountines on the guest that do
					// not finish until the init process has terminated.
					// so start the container, then clean everything up
					init := testcontainer.StartWithSpec(ctx, b, c, spec.Process, nil)
					testcmd.WaitExitCode(ctx, b, init, 0)

					testcontainer.Kill(ctx, b, c)
					testcontainer.Wait(ctx, b, c)
					if err := resources.ReleaseResources(ctx, r, vm, true); err != nil {
						b.Errorf("failed to release container resources: %v", err)
					}
					if err := c.Close(); err != nil {
						b.Errorf("could not close container %q: %v", c.ID(), err)
					}

					cancel()
				}
			})

			b.Run("Start", func(b *testing.B) {
				b.StopTimer()
				b.ResetTimer()

				var vm *uvm.UtilityVM
				if tc.createUVM {
					vm = testuvm.CreateAndStart(pCtx, b, defaultWCOWOptions(pCtx, b))
				}

				for i := 0; i < b.N; i++ {
					ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

					id := criutil.GenerateID()
					scratch := testlayers.WCOWScratchDir(ctx, b, "")
					spec := testoci.CreateWindowsSpec(ctx, b, id,
						testoci.DefaultWindowsSpecOpts(id,
							append(tc.extraOpts,
								ctrdoci.WithProcessCommandLine("cmd /c (exit 0)"),
								testoci.WithWindowsLayerFolders(append(ls, scratch)),
							)...,
						)...)

					c, _, cleanup := testcontainer.Create(ctx, b, vm, spec, id, hcsOwner)

					b.StartTimer()
					if err := c.Start(ctx); err != nil {
						b.Fatalf("could not start %q: %v", c.ID(), err)
					}
					b.StopTimer()

					init := testcmd.Create(ctx, b, c, spec.Process, nil)
					testcmd.Start(ctx, b, init)
					testcmd.WaitExitCode(ctx, b, init, 0)

					testcontainer.Kill(ctx, b, c)
					testcontainer.Wait(ctx, b, c)
					cleanup()
					cancel()
				}
			})

			b.Run("InitExec", func(b *testing.B) {
				b.StopTimer()
				b.ResetTimer()

				var vm *uvm.UtilityVM
				if tc.createUVM {
					vm = testuvm.CreateAndStart(pCtx, b, defaultWCOWOptions(pCtx, b))
				}

				for i := 0; i < b.N; i++ {
					ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

					id := criutil.GenerateID()
					scratch := testlayers.WCOWScratchDir(ctx, b, "")
					spec := testoci.CreateWindowsSpec(ctx, b, id,
						testoci.DefaultWindowsSpecOpts(id,
							append(tc.extraOpts,
								ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
								testoci.WithWindowsLayerFolders(append(ls, scratch)),
							)...,
						)...)

					c, _, cleanup := testcontainer.Create(ctx, b, vm, spec, id, hcsOwner)
					if err := c.Start(ctx); err != nil {
						b.Fatalf("could not start %q: %v", c.ID(), err)
					}
					init := testcmd.Create(ctx, b, c, spec.Process, nil)

					b.StartTimer()
					if err := init.Start(); err != nil {
						b.Fatalf("failed to start init command: %v", err)
					}
					b.StopTimer()

					testcmd.Kill(ctx, b, init)
					testcmd.WaitExitCode(ctx, b, init, tc.killExitCode)

					testcontainer.Kill(ctx, b, c)
					testcontainer.Wait(ctx, b, c)
					cleanup()
					cancel()
				}
			})

			b.Run("InitExecKill", func(b *testing.B) {
				b.StopTimer()
				b.ResetTimer()

				var vm *uvm.UtilityVM
				if tc.createUVM {
					vm = testuvm.CreateAndStart(pCtx, b, defaultWCOWOptions(pCtx, b))
				}

				for i := 0; i < b.N; i++ {
					ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

					id := criutil.GenerateID()
					scratch := testlayers.WCOWScratchDir(ctx, b, "")
					spec := testoci.CreateWindowsSpec(ctx, b, id,
						testoci.DefaultWindowsSpecOpts(id,
							append(tc.extraOpts,
								ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
								testoci.WithWindowsLayerFolders(append(ls, scratch)),
							)...,
						)...)

					c, _, cleanup := testcontainer.Create(ctx, b, vm, spec, id, hcsOwner)
					init := testcontainer.StartWithSpec(ctx, b, c, spec.Process, nil)

					b.StartTimer()
					if ok, err := init.Process.Kill(ctx); !ok {
						b.Fatalf("could not deliver kill to init command")
					} else if err != nil {
						b.Fatalf("could not kill init command: %v", err)
					}

					if err := init.Wait(); err != nil {
						ee := &cmd.ExitError{}
						if !errors.As(err, &ee) {
							b.Fatalf("failed to wait on init command: %v", err)
						}
						if ee.ExitCode() != tc.killExitCode {
							b.Fatalf("got exit code %d, wanted %d", ee.ExitCode(), tc.killExitCode)
						}
					}
					b.StopTimer()

					testcontainer.Kill(ctx, b, c)
					testcontainer.Wait(ctx, b, c)
					cleanup()
					cancel()
				}
			})

			b.Run("Exec", func(b *testing.B) {
				b.StopTimer()
				b.ResetTimer()

				var vm *uvm.UtilityVM
				if tc.createUVM {
					vm = testuvm.CreateAndStart(pCtx, b, defaultWCOWOptions(pCtx, b))
				}

				for i := 0; i < b.N; i++ {
					ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

					id := criutil.GenerateID()
					scratch := testlayers.WCOWScratchDir(ctx, b, "")
					spec := testoci.CreateWindowsSpec(ctx, b, id,
						testoci.DefaultWindowsSpecOpts(id,
							append(tc.extraOpts,
								ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
								testoci.WithWindowsLayerFolders(append(ls, scratch)),
							)...,
						)...)

					c, _, cleanup := testcontainer.Create(ctx, b, vm, spec, id, hcsOwner)
					init := testcontainer.StartWithSpec(ctx, b, c, spec.Process, nil)

					ps := testoci.CreateWindowsSpec(ctx, b, id,
						testoci.DefaultWindowsSpecOpts(id,
							append(tc.extraOpts,
								ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
								testoci.WithWindowsLayerFolders(append(ls, scratch)),
							)...,
						)...).Process

					exec := testcmd.Create(ctx, b, c, ps, nil)

					b.StartTimer()
					if err := exec.Start(); err != nil {
						b.Fatalf("failed to start %q: %v", strings.Join(exec.Spec.Args, " "), err)
					}
					b.StopTimer()

					testcmd.Kill(ctx, b, exec)
					testcmd.WaitExitCode(ctx, b, exec, tc.killExitCode)

					testcmd.Kill(ctx, b, init)
					testcmd.WaitExitCode(ctx, b, init, tc.killExitCode)
					testcontainer.Kill(ctx, b, c)
					testcontainer.Wait(ctx, b, c)
					cleanup()
					cancel()
				}
			})

			b.Run("ExecSync", func(b *testing.B) {
				b.StopTimer()
				b.ResetTimer()

				var vm *uvm.UtilityVM
				if tc.createUVM {
					vm = testuvm.CreateAndStart(pCtx, b, defaultWCOWOptions(pCtx, b))
				}

				for i := 0; i < b.N; i++ {
					ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

					id := criutil.GenerateID()
					scratch := testlayers.WCOWScratchDir(ctx, b, "")
					spec := testoci.CreateWindowsSpec(ctx, b, id,
						testoci.DefaultWindowsSpecOpts(id,
							append(tc.extraOpts,
								ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
								testoci.WithWindowsLayerFolders(append(ls, scratch)),
							)...,
						)...)

					c, _, cleanup := testcontainer.Create(ctx, b, vm, spec, id, hcsOwner)
					init := testcontainer.StartWithSpec(ctx, b, c, spec.Process, nil)

					ps := testoci.CreateWindowsSpec(ctx, b, id,
						testoci.DefaultWindowsSpecOpts(id,
							append(tc.extraOpts,
								ctrdoci.WithProcessCommandLine("cmd /c (exit 0)"),
								testoci.WithWindowsLayerFolders(append(ls, scratch)),
							)...,
						)...).Process

					exec := testcmd.Create(ctx, b, c, ps, nil)

					b.StartTimer()
					if err := exec.Start(); err != nil {
						b.Fatalf("failed to start %q: %v", strings.Join(exec.Spec.Args, " "), err)
					}
					if err := exec.Wait(); err != nil {
						b.Fatalf("failed to wait on %q: %v", strings.Join(exec.Spec.Args, " "), err)
					}
					b.StopTimer()

					testcmd.Kill(ctx, b, init)
					testcmd.WaitExitCode(ctx, b, init, tc.killExitCode)
					testcontainer.Kill(ctx, b, c)
					testcontainer.Wait(ctx, b, c)
					cleanup()
					cancel()
				}
			})

			b.Run("ContainerKill", func(b *testing.B) {
				b.StopTimer()
				b.ResetTimer()

				var vm *uvm.UtilityVM
				if tc.createUVM {
					vm = testuvm.CreateAndStart(pCtx, b, defaultWCOWOptions(pCtx, b))
				}

				for i := 0; i < b.N; i++ {
					ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

					id := criutil.GenerateID()
					scratch := testlayers.WCOWScratchDir(ctx, b, "")
					spec := testoci.CreateWindowsSpec(ctx, b, id,
						testoci.DefaultWindowsSpecOpts(id,
							append(tc.extraOpts,
								ctrdoci.WithProcessCommandLine("cmd /c (exit 0)"),
								testoci.WithWindowsLayerFolders(append(ls, scratch)),
							)...,
						)...)

					c, _, cleanup := testcontainer.Create(ctx, b, vm, spec, id, hcsOwner)

					init := testcontainer.StartWithSpec(ctx, b, c, spec.Process, nil)
					testcmd.WaitExitCode(ctx, b, init, 0)

					b.StartTimer()
					testcontainer.Kill(ctx, b, c)
					testcontainer.Wait(ctx, b, c)
					b.StopTimer()

					cleanup()
					cancel()
				}
			})
		})
	}
}
