//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// execSuccess calls [execSync], but expects the exec to execute with exit code 0 and an empty Stderr.
func execSuccess(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.ExecSyncRequest) string {
	tb.Helper()

	resp := execSync(tb, client, ctx, request)
	if resp.ExitCode != 0 {
		tb.Fatalf("exec %v: failed with exit code %d: %s %s", request.Cmd, resp.ExitCode, string(resp.Stdout), string(resp.Stderr))
	}

	if err := strings.TrimSpace(string(resp.Stderr)); err != "" {
		tb.Fatalf("exec %v: got stderr %q", request.Cmd, err)
	}

	return string(resp.Stdout)
}

func execSync(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.ExecSyncRequest) *runtime.ExecSyncResponse {
	tb.Helper()
	response, err := client.ExecSync(ctx, request)
	if err != nil {
		tb.Fatalf("failed ExecSync request with: %v", err)
	}
	return response
}

func execRequest(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.ExecRequest) string {
	t.Helper()
	response, err := client.Exec(ctx, request)
	if err != nil {
		t.Fatalf("failed Exec request with: %v", err)
	}
	return response.Url
}

// execInHost executes command in the container's host.
// `stdinBuf` is an optional parameter to specify an io.Reader that can be used as stdin for the executed program.
// `stdoutBuf` is an optional parameter to specify an io.Writer that can be used as stdout for the executed program.
// `stderrBuf` is an optional parameter to specify an io.Writer that can be used as stderr for the executed program.
func execInHost(ctx context.Context, client shimdiag.ShimDiagService, args []string, stdinBuf io.Reader, stdoutBuf, stderrBuf io.Writer) (_ int32, err error) {
	var (
		stdin  = ""
		stdout = ""
		stderr = ""
	)

	if stdinBuf != nil {
		stdin, err = cmd.CreatePipeAndListen(stdinBuf, true)
		if err != nil {
			return 0, err
		}
	}
	if stdoutBuf != nil {
		stdout, err = cmd.CreatePipeAndListen(stdoutBuf, false)
		if err != nil {
			return 0, err
		}
	}
	if stderrBuf != nil {
		stderr, err = cmd.CreatePipeAndListen(stderrBuf, false)
		if err != nil {
			return 0, err
		}
	}

	resp, err := client.DiagExecInHost(ctx, &shimdiag.ExecProcessRequest{
		Args:   args,
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return 0, err
	}
	return resp.ExitCode, nil
}

// shimDiagExecOutput is a small wrapper on top of execInHost, that returns the exec output
func shimDiagExecOutput(ctx context.Context, t *testing.T, podID string, cmd []string) string {
	t.Helper()
	shimName := fmt.Sprintf("k8s.io-%s", podID)
	shim, err := shimdiag.GetShim(shimName)
	if err != nil {
		t.Fatalf("failed to find shim %v: %v", shimName, err)
	}
	shimClient := shimdiag.NewShimDiagClient(shim)

	bufOut := &bytes.Buffer{}
	bw := bufio.NewWriter(bufOut)
	bufErr := &bytes.Buffer{}
	bwErr := bufio.NewWriter(bufErr)

	exitCode, err := execInHost(ctx, shimClient, cmd, nil, bw, bwErr)
	if err != nil {
		t.Fatalf("failed to exec request in the host with: %v and %v", err, bufErr.String())
	}
	if exitCode != 0 {
		t.Fatalf("exec request in host failed with exit code %v: %v", exitCode, bufErr.String())
	}

	return strings.TrimSpace(bufOut.String())
}

func filterStrings(input []string, include string) []string {
	var result []string
	for _, str := range input {
		if strings.Contains(str, include) {
			result = append(result, str)
		}
	}
	return result
}
