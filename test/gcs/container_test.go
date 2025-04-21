//go:build linux

package gcs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"golang.org/x/sync/errgroup"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"

	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
)

//
// tests for operations on standalone containers
//

// todo: using `oci.WithTTY` for IO tests is broken and hangs

func TestContainerCreate(t *testing.T) {
	requireFeatures(t, featureStandalone)

	ctx := context.Background()
	host, rtime := getTestState(ctx, t)
	assertNumberContainers(ctx, t, rtime, 0)

	id := t.Name()
	c := createStandaloneContainer(ctx, t, host, id)
	t.Cleanup(func() {
		cleanupContainer(ctx, t, host, c)
	})

	p := startContainer(ctx, t, c, stdio.ConnectionSettings{})
	t.Cleanup(func() {
		killContainer(ctx, t, c)
		waitContainer(ctx, t, c, p, true)
	})

	assertNumberContainers(ctx, t, rtime, 1)
	css := listContainerStates(ctx, t, rtime)
	// guaranteed by assertNumberContainers that css will only have 1 element
	cs := css[0]
	if cs.ID != id {
		t.Fatalf("got id %q, wanted %q", cs.ID, id)
	}
	pid := p.Pid()
	if pid != cs.Pid {
		t.Fatalf("got pid %d, wanted %d", pid, cs.Pid)
	}
	if cs.Status != "running" {
		t.Fatalf("got status %q, wanted %q", cs.Status, "running")
	}
}

func TestContainerDelete(t *testing.T) {
	requireFeatures(t, featureStandalone)

	ctx := context.Background()
	host, rtime := getTestState(ctx, t)
	assertNumberContainers(ctx, t, rtime, 0)

	id := t.Name()

	c := createStandaloneContainer(ctx, t, host, id,
		oci.WithProcessArgs("/bin/sh", "-c", "true"),
	)

	p := startContainer(ctx, t, c, stdio.ConnectionSettings{})
	waitContainer(ctx, t, c, p, false)

	cleanupContainer(ctx, t, host, c)

	_, err := host.GetCreatedContainer(id)
	if hr, herr := gcserr.GetHresult(err); herr != nil || hr != gcserr.HrVmcomputeSystemNotFound {
		t.Fatalf("GetCreatedContainer returned %v, wanted %v", err, gcserr.HrVmcomputeSystemNotFound)
	}
	assertNumberContainers(ctx, t, rtime, 0)
}

//
// IO
//

var ioTests = []struct {
	name string
	args []string
	in   string
	want string
}{
	{
		name: "true",
		args: []string{"/bin/sh", "-c", "true"},
		want: "",
	},
	{
		name: "echo",
		args: []string{"/bin/sh", "-c", `echo -n "hi y'all"`},
		want: "hi y'all",
	},
	{
		name: "tee",
		args: []string{"/bin/sh", "-c", "tee"},
		in:   "are you copying me?",
		want: "are you copying me?",
	},
}

func TestContainerIO(t *testing.T) {
	requireFeatures(t, featureStandalone)

	ctx := context.Background()
	host, rtime := getTestState(ctx, t)
	assertNumberContainers(ctx, t, rtime, 0)

	for _, tt := range ioTests {
		t.Run(tt.name, func(t *testing.T) {
			id := strings.ReplaceAll(t.Name(), "/", "")

			con := newConnectionSettings(tt.in != "", true, true)
			f := createStdIO(ctx, t, con)

			var outStr, errStr string
			g := &errgroup.Group{}
			g.Go(func() error {
				outStr = f.ReadAllOut(ctx, t)

				return nil
			})
			g.Go(func() error {
				errStr = f.ReadAllErr(ctx, t)

				return nil
			})

			c := createStandaloneContainer(ctx, t, host, id,
				oci.WithProcessArgs(tt.args...),
			)
			t.Cleanup(func() {
				cleanupContainer(ctx, t, host, c)
			})
			p := startContainer(ctx, t, c, con)

			f.WriteIn(ctx, t, tt.in)
			f.CloseIn(ctx, t)
			t.Logf("wrote to stdin: %q", tt.in)

			waitContainer(ctx, t, c, p, false)

			_ = g.Wait()
			t.Logf("stdout: %q", outStr)
			t.Logf("stderr: %q", errStr)

			if errStr != "" {
				t.Fatalf("container returned error %q", errStr)
			}
			if outStr != tt.want {
				t.Fatalf("container returned %q; wanted %q", outStr, tt.want)
			}
		})
	}

	assertNumberContainers(ctx, t, rtime, 0)
}

func TestContainerExec(t *testing.T) {
	requireFeatures(t, featureStandalone)

	ctx := namespaces.WithNamespace(context.Background(), testoci.DefaultNamespace)
	host, rtime := getTestState(ctx, t)
	assertNumberContainers(ctx, t, rtime, 0)

	id := t.Name()
	c := createStandaloneContainer(ctx, t, host, id)
	t.Cleanup(func() {
		cleanupContainer(ctx, t, host, c)
	})

	ip := startContainer(ctx, t, c, stdio.ConnectionSettings{})
	t.Cleanup(func() {
		killContainer(ctx, t, c)
		waitContainer(ctx, t, c, ip, true)
	})

	for _, tt := range ioTests {
		t.Run(tt.name, func(t *testing.T) {
			ps := testoci.CreateLinuxSpec(ctx, t, id,
				oci.WithDefaultPathEnv,
				oci.WithProcessArgs(tt.args...),
			).Process
			con := newConnectionSettings(tt.in != "", true, true)
			f := createStdIO(ctx, t, con)

			var outStr, errStr string
			g := &errgroup.Group{}
			g.Go(func() error {
				outStr = f.ReadAllOut(ctx, t)

				return nil
			})
			g.Go(func() error {
				errStr = f.ReadAllErr(ctx, t)

				return nil
			})

			// OS pipes can lose some data, so sleep a bit to let ReadAll* kick off
			time.Sleep(10 * time.Millisecond)

			p := execProcess(ctx, t, c, ps, con)
			f.WriteIn(ctx, t, tt.in)
			f.CloseIn(ctx, t)
			t.Logf("wrote std in: %q", tt.in)

			exch, _ := p.Wait()
			if i := <-exch; i != 0 {
				t.Errorf("process exited with error code %d", i)
			}

			_ = g.Wait()
			t.Logf("stdout: %q", outStr)
			t.Logf("stderr: %q", errStr)

			if errStr != "" {
				t.Fatalf("exec returned error %q", errStr)
			} else if outStr != tt.want {
				t.Fatalf("process returned %q; wanted %q", outStr, tt.want)
			}
		})
	}
}
