package require

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/test/internal/flag"
)

// Features checks the wanted features are present in given,
// and skips the test if any are missing. If the given set is empty,
// the function returns (by default all features are enabled).
func Features(tb testing.TB, given flag.StringSet, want ...string) {
	tb.Helper()
	if len(given) == 0 {
		return
	}
	for _, f := range want {
		ff := flag.Standardize(f)
		if _, ok := given[ff]; !ok {
			tb.Skipf("skipping test due to feature not set: %s", f)
		}
	}
}

// Binary checks if `binary` exists in the same directory as the test
// binary.
// Returns full binary path if it exists, otherwise, skips the test.
func Binary(tb testing.TB, binary string) string {
	tb.Helper()
	executable, err := os.Executable()
	if err != nil {
		tb.Skipf("error locating executable: %s", err)
		return ""
	}

	baseDir := filepath.Dir(executable)
	return File(tb, baseDir, binary)
}

// File checks if `file` exists in `path`, and returns the full path
// if it exists. Otherwise, it skips the test.
func File(tb testing.TB, path, file string) string {
	tb.Helper()
	path, err := filepath.Abs(path)
	if err != nil {
		tb.Fatalf("could not resolve path %q: %v", path, err)
	}

	p := filepath.Join(path, file)
	fi, err := os.Stat(p)
	switch {
	case os.IsNotExist(err):
		tb.Skipf("file %q not found", p)
	case err != nil:
		tb.Fatalf("could not stat file %q: %v", p, err)
	case fi.IsDir():
		tb.Fatalf("path %q is a directory", p)
	default:
	}

	return p
}
