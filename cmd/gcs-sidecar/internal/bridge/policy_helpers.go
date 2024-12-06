//go:build windows
// +build windows

package bridge

import (
	"context"

	hcsschema "github.com/Microsoft/hcsshim/cmd/gcs-sidecar/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/cmd/gcs-sidecar/internal/protocol/guestrequest"
)

func ExecProcess(ctx context.Context, containerID string, params hcsschema.ProcessParameters) error {
	/*

		err = h.securityPolicyEnforcer.EnforceExecExternalProcessPolicy(
			ctx,
			params.CommandArgs,
			processParamEnvTOOCIEnv(params.Environment),
			params.WorkingDirectory,
		)
		if err != nil {
			return errors.Wrapf(err, "exec is denied due to policy")
		}
	*/
	return nil
}

func signalProcess(containerID string, processID uint32, signal guestrequest.SignalValueWCOW) error {
	/*
		err = h.securityPolicyEnforcer.EnforceSignalContainerProcessPolicy(ctx, containerID, signal, signalingInitProcess, startupArgList)
		if err != nil {
			return err
		}
	*/

	return nil
}
func resizeConsole(containerID string, height uint16, width uint16) error {
	// not validated in clcow
	return nil
}
