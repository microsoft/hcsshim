//go:build linux

package gcs

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
)

//
// tests for operations on sandbox and workload (CRI) containers
//

// TestCRILifecycle tests the entire CRI container workflow: creating and starting a CRI sandbox
// pod container, creating, starting, and stopping a workload container within that pod, asserting
// that all operations were successful, and mounting (and unmounting) rootfs's as necessary.
func TestCRILifecycle(t *testing.T) {
	requireFeatures(t, featureCRI)

	ctx := context.Background()
	host, rtime := getTestState(ctx, t)
	assertNumberContainers(ctx, t, rtime, 0)

	sid := t.Name()
	scratch, rootfs := mountRootfs(ctx, t, host, sid)
	t.Cleanup(func() {
		unmountRootfs(ctx, t, scratch)
	})
	createNamespace(ctx, t, sid)
	t.Cleanup(func() {
		removeNamespace(ctx, t, sid)
	})

	spec := sandboxSpec(ctx, t, "test-sandbox", sid, sid, rootfs)
	sandbox := createContainer(ctx, t, host, sid, &prot.VMHostedContainerSettingsV2{
		OCIBundlePath:    scratch,
		OCISpecification: spec,
	})
	t.Cleanup(func() {
		cleanupContainer(ctx, t, host, sandbox)
		assertNumberContainers(ctx, t, rtime, 0)
	})

	assertNumberContainers(ctx, t, rtime, 1)
	assertContainerState(ctx, t, rtime, sid, "created")

	sandboxInit := startContainer(ctx, t, sandbox, stdio.ConnectionSettings{})
	t.Cleanup(func() {
		killContainer(ctx, t, sandbox)
		waitContainer(ctx, t, sandbox, sandboxInit, true)
	})

	assertContainerState(ctx, t, rtime, sid, "running")
	cs := getContainerState(ctx, t, rtime, sid)
	pid := sandboxInit.Pid()
	if pid != cs.Pid {
		t.Fatalf("got sandbox pid %d, wanted %d", pid, cs.Pid)
	}

	cid := "container" + sid
	cscratch, crootfs := mountRootfs(ctx, t, host, cid)
	t.Cleanup(func() {
		unmountRootfs(ctx, t, cscratch)
	})

	cspec := containerSpec(ctx, t, sid, uint32(sandboxInit.Pid()), "test-container", cid,
		[]string{"/bin/sh", "-c"},
		[]string{tailNull},
		"/", sid, crootfs,
	)
	workload := createContainer(ctx, t, host, cid, &prot.VMHostedContainerSettingsV2{
		OCIBundlePath:    cscratch,
		OCISpecification: cspec,
	})
	t.Cleanup(func() {
		cleanupContainer(ctx, t, host, workload)
		assertNumberContainers(ctx, t, rtime, 1)
	})

	assertNumberContainers(ctx, t, rtime, 2)
	assertContainerState(ctx, t, rtime, cid, "created")

	workloadInit := startContainer(ctx, t, workload, stdio.ConnectionSettings{})
	assertContainerState(ctx, t, rtime, cid, "running")
	t.Cleanup(func() {
		killContainer(ctx, t, workload)
		waitContainer(ctx, t, workload, workloadInit, true)
	})

	cs = getContainerState(ctx, t, rtime, cid)
	pid = workloadInit.Pid()
	if pid != cs.Pid {
		t.Fatalf("got sandbox pid %d, wanted %d", pid, cs.Pid)
	}
}
