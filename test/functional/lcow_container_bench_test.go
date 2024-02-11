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

func BenchmarkLCOW_Container(b *testing.B) {
	requireFeatures(b, featureLCOW, featureUVM, featureContainer)
	require.Build(b, osversion.RS5)

	pCtx := util.Context(namespacedContext(context.Background()), b)
	ls := linuxImageLayers(pCtx, b)

	// Create a new uVM per benchmark in case any left over state lingers

	// there is (potentially) a memory leak in the Linux GCS that causes "memory usage for cgroup exceeded threshold"
	// errors for the `/gcs` cgroup to be raised.
	// so every so often iterations, we re-create the uVM
	const recreateIters = 100

	b.Run("Create", func(b *testing.B) {
		var (
			vm        *uvm.UtilityVM
			vmCleanup testuvm.CleanupFn
			cache     string
		)
		b.Cleanup(func() {
			if vmCleanup != nil {
				vmCleanup(pCtx)
			}
		})

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

			if i%recreateIters == 0 {
				if vmCleanup != nil {
					vmCleanup(ctx)
				}
				// recreate the uVM
				opts := defaultLCOWOptions(ctx, b)
				vm, vmCleanup = testuvm.CreateLCOW(ctx, b, opts)
				testuvm.Start(ctx, b, vm)
				cache = testlayers.CacheFile(ctx, b, "")
			}

			id := criutil.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := testoci.CreateLinuxSpec(ctx, b, id,
				testoci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", "true"),
					testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

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
			init := testcontainer.Start(ctx, b, c, nil)
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
		var (
			vm        *uvm.UtilityVM
			vmCleanup testuvm.CleanupFn
			cache     string
		)
		b.Cleanup(func() {
			if vmCleanup != nil {
				vmCleanup(pCtx)
			}
		})

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

			if i%recreateIters == 0 {
				if vmCleanup != nil {
					vmCleanup(ctx)
				}
				// recreate the uVM
				opts := defaultLCOWOptions(ctx, b)
				vm, vmCleanup = testuvm.CreateLCOW(ctx, b, opts)
				testuvm.Start(ctx, b, vm)
				cache = testlayers.CacheFile(ctx, b, "")
			}

			id := criutil.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := testoci.CreateLinuxSpec(ctx, b, id,
				testoci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", "true"),
					testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := testcontainer.Create(ctx, b, vm, spec, id, hcsOwner)

			b.StartTimer()
			if err := c.Start(ctx); err != nil {
				b.Fatalf("could not start %q: %v", c.ID(), err)
			}
			b.StopTimer()

			init := testcmd.Create(ctx, b, c, nil, nil)
			testcmd.Start(ctx, b, init)
			testcmd.WaitExitCode(ctx, b, init, 0)

			testcontainer.Kill(ctx, b, c)
			testcontainer.Wait(ctx, b, c)
			cleanup()
			cancel()
		}
	})

	b.Run("InitExec", func(b *testing.B) {
		var (
			vm        *uvm.UtilityVM
			vmCleanup testuvm.CleanupFn
			cache     string
		)
		b.Cleanup(func() {
			if vmCleanup != nil {
				vmCleanup(pCtx)
			}
		})

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

			if i%recreateIters == 0 {
				if vmCleanup != nil {
					vmCleanup(ctx)
				}
				// recreate the uVM
				opts := defaultLCOWOptions(ctx, b)
				vm, vmCleanup = testuvm.CreateLCOW(ctx, b, opts)
				testuvm.Start(ctx, b, vm)
				cache = testlayers.CacheFile(ctx, b, "")
			}

			id := criutil.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := testoci.CreateLinuxSpec(ctx, b, id,
				testoci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", testoci.TailNullArgs),
					testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := testcontainer.Create(ctx, b, vm, spec, id, hcsOwner)
			if err := c.Start(ctx); err != nil {
				b.Fatalf("could not start %q: %v", c.ID(), err)
			}
			init := testcmd.Create(ctx, b, c, nil, nil)

			b.StartTimer()
			if err := init.Start(); err != nil {
				b.Fatalf("failed to start init command: %v", err)
			}
			b.StopTimer()

			testcmd.Kill(ctx, b, init)
			testcmd.WaitExitCode(ctx, b, init, testcmd.ForcedKilledExitCode)

			testcontainer.Kill(ctx, b, c)
			testcontainer.Wait(ctx, b, c)
			cleanup()
			cancel()
		}
	})

	b.Run("InitExecKill", func(b *testing.B) {
		var (
			vm        *uvm.UtilityVM
			vmCleanup testuvm.CleanupFn
			cache     string
		)
		b.Cleanup(func() {
			if vmCleanup != nil {
				vmCleanup(pCtx)
			}
		})

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

			if i%recreateIters == 0 {
				if vmCleanup != nil {
					vmCleanup(ctx)
				}
				// recreate the uVM
				opts := defaultLCOWOptions(ctx, b)
				vm, vmCleanup = testuvm.CreateLCOW(ctx, b, opts)
				testuvm.Start(ctx, b, vm)
				cache = testlayers.CacheFile(ctx, b, "")
			}

			id := criutil.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := testoci.CreateLinuxSpec(ctx, b, id,
				testoci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", testoci.TailNullArgs),
					testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := testcontainer.Create(ctx, b, vm, spec, id, hcsOwner)
			init := testcontainer.Start(ctx, b, c, nil)

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
				if ee.ExitCode() != testcmd.ForcedKilledExitCode {
					b.Fatalf("got exit code %d, wanted %d", ee.ExitCode(), testcmd.ForcedKilledExitCode)
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
		var (
			vm        *uvm.UtilityVM
			vmCleanup testuvm.CleanupFn
			cache     string
		)
		b.Cleanup(func() {
			if vmCleanup != nil {
				vmCleanup(pCtx)
			}
		})

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

			if i%recreateIters == 0 {
				if vmCleanup != nil {
					vmCleanup(ctx)
				}
				// recreate the uVM
				opts := defaultLCOWOptions(ctx, b)
				vm, vmCleanup = testuvm.CreateLCOW(ctx, b, opts)
				testuvm.Start(ctx, b, vm)
				cache = testlayers.CacheFile(ctx, b, "")
			}

			id := criutil.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := testoci.CreateLinuxSpec(ctx, b, id,
				testoci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", testoci.TailNullArgs),
					testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := testcontainer.Create(ctx, b, vm, spec, id, hcsOwner)
			init := testcontainer.Start(ctx, b, c, nil)

			ps := testoci.CreateLinuxSpec(ctx, b, id,
				testoci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithDefaultPathEnv,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", testoci.TailNullArgs))...,
			).Process

			exec := testcmd.Create(ctx, b, c, ps, nil)

			b.StartTimer()
			if err := exec.Start(); err != nil {
				b.Fatalf("failed to start %q: %v", strings.Join(exec.Spec.Args, " "), err)
			}
			b.StopTimer()

			testcmd.Kill(ctx, b, exec)
			testcmd.WaitExitCode(ctx, b, exec, testcmd.ForcedKilledExitCode)

			testcmd.Kill(ctx, b, init)
			testcmd.WaitExitCode(ctx, b, init, testcmd.ForcedKilledExitCode)
			testcontainer.Kill(ctx, b, c)
			testcontainer.Wait(ctx, b, c)
			cleanup()
			cancel()
		}
	})

	b.Run("ExecSync", func(b *testing.B) {
		var (
			vm        *uvm.UtilityVM
			vmCleanup testuvm.CleanupFn
			cache     string
		)
		b.Cleanup(func() {
			if vmCleanup != nil {
				vmCleanup(pCtx)
			}
		})

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

			if i%recreateIters == 0 {
				if vmCleanup != nil {
					vmCleanup(ctx)
				}
				// recreate the uVM
				opts := defaultLCOWOptions(ctx, b)
				vm, vmCleanup = testuvm.CreateLCOW(ctx, b, opts)
				testuvm.Start(ctx, b, vm)
				cache = testlayers.CacheFile(ctx, b, "")
			}

			id := criutil.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := testoci.CreateLinuxSpec(ctx, b, id,
				testoci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", testoci.TailNullArgs),
					testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := testcontainer.Create(ctx, b, vm, spec, id, hcsOwner)
			init := testcontainer.Start(ctx, b, c, nil)

			ps := testoci.CreateLinuxSpec(ctx, b, id,
				testoci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithDefaultPathEnv,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", "true"))...,
			).Process

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
			testcmd.WaitExitCode(ctx, b, init, testcmd.ForcedKilledExitCode)
			testcontainer.Kill(ctx, b, c)
			testcontainer.Wait(ctx, b, c)
			cleanup()
			cancel()
		}
	})

	b.Run("ContainerKill", func(b *testing.B) {
		var (
			vm        *uvm.UtilityVM
			vmCleanup testuvm.CleanupFn
			cache     string
		)
		b.Cleanup(func() {
			if vmCleanup != nil {
				vmCleanup(pCtx)
			}
		})

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(pCtx, benchmarkIterationTimeout)

			if i%recreateIters == 0 {
				if vmCleanup != nil {
					vmCleanup(ctx)
				}
				// recreate the uVM
				opts := defaultLCOWOptions(ctx, b)
				vm, vmCleanup = testuvm.CreateLCOW(ctx, b, opts)
				testuvm.Start(ctx, b, vm)
				cache = testlayers.CacheFile(ctx, b, "")
			}

			id := criutil.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := testoci.CreateLinuxSpec(ctx, b, id,
				testoci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", "true"),
					testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := testcontainer.Create(ctx, b, vm, spec, id, hcsOwner)

			// (c container).Wait() waits until the gc receives a notification message from
			// the guest (via the bridge) that the container exited.
			// The Linux guest starts a goroutine to send that notification (bridge_v2.go:createContainerV2)
			// That goroutine, in turn, waits on the init process, which does not unblock until it has
			// been waited on (usually via a WaitForProcess request) and had its exit code read
			// (hcsv2/process.go:(*containerProcess).Wait).
			//
			// So ... to test container kill and wait times, we need to first start and wait on the init process
			init := testcontainer.Start(ctx, b, c, nil)
			testcmd.WaitExitCode(ctx, b, init, 0)

			b.StartTimer()
			testcontainer.Kill(ctx, b, c)
			testcontainer.Wait(ctx, b, c)
			b.StopTimer()

			cleanup()
			cancel()
		}
	})
}
