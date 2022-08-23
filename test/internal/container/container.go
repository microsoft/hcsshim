//go:build windows

package container

import (
	"context"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
)

func Create(
	ctx context.Context,
	t testing.TB,
	vm *uvm.UtilityVM,
	spec *specs.Spec,
	name, owner string,
) (cow.Container, *resources.Resources, func()) {
	t.Helper()

	if spec.Windows == nil || spec.Windows.Network == nil || spec.Windows.LayerFolders == nil {
		t.Fatalf("improperly configured windows spec for container %q: %#+v", name, spec.Windows)
	}

	co := &hcsoci.CreateOptions{
		ID:            name,
		HostingSystem: vm,
		Owner:         owner,
		Spec:          spec,
		// dont create a network namespace on the host side
		NetworkNamespace: "", //spec.Windows.Network.NetworkNamespace,
	}

	c, r, err := hcsoci.CreateContainer(ctx, co)
	if err != nil {
		t.Fatalf("could not create container %q: %v", co.ID, err)
	}
	f := func() {
		if err := resources.ReleaseResources(ctx, r, vm, true); err != nil {
			t.Errorf("failed to release container resources: %v", err)
		}
		if err := c.Close(); err != nil {
			t.Errorf("could not close container %q: %v", c.ID(), err)
		}
	}

	return c, r, f
}

func Start(ctx context.Context, t testing.TB, c cow.Container, io *testcmd.BufferedIO) *cmd.Cmd {
	t.Helper()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("could not start %q: %v", c.ID(), err)
	}

	// OCI spec is nil to tell bridge to start container's init process
	init := testcmd.Create(ctx, t, c, nil, io)
	testcmd.Start(ctx, t, init)

	return init
}

func Wait(_ context.Context, t testing.TB, c cow.Container) {
	// todo: add wait on ctx.Done
	if err := c.Wait(); err != nil {
		t.Fatalf("could not wait on container %q: %v", c.ID(), err)
	}
}

func Kill(ctx context.Context, t testing.TB, c cow.Container) {
	if err := c.Shutdown(ctx); err != nil {
		t.Fatalf("could not terminate container %q: %v", c.ID(), err)
	}
}
