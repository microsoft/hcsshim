//go:build windows && functional
// +build windows,functional

package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/test/pkg/require"
)

const shimExe = "containerd-shim-runhcs-v1.exe"

func runGlobalCommand(t *testing.T, args []string) (string, string, error) {
	t.Helper()

	shim := require.BinaryInPath(t, shimExe)
	cmd := exec.Command(
		shim,
		args...,
	)
	t.Logf("execing global command: %s", cmd.String())

	outb := bytes.Buffer{}
	errb := bytes.Buffer{}
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	err := cmd.Run()
	return outb.String(), errb.String(), err
}

func verifyGlobalCommandSuccess(t *testing.T, expectedStdout, stdout, expectedStderr, stderr string, runErr error) {
	t.Helper()
	if runErr != nil {
		t.Fatalf("expected no error got stdout: '%s', stderr: '%s', err: '%v'", stdout, stderr, runErr)
	}

	verifyGlobalCommandOut(t, expectedStdout, stdout, expectedStderr, stderr)
}

func verifyGlobalCommandFailure(t *testing.T, expectedStdout, stdout, expectedStderr, stderr string, runErr error) {
	t.Helper()
	if runErr == nil || runErr.Error() != "exit status 1" {
		t.Fatalf("expected error: 'exit status 1', got: '%v'", runErr)
	}

	verifyGlobalCommandOut(t, expectedStdout, stdout, expectedStderr, stderr)
}

func verifyGlobalCommandOut(t *testing.T, expectedStdout, stdout, expectedStderr, stderr string) {
	t.Helper()
	// stdout verify
	if expectedStdout == "" && expectedStdout != stdout {
		t.Fatalf("expected stdout empty got: %s", stdout)
	} else if !strings.Contains(stdout, expectedStdout) {
		t.Fatalf("expected stdout to begin with: %s, got: %s", expectedStdout, stdout)
	}

	// stderr verify
	if expectedStderr == "" && expectedStderr != stderr {
		t.Fatalf("expected stderr empty got: %s", stderr)
	} else if !strings.Contains(stderr, expectedStderr) {
		t.Fatalf("expected stderr to begin with: %s, got: %s", expectedStderr, stderr)
	}
}

func Test_Global_Command_No_Namespace(t *testing.T) {
	stdout, stderr, err := runGlobalCommand(
		t,
		[]string{})
	verifyGlobalCommandFailure(t, "", stdout, "namespace is required\n", stderr, err)
}

func Test_Global_Command_No_Address(t *testing.T) {
	stdout, stderr, err := runGlobalCommand(
		t,
		[]string{
			"--namespace", t.Name(),
		})
	verifyGlobalCommandFailure(t, "", stdout, "address is required\n", stderr, err)
}

func Test_Global_Command_No_PublishBinary(t *testing.T) {
	stdout, stderr, err := runGlobalCommand(
		t,
		[]string{
			"--namespace", t.Name(),
			"--address", t.Name(),
		})
	verifyGlobalCommandFailure(t, "", stdout, "publish-binary is required\n", stderr, err)
}

func Test_Global_Command_No_ID(t *testing.T) {
	stdout, stderr, err := runGlobalCommand(
		t,
		[]string{
			"--namespace", t.Name(),
			"--address", t.Name(),
			"--publish-binary", t.Name(),
		})
	verifyGlobalCommandFailure(t, "", stdout, "id is required\n", stderr, err)
}

func Test_Global_Command_No_Command(t *testing.T) {
	stdout, stderr, err := runGlobalCommand(
		t,
		[]string{
			"--namespace", t.Name(),
			"--address", t.Name(),
			"--publish-binary", t.Name(),
			"--id", t.Name(),
		})
	verifyGlobalCommandSuccess(
		t,
		"NAME:\n", stdout,
		"", stderr,
		err)
}
