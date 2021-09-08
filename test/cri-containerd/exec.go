// +build functional

package cri_containerd

import (
	"context"
	"io"
	"testing"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func execSync(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.ExecSyncRequest) *runtime.ExecSyncResponse {
	response, err := client.ExecSync(ctx, request)
	if err != nil {
		t.Fatalf("failed ExecSync request with: %v", err)
	}
	return response
}

func execRequest(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, request *runtime.ExecRequest) string {
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
