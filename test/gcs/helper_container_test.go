//go:build linux

package gcs

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/containerd/containerd/namespaces"
	ctrdoci "github.com/containerd/containerd/oci"
	oci "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	"github.com/Microsoft/hcsshim/internal/guest/runtime/hcsv2"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/guest/storage"
	"github.com/Microsoft/hcsshim/internal/guest/storage/overlay"
	"github.com/Microsoft/hcsshim/internal/guestpath"

	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
)

// todo: autogenerate/fuzz realistic specs

//
// testing helper functions for generic container management
//

const tailNull = "tail -f /dev/null"

// Creates an overlay mount, and then a container using that mount that runs until stopped.
// The container is created on its own, and not associated with a sandbox pod, and is therefore not CRI compliant.
// [unmountRootfs] is added to the test cleanup.
func createStandaloneContainer(ctx context.Context, tb testing.TB, host *hcsv2.Host, id string, extra ...ctrdoci.SpecOpts) *hcsv2.Container {
	tb.Helper()
	ctx = namespaces.WithNamespace(ctx, testoci.DefaultNamespace)
	scratch, rootfs := mountRootfs(ctx, tb, host, id)
	// spec is passed in from containerd and then updated in internal\hcsoci\create.go:CreateContainer()
	opts := testoci.DefaultLinuxSpecOpts(id,
		ctrdoci.WithRootFSPath(rootfs),
		ctrdoci.WithProcessArgs("/bin/sh", "-c", tailNull),
	)
	opts = append(opts, extra...)
	s := testoci.CreateLinuxSpec(ctx, tb, id, opts...)
	r := &prot.VMHostedContainerSettingsV2{
		OCIBundlePath:    scratch,
		OCISpecification: s,
	}

	tb.Cleanup(func() {
		unmountRootfs(ctx, tb, scratch)
	})

	return createContainer(ctx, tb, host, id, r)
}

func createContainer(ctx context.Context, tb testing.TB, host *hcsv2.Host, id string, s *prot.VMHostedContainerSettingsV2) *hcsv2.Container {
	tb.Helper()
	c, err := host.CreateContainer(ctx, id, s)
	if err != nil {
		tb.Fatalf("could not create container %q: %v", id, err)
	}

	return c
}

func removeContainer(_ context.Context, tb testing.TB, host *hcsv2.Host, id string) {
	tb.Helper()
	host.RemoveContainer(id)
}

func startContainer(ctx context.Context, tb testing.TB, c *hcsv2.Container, conn stdio.ConnectionSettings) hcsv2.Process {
	tb.Helper()
	pid, err := c.Start(ctx, conn)
	if err != nil {
		tb.Fatalf("could not start container %q: %v", c.ID(), err)
	}

	return getProcess(ctx, tb, c, uint32(pid))
}

// waitContainer waits on the container's init process, p.
func waitContainer(ctx context.Context, tb testing.TB, c *hcsv2.Container, p hcsv2.Process, forced bool) {
	tb.Helper()
	var e int
	ch := make(chan prot.NotificationType)

	// have to read the init process exit code to close the container
	exch, dch := p.Wait()
	defer close(dch)
	go func() {
		e = <-exch
		dch <- true
		ch <- c.Wait()
		close(ch)
	}()

	select {
	case n, ok := <-ch:
		if !ok {
			tb.Fatalf("container %q did not return a notification", c.ID())
		}

		switch {
		// UnexpectedExit is the default, ForcedExit if killed
		case n == prot.NtGracefulExit:
		case n == prot.NtUnexpectedExit:
		case forced && n == prot.NtForcedExit:
		default:
			tb.Fatalf("container %q exited with %s", c.ID(), n)
		}
	case <-ctx.Done():
		tb.Fatalf("context canceled: %v", ctx.Err())
	}

	switch {
	case e == 0:
	case forced && e == 137:
	default:
		tb.Fatalf("got exit code %d", e)
	}
}

func waitContainerRaw(c *hcsv2.Container, p hcsv2.Process) (int, prot.NotificationType) {
	exch, dch := p.Wait()
	defer close(dch)
	r := <-exch
	dch <- true
	n := c.Wait()

	return r, n
}

func execProcess(ctx context.Context, tb testing.TB, c *hcsv2.Container, p *oci.Process, con stdio.ConnectionSettings) hcsv2.Process {
	tb.Helper()
	pid, err := c.ExecProcess(ctx, p, con)
	if err != nil {
		tb.Fatalf("could not exec process: %v", err)
	}

	return getProcess(ctx, tb, c, uint32(pid))
}

func getProcess(_ context.Context, tb testing.TB, c *hcsv2.Container, pid uint32) hcsv2.Process {
	tb.Helper()
	p, err := c.GetProcess(pid)
	if err != nil {
		tb.Fatalf("could not get process %d: %v", pid, err)
	}

	return p
}

func killContainer(ctx context.Context, tb testing.TB, c *hcsv2.Container) {
	tb.Helper()
	if err := c.Kill(ctx, syscall.SIGKILL); err != nil {
		tb.Fatalf("could not kill container %q: %v", c.ID(), err)
	}
}

func deleteContainer(ctx context.Context, tb testing.TB, c *hcsv2.Container) {
	tb.Helper()
	if err := c.Delete(ctx); err != nil {
		tb.Fatalf("could not delete container %q: %v", c.ID(), err)
	}
}

func cleanupContainer(ctx context.Context, tb testing.TB, host *hcsv2.Host, c *hcsv2.Container) {
	tb.Helper()
	deleteContainer(ctx, tb, c)
	removeContainer(ctx, tb, host, c.ID())
}

//
// runtime
//

func listContainerStates(_ context.Context, tb testing.TB, rt runtime.Runtime) []runtime.ContainerState {
	tb.Helper()
	css, err := rt.ListContainerStates()
	if err != nil {
		tb.Fatalf("could not list containers: %v", err)
	}

	return css
}

// assertNumberContainers asserts that n containers are found, and then returns the container states.
func assertNumberContainers(ctx context.Context, tb testing.TB, rt runtime.Runtime, n int) {
	tb.Helper()
	fmt := "found %d running containers, wanted %d"
	css := listContainerStates(ctx, tb, rt)
	nn := len(css)
	if nn != n {
		if nn == 0 {
			tb.Fatalf(fmt, nn, n)
		}

		cs := make([]string, nn)
		for i, c := range css {
			cs[i] = c.ID
		}

		tb.Fatalf(fmt+":\n%#+v", nn, n, cs)
	}
}

func getContainerState(ctx context.Context, tb testing.TB, rt runtime.Runtime, id string) runtime.ContainerState {
	tb.Helper()
	css := listContainerStates(ctx, tb, rt)

	for _, cs := range css {
		if cs.ID == id {
			return cs
		}
	}

	tb.Fatalf("could not find container %q", id)
	return runtime.ContainerState{} // just to make the linter happy
}

func assertContainerState(ctx context.Context, tb testing.TB, rt runtime.Runtime, id, state string) {
	tb.Helper()
	cs := getContainerState(ctx, tb, rt, id)
	if cs.Status != state {
		tb.Fatalf("got container %q status %q, wanted %q", id, cs.Status, state)
	}
}

//
// mount management
//

func mountRootfs(ctx context.Context, tb testing.TB, host *hcsv2.Host, id string) (scratch string, rootfs string) {
	tb.Helper()
	scratch = filepath.Join(guestpath.LCOWRootPrefixInUVM, id)
	rootfs = filepath.Join(scratch, "rootfs")
	if err := overlay.MountLayer(ctx,
		[]string{*flagRootfsPath},
		filepath.Join(scratch, "upper"),
		filepath.Join(scratch, "work"),
		rootfs,
		false, // readonly
	); err != nil {
		tb.Fatalf("could not mount overlay layers from %q: %v", *flagRootfsPath, err)
	}

	return scratch, rootfs
}

func unmountRootfs(ctx context.Context, tb testing.TB, path string) {
	tb.Helper()
	if err := storage.UnmountAllInPath(ctx, path, true); err != nil {
		tb.Fatalf("could not unmount container rootfs: %v", err)
	}
	if err := os.RemoveAll(path); err != nil {
		tb.Fatalf("could not remove container directory: %v", err)
	}
}

//
// network namespaces
//

func createNamespace(ctx context.Context, tb testing.TB, nns string) {
	tb.Helper()
	ns := hcsv2.GetOrAddNetworkNamespace(nns)
	if err := ns.Sync(ctx); err != nil {
		tb.Fatalf("could not sync new namespace %q: %v", nns, err)
	}
}

func removeNamespace(ctx context.Context, tb testing.TB, nns string) {
	tb.Helper()
	if err := hcsv2.RemoveNetworkNamespace(ctx, nns); err != nil {
		tb.Fatalf("could not remove namespace %q: %v", nns, err)
	}
}
