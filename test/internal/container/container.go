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
	"github.com/Microsoft/hcsshim/internal/jobcontainers"
	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
)

// TODO: update cleanup func to accept context, same as uVM cleanup

// a test version of the container creation logic in: cmd\containerd-shim-runhcs-v1\task_hcs.go:createContainer
func Create(
	ctx context.Context,
	tb testing.TB,
	vm *uvm.UtilityVM,
	spec *specs.Spec,
	name, owner string,
) (c cow.Container, r *resources.Resources, _ func()) {
	tb.Helper()

	if spec.Windows == nil || spec.Windows.Network == nil || spec.Windows.LayerFolders == nil {
		tb.Fatalf("improperly configured windows spec for container %q: %#+v", name, spec.Windows)
	}

	var err error
	if oci.IsJobContainer(spec) {
		c, r, err = jobcontainers.Create(ctx, name, spec)
	} else {
		co := &hcsoci.CreateOptions{
			ID:            name,
			HostingSystem: vm,
			Owner:         owner,
			Spec:          spec,
			// Don't create a network namespace on the host side:
			// If one is needed, it'll be created manually during testing.
			// Additionally, these are "standalone" containers, and not CRI pod/workload containers,
			// so leave end-to-end testing with namespaces for CRI tests
			NetworkNamespace: "",
		}

		if co.Spec.Linux != nil {
			if vm == nil {
				tb.Fatalf("LCOW requires a uVM")
			}

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

		c, r, err = hcsoci.CreateContainer(ctx, co)
	}

	if err != nil {
		tb.Fatalf("could not create container %q: %v", name, err)
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

// todo: unify Start and StartWithSpec and add logic to check for WCOW

// for starting an LCOW container, where no process spec is passed
//
// see:
//   - github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/exec_hcs.go: (*hcsExec).startInternal
//   - github.com/Microsoft/hcsshim/cmd/internal/cmd/cmd.go: (*Cmd).Start
func Start(ctx context.Context, tb testing.TB, c cow.Container, io *testcmd.BufferedIO) *cmd.Cmd {
	tb.Helper()

	// OCI spec is nil to tell bridge to start container's init process
	return StartWithSpec(ctx, tb, c, nil, io)
}

func StartWithSpec(ctx context.Context, tb testing.TB, c cow.Container, p *specs.Process, io *testcmd.BufferedIO) *cmd.Cmd {
	tb.Helper()
	if err := c.Start(ctx); err != nil {
		tb.Fatalf("could not start %q: %v", c.ID(), err)
	}

	init := testcmd.Create(ctx, tb, c, p, io)
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
