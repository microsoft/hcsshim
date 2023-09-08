package require

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/test/pkg/flag"
)

// Features checks the wanted features are present in given,
// and skips the test if any are missing or explicitly excluded.
// If the given set is empty, the function returns
// (by default, all features are enabled).
//
// See [flag.NewFeatureFlag] and [flag.IncludeExcludeStringSet] for more details.
func Features(tb testing.TB, given *flag.IncludeExcludeStringSet, want ...string) {
	tb.Helper()
	if given.Len() == 0 {
		return
	}
	for _, f := range want {
		if !given.IsSet(f) {
			tb.Skipf("skipping test due to feature not set: %s", f)
		}
	}
}

// AnyFeature checks if at least one of the features are enabled.
//
// See [Features] for more information.
func AnyFeature(tb testing.TB, given *flag.IncludeExcludeStringSet, want ...string) {
	tb.Helper()
	if given.Len() == 0 {
		return
	}
	for _, f := range want {
		if given.IsSet(f) {
			return
		}
	}
	tb.Skipf("skipping test due to missing features: %s", want)
}

// Binary tries to locate `binary` in the PATH (or the current working directory),
// or the same the same directory as the currently-executing binary.
//
// Returns full binary path if it exists, otherwise, skips the test.
func BinaryInPath(tb testing.TB, binary string) string {
	tb.Helper()

	if path, err := exec.LookPath(binary); err == nil || errors.Is(err, exec.ErrDot) {
		p, found, err := fileExists(path)
		if found {
			return p
		}
		tb.Logf("did not find binary %q at path %q: %v", binary, path, err)
	} else {
		tb.Logf("could not look up binary %q: %v", binary, err)
	}

	return Binary(tb, binary)
}

// Binary checks if `binary` exists in the same directory as the test
// binary.
// Returns full binary path if it exists, otherwise, skips the test.
func Binary(tb testing.TB, binary string) string {
	tb.Helper()

	executable, err := os.Executable()
	if err != nil {
		tb.Skipf("error locating executable: %v", err)
		return ""
	}

	baseDir := filepath.Dir(executable)
	return File(tb, baseDir, binary)
}

// File checks if `file` exists in `path`, and returns the full path
// if it exists. Otherwise, it skips the test.
func File(tb testing.TB, path, file string) string {
	tb.Helper()

	p, found, err := fileExists(filepath.Join(path, file))
	if err != nil {
		tb.Fatal(err)
	} else if !found {
		tb.Skipf("file %q not found in %q", file, path)
	}
	return p
}

// fileExists checks if path points to a valid file.
func fileExists(path string) (string, bool, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", false, fmt.Errorf("could not resolve path %q: %w", path, err)
	}

	fi, err := os.Stat(path)
	switch {
	case os.IsNotExist(err):
		return path, false, nil
	case err != nil:
		return "", false, fmt.Errorf("could not stat file %q: %w", path, err)
	case fi.IsDir():
		return "", false, fmt.Errorf("path %q is a directory", path)
	default:
	}

	return path, true, nil
}
