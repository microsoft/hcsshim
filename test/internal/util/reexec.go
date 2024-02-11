package util

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Tests may need to run code from a different process or scope (e.g., from within a container or uVM).
// Rather than creating a dedicated binary per testcase/situtation, allow tests to re-exec
// the current testing binary and only run a dedicated code block.
// This leverages builtin `go test -run <regex>` functionality, consolidates code
// so that the test and re-exec code can be updated in tandem, and removes the need to build
// and manage additional utility binaries for testing.
//
// Inspired by pipe tests in golang source:
// https://cs.opensource.google/go/go/+/master:src/os/pipe_test.go;l=266-273;drc=0dfb22ed70749a2cd6d95ec6eee63bb213a940d4
//
// Tests that re-exec themselves should set the [ReExecEnv] environment variable in the new process
// to skip testing setup and allow [RunInReExec] to function properly.
// Additionally, [TestNameRegex] should be used to run only the current test case in the re-exec.

// ReExecEnv is used to indicate that the current testing binary has been re-execed.
//
// Tests should set this environment variable before re-execing themselves.
const ReExecEnv = "HCSSHIM_TEST_RE_EXEC"

// IsTestReExec checks if the current test execution is a re-exec of a testing binary.
// I.e., it checks if the [ReExecEnv] environment variable is set.
func IsTestReExec() bool {
	if !Testing() {
		return false
	}

	_, ok := os.LookupEnv(ReExecEnv)
	return ok
}

// RunInReExec checks if it is executing in within a re-exec (via [IsTestReExec])
// and, if so, calls f and then [testing.TB.Skip] to skip the remainder of the test.
func RunInReExec(ctx context.Context, tb testing.TB, f func(context.Context, testing.TB)) {
	tb.Helper()

	if !IsTestReExec() {
		return
	}

	f(ctx, tb)
	tb.Skip("finished running code from re-exec")
}

// TestNameRegex returns a regex expresion that matches the current test name exactly.
//
// `-test.run regex` matches the individual test name components be splitting on `/`.
// So `A/B` will first match test names against `A`, and then, for all matched tests,
// match sub-tests against `B`.
// Therefore, for a test named `foo/bar`, return `^foo$/^bar$`.
//
// See: `go help test`.
func TestNameRegex(tb testing.TB) string {
	tb.Helper()

	ss := make([]string, 0)
	for _, s := range strings.Split(tb.Name(), `/`) {
		ss = append(ss, `^`+s+`$`)
	}
	return strings.Join(ss, `/`)
}
