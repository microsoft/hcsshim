//go:build windows && functional
// +build windows,functional

package main

import (
	"testing"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	"google.golang.org/protobuf/proto"
)

func verifyDeleteCommandSuccess(t *testing.T, stdout, stderr string, runErr error, begin, end time.Time) {
	t.Helper()
	if runErr != nil {
		t.Fatalf("expected `delete` command success got err: %v", runErr)
	}
	if stdout == "" {
		t.Fatalf("expected `delete` command stdout to be non-empty, stdout: %v", stdout)
	}
	// don't check stderr, since logs will be printed to it
	var resp task.DeleteResponse
	if err := proto.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("failed to unmarshal stdout to DeleteResponse with err: '%v", err)
	}
	if resp.ExitStatus != 255 {
		t.Fatalf("DeleteResponse exit status is 255 by convention, got: %v", resp.ExitStatus)
	}
	if begin.After(resp.ExitedAt.AsTime()) || end.Before(resp.ExitedAt.AsTime()) {
		t.Fatalf("DeleteResponse.ExitedAt should be between, %v and %v, got: %v", begin, end, resp.ExitedAt)
	}
}

func Test_Delete_No_Bundle_Arg(t *testing.T) {
	stdout, stderr, err := runGlobalCommand(
		t,
		[]string{
			"--namespace", t.Name(),
			"--address", t.Name(),
			"--publish-binary", t.Name(),
			"--id", t.Name(),
			"delete",
		})
	verifyGlobalCommandFailure(
		t,
		"", stdout,
		"bundle is required\n", stderr,
		err)
}

func Test_Delete_No_Bundle_Path(t *testing.T) {
	before := time.Now()
	stdout, stderr, err := runGlobalCommand(
		t,
		[]string{
			"--namespace", t.Name(),
			"--address", t.Name(),
			"--publish-binary", t.Name(),
			"--id", t.Name(),
			"--bundle", "C:\\This\\Path\\Does\\Not\\Exist",
			"delete",
		})
	after := time.Now()
	verifyDeleteCommandSuccess(
		t,
		stdout, stderr, err,
		before, after)
}

func Test_Delete_HcsSystem_NotFound(t *testing.T) {
	// `delete` no longer removes bundle, but still create a directory regardless
	//
	// https://github.com/microsoft/hcsshim/commit/450cdb150a74aa594d7fe63bb0b3a2a37f5dd782
	dir := t.TempDir()

	before := time.Now()
	stdout, stderr, err := runGlobalCommand(
		t,
		[]string{
			"--namespace", t.Name(),
			"--address", t.Name(),
			"--publish-binary", t.Name(),
			"--id", t.Name(),
			"--bundle", dir,
			"delete",
		})
	after := time.Now()
	verifyDeleteCommandSuccess(
		t,
		stdout, stderr, err,
		before, after)
}
