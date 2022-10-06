//go:build linux

package gcs

import (
	"context"
	"testing"

	cri_util "github.com/containerd/containerd/pkg/cri/util"

	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/runtime/hcsv2"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
)

func BenchmarkCRISanboxCreate(b *testing.B) {
	requireFeatures(b, featureCRI)
	ctx := context.Background()
	host, _ := getTestState(ctx, b)

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := cri_util.GenerateID()
		scratch, rootfs := mountRootfs(ctx, b, host, id)
		nns := id
		createNamespace(ctx, b, nns)
		spec := sandboxSpec(ctx, b, "test-bench-sandbox", id, nns, rootfs)
		r := &prot.VMHostedContainerSettingsV2{
			OCIBundlePath:    scratch,
			OCISpecification: spec,
		}

		b.StartTimer()
		c := createContainer(ctx, b, host, id, r)
		b.StopTimer()

		// create launches background go-routines
		// so kill container to end those and avoid future perf hits
		killContainer(ctx, b, c)
		cleanupContainer(ctx, b, host, c)
		removeNamespace(ctx, b, nns)
		unmountRootfs(ctx, b, scratch)
	}
}

func BenchmarkCRISandboxStart(b *testing.B) {
	requireFeatures(b, featureCRI)
	ctx := context.Background()
	host, _ := getTestState(ctx, b)

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := cri_util.GenerateID()
		scratch, rootfs := mountRootfs(ctx, b, host, id)
		nns := id
		createNamespace(ctx, b, nns)
		spec := sandboxSpec(ctx, b, "test-bench-sandbox", id, nns, rootfs)
		r := &prot.VMHostedContainerSettingsV2{
			OCIBundlePath:    scratch,
			OCISpecification: spec,
		}

		c := createContainer(ctx, b, host, id, r)

		b.StartTimer()
		p := startContainer(ctx, b, c, stdio.ConnectionSettings{})
		b.StopTimer()

		killContainer(ctx, b, c)
		waitContainer(ctx, b, c, p, true)
		cleanupContainer(ctx, b, host, c)
		removeNamespace(ctx, b, nns)
		unmountRootfs(ctx, b, scratch)
	}
}

func BenchmarkCRISandboxKill(b *testing.B) {
	requireFeatures(b, featureCRI)
	ctx := context.Background()
	host, _ := getTestState(ctx, b)

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := cri_util.GenerateID()
		scratch, rootfs := mountRootfs(ctx, b, host, id)
		nns := id
		createNamespace(ctx, b, nns)
		spec := sandboxSpec(ctx, b, "test-bench-sandbox", id, nns, rootfs)
		r := &prot.VMHostedContainerSettingsV2{
			OCIBundlePath:    scratch,
			OCISpecification: spec,
		}

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
		removeNamespace(ctx, b, nns)
		unmountRootfs(ctx, b, scratch)
	}
}

func BenchmarkCRIWorkload(b *testing.B) {
	requireFeatures(b, featureCRI)
	ctx := context.Background()
	host, _ := getTestState(ctx, b)

	sid := b.Name()
	sScratch, sRootfs := mountRootfs(ctx, b, host, sid)
	b.Cleanup(func() {
		unmountRootfs(ctx, b, sScratch)
	})
	nns := sid
	createNamespace(ctx, b, nns)
	b.Cleanup(func() {
		removeNamespace(ctx, b, nns)
	})

	sSpec := sandboxSpec(ctx, b, "test-bench-sandbox", sid, nns, sRootfs)
	sandbox := createContainer(ctx, b, host, sid, &prot.VMHostedContainerSettingsV2{
		OCIBundlePath:    sScratch,
		OCISpecification: sSpec,
	})
	b.Cleanup(func() {
		cleanupContainer(ctx, b, host, sandbox)
	})

	sandboxInit := startContainer(ctx, b, sandbox, stdio.ConnectionSettings{})
	b.Cleanup(func() {
		killContainer(ctx, b, sandbox)
		waitContainer(ctx, b, sandbox, sandboxInit, true)
	})

	b.Run("Create", func(b *testing.B) {
		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id, r, cleanup := workloadContainerRequest(ctx, b, host, sid, uint32(sandboxInit.Pid()), nns)

			b.StartTimer()
			c := createContainer(ctx, b, host, id, r)
			b.StopTimer()

			// create launches background go-routines
			// so kill container to end those and avoid future perf hits
			killContainer(ctx, b, c)
			// edge case where workload container transitions from "created" to "paused"
			//  then "stopped"
			waitContainerRaw(c, c.InitProcess())
			cleanupContainer(ctx, b, host, c)
			cleanup()
		}
	})

	b.Run("Start", func(b *testing.B) {
		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id, r, cleanup := workloadContainerRequest(ctx, b, host, sid, uint32(sandboxInit.Pid()), nns)
			c := createContainer(ctx, b, host, id, r)

			b.StartTimer()
			p := startContainer(ctx, b, c, stdio.ConnectionSettings{})
			b.StopTimer()

			killContainer(ctx, b, c)
			waitContainer(ctx, b, c, p, true)
			cleanupContainer(ctx, b, host, c)
			cleanup()
		}
	})

	b.Run("Kill", func(b *testing.B) {
		b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id, r, cleanup := workloadContainerRequest(ctx, b, host, sid, uint32(sandboxInit.Pid()), nns)
			c := createContainer(ctx, b, host, id, r)
			p := startContainer(ctx, b, c, stdio.ConnectionSettings{})

			b.StartTimer()
			killContainer(ctx, b, c)
			_, n := waitContainerRaw(c, p)
			b.StopTimer()

			switch n {
			case prot.NtForcedExit:
			default:
				b.Fatalf("container exit was %q, expected %q", n, prot.NtForcedExit)
			}

			cleanupContainer(ctx, b, host, c)
			cleanup()
		}
	})
}

func workloadContainerRequest(
	ctx context.Context,
	tb testing.TB,
	host *hcsv2.Host,
	sid string,
	spid uint32,
	nns string,
) (string, *prot.VMHostedContainerSettingsV2, func()) {
	tb.Helper()
	id := sid + cri_util.GenerateID()
	scratch, rootfs := mountRootfs(ctx, tb, host, id)
	spec := containerSpec(ctx, tb,
		sid,
		spid,
		"test-bench-container",
		id,
		[]string{"/bin/sh", "-c"},
		[]string{tailNull},
		"/",
		nns,
		rootfs,
	)
	r := &prot.VMHostedContainerSettingsV2{
		OCIBundlePath:    scratch,
		OCISpecification: spec,
	}
	f := func() {
		unmountRootfs(ctx, tb, scratch)
	}

	return id, r, f
}
