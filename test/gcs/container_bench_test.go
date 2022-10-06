//go:build linux

package gcs

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/runtime/hcsv2"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	cri_util "github.com/containerd/containerd/pkg/cri/util"

	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
)

func BenchmarkContainerCreate(b *testing.B) {
	requireFeatures(b, featureStandalone)
	ctx := namespaces.WithNamespace(context.Background(), testoci.DefaultNamespace)
	host, _ := getTestState(ctx, b)

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := b.Name() + cri_util.GenerateID()
		scratch, rootfs := mountRootfs(ctx, b, host, id)

		s := testoci.CreateLinuxSpec(ctx, b, id,
			testoci.DefaultLinuxSpecOpts(id,
				oci.WithRootFSPath(rootfs),
				oci.WithProcessArgs("/bin/sh", "-c", tailNull),
			)...,
		)
		r := &prot.VMHostedContainerSettingsV2{
			OCIBundlePath:    scratch,
			OCISpecification: s,
		}

		b.StartTimer()
		c := createContainer(ctx, b, host, id, r)
		b.StopTimer()

		// create launches background go-routines
		// so kill container to end those and avoid future perf hits
		killContainer(ctx, b, c)
		deleteContainer(ctx, b, c)
		removeContainer(ctx, b, host, id)
		unmountRootfs(ctx, b, scratch)
	}
}

func BenchmarkContainerStart(b *testing.B) {
	requireFeatures(b, featureStandalone)
	ctx := context.Background()
	host, _ := getTestState(ctx, b)

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, r, cleanup := standaloneContainerRequest(ctx, b, host)

		c := createContainer(ctx, b, host, id, r)

		b.StartTimer()
		p := startContainer(ctx, b, c, stdio.ConnectionSettings{})
		b.StopTimer()

		killContainer(ctx, b, c)
		waitContainer(ctx, b, c, p, true)
		cleanupContainer(ctx, b, host, c)
		cleanup()
	}
}

func BenchmarkContainerKill(b *testing.B) {
	requireFeatures(b, featureStandalone)
	ctx := context.Background()
	host, _ := getTestState(ctx, b)

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, r, cleanup := standaloneContainerRequest(ctx, b, host)
		c := createContainer(ctx, b, host, id, r)
		p := startContainer(ctx, b, c, stdio.ConnectionSettings{})

		b.StartTimer()
		killContainer(ctx, b, c)
		_, n := waitContainerRaw(c, p)
		b.StopTimer()

		switch n {
		case prot.NtForcedExit:
		default:
			b.Fatalf("container exit was %s", n)
		}

		cleanupContainer(ctx, b, host, c)
		cleanup()
	}
}

// benchmark container create through wait until exit.
func BenchmarkContainerCompleteExit(b *testing.B) {
	requireFeatures(b, featureStandalone)
	ctx := context.Background()
	host, _ := getTestState(ctx, b)

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, r, cleanup := standaloneContainerRequest(ctx, b, host, oci.WithProcessArgs("/bin/sh", "-c", "true"))

		b.StartTimer()
		c := createContainer(ctx, b, host, id, r)
		p := startContainer(ctx, b, c, stdio.ConnectionSettings{})
		e, n := waitContainerRaw(c, p)
		b.StopTimer()

		switch n {
		case prot.NtGracefulExit, prot.NtUnexpectedExit:
		default:
			b.Fatalf("container exit was %s", n)
		}

		if e != 0 {
			b.Fatalf("container exit code was %d", e)
		}

		killContainer(ctx, b, c)
		c.Wait()
		cleanupContainer(ctx, b, host, c)
		cleanup()
	}
}

func BenchmarkContainerCompleteKill(b *testing.B) {
	requireFeatures(b, featureStandalone)
	ctx := context.Background()
	host, _ := getTestState(ctx, b)

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, r, cleanup := standaloneContainerRequest(ctx, b, host)

		b.StartTimer()
		c := createContainer(ctx, b, host, id, r)
		p := startContainer(ctx, b, c, stdio.ConnectionSettings{})
		killContainer(ctx, b, c)
		_, n := waitContainerRaw(c, p)
		b.StopTimer()

		switch n {
		case prot.NtForcedExit:
		default:
			b.Fatalf("container exit was %s", n)
		}

		cleanupContainer(ctx, b, host, c)
		cleanup()
	}
}

func BenchmarkContainerExec(b *testing.B) {
	requireFeatures(b, featureStandalone)
	ctx := namespaces.WithNamespace(context.Background(), testoci.DefaultNamespace)
	host, _ := getTestState(ctx, b)

	id := b.Name()
	c := createStandaloneContainer(ctx, b, host, id)
	ip := startContainer(ctx, b, c, stdio.ConnectionSettings{})

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ps := testoci.CreateLinuxSpec(ctx, b, id,
			oci.WithDefaultPathEnv,
			oci.WithProcessArgs("/bin/sh", "-c", "true"),
		).Process

		b.StartTimer()
		p := execProcess(ctx, b, c, ps, stdio.ConnectionSettings{})
		exch, dch := p.Wait()
		if e := <-exch; e != 0 {
			b.Errorf("process exited with error code %d", e)
		}
		b.StopTimer()

		dch <- true
		close(dch)
	}

	killContainer(ctx, b, c)
	waitContainer(ctx, b, c, ip, true)
	cleanupContainer(ctx, b, host, c)
}

func standaloneContainerRequest(
	ctx context.Context,
	tb testing.TB,
	host *hcsv2.Host,
	extra ...oci.SpecOpts,
) (string, *prot.VMHostedContainerSettingsV2, func()) {
	tb.Helper()
	ctx = namespaces.WithNamespace(ctx, testoci.DefaultNamespace)
	id := tb.Name() + cri_util.GenerateID()
	scratch, rootfs := mountRootfs(ctx, tb, host, id)

	opts := testoci.DefaultLinuxSpecOpts(id,
		oci.WithRootFSPath(rootfs),
		oci.WithProcessArgs("/bin/sh", "-c", tailNull),
	)
	opts = append(opts, extra...)
	s := testoci.CreateLinuxSpec(ctx, tb, id, opts...)
	r := &prot.VMHostedContainerSettingsV2{
		OCIBundlePath:    scratch,
		OCISpecification: s,
	}
	f := func() {
		unmountRootfs(ctx, tb, scratch)
	}

	return id, r, f
}
