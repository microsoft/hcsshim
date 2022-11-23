//go:build windows && functional
// +build windows,functional

package functional

import (
	"testing"

	ctrdoci "github.com/containerd/containerd/oci"
	cri_util "github.com/containerd/containerd/pkg/cri/util"

	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/internal/cmd"
	"github.com/Microsoft/hcsshim/test/internal/container"
	"github.com/Microsoft/hcsshim/test/internal/layers"
	"github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/internal/require"
	"github.com/Microsoft/hcsshim/test/internal/uvm"
)

func BenchmarkLCOW_Container(b *testing.B) {
	requireFeatures(b, featureLCOW, featureContainer)
	require.Build(b, osversion.RS5)

	ctx := namespacedContext()
	ls := linuxImageLayers(ctx, b)

	// Create a new uvm per benchmark in case any left over state lingers

	b.Run("Create", func(b *testing.B) {
		vm := uvm.CreateAndStartLCOWFromOpts(ctx, b, defaultLCOWOptions(b))
		cache := layers.CacheFile(ctx, b, "")

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := cri_util.GenerateID()
			scratch, _ := layers.ScratchSpace(ctx, b, vm, "", "", cache)
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
			cmd.WaitExitCode(ctx, b, init, 0)

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
		cache := layers.CacheFile(ctx, b, "")

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := cri_util.GenerateID()
			scratch, _ := layers.ScratchSpace(ctx, b, vm, "", "", cache)
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

			init := cmd.Create(ctx, b, c, nil, nil)
			cmd.Start(ctx, b, init)
			cmd.WaitExitCode(ctx, b, init, 0)

			container.Kill(ctx, b, c)
			container.Wait(ctx, b, c)
			cleanup()
		}
	})

	b.Run("InitExec", func(b *testing.B) {
		vm := uvm.CreateAndStartLCOWFromOpts(ctx, b, defaultLCOWOptions(b))
		cache := layers.CacheFile(ctx, b, "")

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := cri_util.GenerateID()
			scratch, _ := layers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := oci.CreateLinuxSpec(ctx, b, id,
				oci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", oci.TailNullArgs),
					oci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := container.Create(ctx, b, vm, spec, id, hcsOwner)
			if err := c.Start(ctx); err != nil {
				b.Fatalf("could not start %q: %v", c.ID(), err)
			}
			init := cmd.Create(ctx, b, c, nil, nil)

			b.StartTimer()
			cmd.Start(ctx, b, init)
			b.StopTimer()

			cmd.Kill(ctx, b, init)
			cmd.WaitExitCode(ctx, b, init, 137)

			container.Kill(ctx, b, c)
			container.Wait(ctx, b, c)
			cleanup()
		}
	})

	b.Run("InitExecKill", func(b *testing.B) {
		vm := uvm.CreateAndStartLCOWFromOpts(ctx, b, defaultLCOWOptions(b))
		cache := layers.CacheFile(ctx, b, "")

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := cri_util.GenerateID()
			scratch, _ := layers.ScratchSpace(ctx, b, vm, "", "", cache)
			spec := oci.CreateLinuxSpec(ctx, b, id,
				oci.DefaultLinuxSpecOpts(id,
					ctrdoci.WithProcessArgs("/bin/sh", "-c", oci.TailNullArgs),
					oci.WithWindowsLayerFolders(append(ls, scratch)))...)

			c, _, cleanup := container.Create(ctx, b, vm, spec, id, hcsOwner)
			init := container.Start(ctx, b, c, nil)

			b.StartTimer()
			cmd.Kill(ctx, b, init)
			cmd.WaitExitCode(ctx, b, init, 137)
			b.StopTimer()

			container.Kill(ctx, b, c)
			container.Wait(ctx, b, c)
			cleanup()
		}
	})

	b.Run("ContainerKill", func(b *testing.B) {
		vm := uvm.CreateAndStartLCOWFromOpts(ctx, b, defaultLCOWOptions(b))
		cache := layers.CacheFile(ctx, b, "")

		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := cri_util.GenerateID()
			scratch, _ := layers.ScratchSpace(ctx, b, vm, "", "", cache)
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
			cmd.WaitExitCode(ctx, b, init, 0)

			b.StartTimer()
			container.Kill(ctx, b, c)
			container.Wait(ctx, b, c)
			b.StopTimer()

			cleanup()
		}
	})
}
