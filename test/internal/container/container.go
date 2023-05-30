//go:build windows

package container

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
)

func Create(
	ctx context.Context,
	tb testing.TB,
	vm *uvm.UtilityVM,
	spec *specs.Spec,
	name, owner string,
) (cow.Container, *resources.Resources, func()) {
	tb.Helper()

	if spec.Windows == nil || spec.Windows.Network == nil || spec.Windows.LayerFolders == nil {
		tb.Fatalf("improperly configured windows spec for container %q: %#+v", name, spec.Windows)
	}

	co := &hcsoci.CreateOptions{
		ID:            name,
		HostingSystem: vm,
		Owner:         owner,
		Spec:          spec,
		// dont create a network namespace on the host side
		NetworkNamespace: "", //spec.Windows.Network.NetworkNamespace,
	}

	if co.Spec.Linux != nil {
		var layerFolders []string
		if co.Spec.Windows != nil {
			layerFolders = co.Spec.Windows.LayerFolders
		}
		if len(layerFolders) <= 1 {
			tb.Fatalf("LCOW requires at least 2 layers (including scratch): %v", layerFolders)
		}
		scratch := layerFolders[len(layerFolders)-1]
		parents := layerFolders[:len(layerFolders)-1]

		// todo: support partitioned layers
		co.LCOWLayers = &layers.LCOWLayers{
			Layers:         make([]*layers.LCOWLayer, 0, len(parents)),
			ScratchVHDPath: filepath.Join(scratch, "sandbox.vhdx"),
		}

		for _, p := range parents {
			co.LCOWLayers.Layers = append(co.LCOWLayers.Layers, &layers.LCOWLayer{VHDPath: filepath.Join(p, "layer.vhd")})
		}
	}

	c, r, err := hcsoci.CreateContainer(ctx, co)
	if err != nil {
		tb.Fatalf("could not create container %q: %v", co.ID, err)
	}
	f := func() {
		if err := resources.ReleaseResources(ctx, r, vm, true); err != nil {
			tb.Errorf("failed to release container resources: %v", err)
		}
		if err := c.Close(); err != nil {
			tb.Errorf("could not close container %q: %v", c.ID(), err)
		}
	}

	return c, r, f
}

func Start(ctx context.Context, tb testing.TB, c cow.Container, io *testcmd.BufferedIO) *cmd.Cmd {
	tb.Helper()
	if err := c.Start(ctx); err != nil {
		tb.Fatalf("could not start %q: %v", c.ID(), err)
	}

	// OCI spec is nil to tell bridge to start container's init process
	init := testcmd.Create(ctx, tb, c, nil, io)
	testcmd.Start(ctx, tb, init)

	return init
}

func Wait(_ context.Context, tb testing.TB, c cow.Container) {
	tb.Helper()
	// todo: add wait on ctx.Done
	if err := c.Wait(); err != nil {
		tb.Fatalf("could not wait on container %q: %v", c.ID(), err)
	}
}

func Kill(ctx context.Context, tb testing.TB, c cow.Container) {
	tb.Helper()
	if err := c.Shutdown(ctx); err != nil {
		tb.Fatalf("could not terminate container %q: %v", c.ID(), err)
	}
}
