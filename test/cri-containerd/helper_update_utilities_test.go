//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"bufio"
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/opencontainers/runtime-spec/specs-go"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const podJobObjectUtilPath = "C:\\jobobject-util.exe"

func containerJobObjectName(id string) string {
	return "\\Container_" + id
}

func createJobObjectsGetUtilArgs(ctx context.Context, cid, toolPath string, options []string) []string {
	args := []string{"cmd", "/c", toolPath, "get"}
	args = append(args, options...)
	args = append(args, containerJobObjectName(cid))
	return args
}

func checkLCOWResourceLimit(t *testing.T, ctx context.Context, client runtime.RuntimeServiceClient, cid, path string, expected uint64) {
	t.Helper()
	cmd := []string{"cat", path}
	containerExecReq := &runtime.ExecSyncRequest{
		ContainerId: cid,
		Cmd:         cmd,
		Timeout:     20,
	}
	r := execSync(t, client, ctx, containerExecReq)
	if r.ExitCode != 0 {
		t.Fatalf("failed with exit code %d to cat path: %s", r.ExitCode, r.Stderr)
	}
	output := strings.TrimSpace(string(r.Stdout))
	bytesActual, err := strconv.ParseUint(output, 10, 0)
	if err != nil {
		t.Fatalf("could not parse output %s: %s", output, err)
	}
	if bytesActual != expected {
		t.Fatalf("expected to have a memory limit of %v, instead got %v", expected, bytesActual)
	}
}

func checkWCOWResourceLimit(t *testing.T, ctx context.Context, runtimeHandler, shimName, cid, query string, expected uint64) {
	t.Helper()
	shim, err := shimdiag.GetShim(shimName)
	if err != nil {
		t.Fatalf("failed to find shim %v: %v", shimName, err)
	}
	shimClient := shimdiag.NewShimDiagClient(shim)

	// share the test utility in so we can query the job object
	guestPath := ""
	if runtimeHandler == wcowProcessRuntimeHandler {
		guestPath = testJobObjectUtilFilePath
	} else {
		guestPath = podJobObjectUtilPath
		if err := shareInUVM(ctx, shimClient, testJobObjectUtilFilePath, guestPath, false); err != nil {
			t.Fatalf("failed to share test utility into pod: %v", err)
		}
	}

	// query the job object
	options := []string{"--use-nt", "--" + query}
	args := createJobObjectsGetUtilArgs(ctx, cid, guestPath, options)

	buf := &bytes.Buffer{}
	bw := bufio.NewWriter(buf)
	bufErr := &bytes.Buffer{}
	bwErr := bufio.NewWriter(bufErr)

	exitCode, err := execInHost(ctx, shimClient, args, nil, bw, bwErr)
	if err != nil {
		t.Fatalf("failed to exec request in the host with: %v and %v", err, bufErr.String())
	}
	if exitCode != 0 {
		t.Fatalf("exec request in host failed with exit code %v: %v", exitCode, bufErr.String())
	}

	// validate the results
	value := strings.TrimSpace(strings.TrimPrefix(buf.String(), query+": "))
	limitActual, err := strconv.ParseUint(value, 10, 0)
	if err != nil {
		t.Fatalf("could not parse output %s: %s", buf.String(), err)
	}
	if limitActual != expected {
		t.Fatalf("expected to have a limit of %v, instead got %v", expected, limitActual)
	}
}

func updateContainer(
	tb testing.TB,
	client runtime.RuntimeServiceClient,
	ctx context.Context,
	req *runtime.UpdateContainerResourcesRequest,
) {
	tb.Helper()
	if _, err := client.UpdateContainerResources(ctx, req); err != nil {
		tb.Fatalf("updating container resources for %q with %v", req.ContainerId, err)
	}
}

func checkAnnotation(tb testing.TB, spec *specs.Spec, k, v string) {
	tb.Helper()

	if got := spec.Annotations[k]; got != v {
		tb.Fatalf("annotation %q: got %q, wanted %q", k, got, v)
	}
}

func getOCILinuxResources(tb testing.TB, spec *specs.Spec) *specs.LinuxResources {
	tb.Helper()

	if spec == nil {
		tb.Fatalf("nil OCI spec, %T", spec)
	}
	if spec.Linux == nil {
		tb.Fatalf("nil OCI Linux configuration, %T", spec.Linux)
	}
	if spec.Linux.Resources == nil {
		tb.Fatalf("nil OCI Linux resources, %T", spec.Linux.Resources)
	}
	return spec.Linux.Resources
}

func getOCIWindowsResources(tb testing.TB, spec *specs.Spec) *specs.WindowsResources {
	tb.Helper()

	if spec == nil {
		tb.Fatalf("nil OCI spec, %T", spec)
	}
	if spec.Windows == nil {
		tb.Fatalf("nil OCI Windows configuration, %T", spec.Windows)
	}
	if spec.Windows.Resources == nil {
		tb.Fatalf("nil OCI Windows resources, %T", spec.Windows.Resources)
	}
	return spec.Windows.Resources
}
