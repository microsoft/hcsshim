//go:build windows && functional
// +build windows,functional

package functional

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	ctrdoci "github.com/containerd/containerd/oci"
	cri_util "github.com/containerd/containerd/pkg/cri/util"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/osversion"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	"github.com/Microsoft/hcsshim/test/internal/container"
	testlayers "github.com/Microsoft/hcsshim/test/internal/layers"
	"github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	"github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func BenchmarkLCOW_Container(b *testing.B) {
	requireFeatures(b, featureLCOW, featureContainer)
	require.Build(b, osversion.RS5)

	ctx := namespacedContext()
	ls := linuxImageLayers(ctx, b)

	// Create a new uvm per benchmark in case any left over state lingers

	b.Run("Create", func(b *testing.B) {
		vm := uvm.CreateAndStartLCOWFromOpts(ctx, b, defaultLCOWOptions(b))
		cache := testlayers.CacheFile(ctx, b, "")

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := cri_util.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := oci.CreateLinuxSpec(ctx, b, id,
				oci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", "true"),
					oci.WithWindowsLayerFolders(append(ls, scratch)))...)

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

			// container creations launches gorountines on the guest that do
			// not finish until the init process has terminated.
			// so start the container, then clean everything up
			init := container.Start(ctx, b, c, nil)
			testcmd.WaitExitCode(ctx, b, init, 0)

			container.Kill(ctx, b, c)
			container.Wait(ctx, b, c)
			if err := resources.ReleaseResources(ctx, r, vm, true); err != nil {
				b.Errorf("failed to release container resources: %v", err)
			}
			if err := c.Close(); err != nil {
				b.Errorf("could not close container %q: %v", c.ID(), err)
			}
		}
	})

	b.Run("Start", func(b *testing.B) {
		vm := uvm.CreateAndStartLCOWFromOpts(ctx, b, defaultLCOWOptions(b))
		cache := testlayers.CacheFile(ctx, b, "")

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := cri_util.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := oci.CreateLinuxSpec(ctx, b, id,
				oci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", "true"),
					oci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := container.Create(ctx, b, vm, spec, id, hcsOwner)

			b.StartTimer()
			if err := c.Start(ctx); err != nil {
				b.Fatalf("could not start %q: %v", c.ID(), err)
			}
			b.StopTimer()

			init := testcmd.Create(ctx, b, c, nil, nil)
			testcmd.Start(ctx, b, init)
			testcmd.WaitExitCode(ctx, b, init, 0)

			container.Kill(ctx, b, c)
			container.Wait(ctx, b, c)
			cleanup()
		}
	})

	b.Run("InitExec", func(b *testing.B) {
		vm := uvm.CreateAndStartLCOWFromOpts(ctx, b, defaultLCOWOptions(b))
		cache := testlayers.CacheFile(ctx, b, "")

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := cri_util.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := oci.CreateLinuxSpec(ctx, b, id,
				oci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", oci.TailNullArgs),
					oci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := container.Create(ctx, b, vm, spec, id, hcsOwner)
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

			container.Kill(ctx, b, c)
			container.Wait(ctx, b, c)
			cleanup()
		}
	})

	b.Run("InitExecKill", func(b *testing.B) {
		vm := uvm.CreateAndStartLCOWFromOpts(ctx, b, defaultLCOWOptions(b))
		cache := testlayers.CacheFile(ctx, b, "")

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := cri_util.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := oci.CreateLinuxSpec(ctx, b, id,
				oci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", oci.TailNullArgs),
					oci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := container.Create(ctx, b, vm, spec, id, hcsOwner)
			init := container.Start(ctx, b, c, nil)

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

			container.Kill(ctx, b, c)
			container.Wait(ctx, b, c)
			cleanup()
		}
	})

	b.Run("Exec", func(b *testing.B) {
		vm := uvm.CreateAndStartLCOWFromOpts(ctx, b, defaultLCOWOptions(b))
		cache := testlayers.CacheFile(ctx, b, "")

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := cri_util.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := oci.CreateLinuxSpec(ctx, b, id,
				oci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", oci.TailNullArgs),
					oci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := container.Create(ctx, b, vm, spec, id, hcsOwner)
			init := container.Start(ctx, b, c, nil)

			ps := oci.CreateLinuxSpec(ctx, b, id,
				oci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithDefaultPathEnv,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", oci.TailNullArgs))...,
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
			container.Kill(ctx, b, c)
			container.Wait(ctx, b, c)
			cleanup()
		}
	})

	b.Run("ExecSync", func(b *testing.B) {
		vm := uvm.CreateAndStartLCOWFromOpts(ctx, b, defaultLCOWOptions(b))
		cache := testlayers.CacheFile(ctx, b, "")

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := cri_util.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := oci.CreateLinuxSpec(ctx, b, id,
				oci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", oci.TailNullArgs),
					oci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := container.Create(ctx, b, vm, spec, id, hcsOwner)
			init := container.Start(ctx, b, c, nil)

			ps := oci.CreateLinuxSpec(ctx, b, id,
				oci.DefaultLinuxSpecOpts(id,
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
			container.Kill(ctx, b, c)
			container.Wait(ctx, b, c)
			cleanup()
		}
	})

	b.Run("ContainerKill", func(b *testing.B) {
		vm := uvm.CreateAndStartLCOWFromOpts(ctx, b, defaultLCOWOptions(b))
		cache := testlayers.CacheFile(ctx, b, "")

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := cri_util.GenerateID()
			scratch, _ := testlayers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := oci.CreateLinuxSpec(ctx, b, id,
				oci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", "true"),
					oci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := container.Create(ctx, b, vm, spec, id, hcsOwner)

			// (c container).Wait() waits until the gc receives a notification message from
			// the guest (via the bridge) that the container exited.
			// The Linux guest starts a goroutine to send that notification (bridge_v2.go:createContainerV2)
			// That goroutine, in turn, waits on the init process, which does not unblock until it has
			// been waited on (usually via a WaitForProcess request) and had its exit code read
			// (hcsv2/process.go:(*containerProcess).Wait).
			//
			// So ... to test container kill and wait times, we need to first start and wait on the init process
			init := container.Start(ctx, b, c, nil)
			testcmd.WaitExitCode(ctx, b, init, 0)

			b.StartTimer()
			container.Kill(ctx, b, c)
			container.Wait(ctx, b, c)
			b.StopTimer()

			cleanup()
		}
	})
}
