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

func execSync(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.ExecSyncRequest) *runtime.ExecSyncResponse {
	tb.Helper()
	response, err := client.ExecSync(ctx, request)
	if err != nil {
		tb.Fatalf("failed ExecSync request with: %v", err)
	}
	return response
}

func execRequest(tb testing.TB, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.ExecRequest) string {
	tb.Helper()
	response, err := client.Exec(ctx, request)
	if err != nil {
		tb.Fatalf("failed Exec request with: %v", err)
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

func shimDiagExecOutputWithErr(ctx context.Context, tb testing.TB, podID string, cmd []string) (string, error) {
	tb.Helper()
	shimName := fmt.Sprintf("k8s.io-%s", podID)
	shim, err := shimdiag.GetShim(shimName)
	if err != nil {
		return "", err
	}
	shimClient := shimdiag.NewShimDiagClient(shim)

	bufOut := &bytes.Buffer{}
	bw := bufio.NewWriter(bufOut)
	bufErr := &bytes.Buffer{}
	bwErr := bufio.NewWriter(bufErr)

	exitCode, err := execInHost(ctx, shimClient, cmd, nil, bw, bwErr)
	if err != nil {
		return "", fmt.Errorf("failed to exec request in the host with: %s and %s", err, bufErr.String())

	}
	if exitCode != 0 {
		return "", fmt.Errorf("exec request in host failed with exit code %v: %v", exitCode, bufErr.String())
	}
	return strings.TrimSpace(bufOut.String()), nil
}

// shimDiagExecOutput is a small wrapper on top of execInHost, that returns the exec output
func shimDiagExecOutput(ctx context.Context, tb testing.TB, podID string, cmd []string) string {
	tb.Helper()
	out, err := shimDiagExecOutputWithErr(ctx, tb, podID, cmd)
	if err != nil {
		tb.Fatal(err)
	}
	return out
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
