package util

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Microsoft/hcsshim/internal/version"
)

// CleanName returns a string appropriate for uVM, container, or file names.
//
// Based on [testing.TB.TempDir].
func CleanName(n string) string {
	mapper := func(r rune) rune {
		const allowed = "!#$%&()+,-.=@^_{}~ "
		if unicode.IsLetter(r) || unicode.IsNumber(r) || strings.ContainsRune(allowed, r) {
			return r
		}
		return -1
	}
	return strings.TrimSpace(strings.Map(mapper, n))
}

// RunningBenchmarks returns whether benchmarks were requested to be run.
//
// Returning true implies the current executable is a testing binary (either built or run by `go test`).
//
// Should not be called from init() or global variable initialization since there is no guarantee on
// if the testing flags have been defined yet (and ideally should be called after [flag.Parse]).
func RunningBenchmarks() (b bool) {
	// TODO (go1.21): use [testing.Testing] (ref: https://github.com/golang/go/issues/52600) to shortcircut

	// go doesn't run benchmarks unless -test.bench flag is passed, so thats our cue.
	// ref: https://pkg.go.dev/testing#hdr-Benchmarks

	// `go test` (and `go test -c`) runs in a binary with a suite of `-test.*` flags defined.
	// use that as our cue that we are in a testing binary.
	//
	// even if benchmarks are run via `go test -bench='.' `, go will still compile a testing binary
	// and pass the flags as `-test.*`
	f := flag.Lookup("test.bench")
	return f != nil && f.Value.String() != ""
}

// PrintAdditionalBenchmarkConfig outputs additional configuration settings, such as:
//   - start time
//   - number of CPUs available
//   - go version
//   - version information from [github.com/Microsoft/hcsshim/internal/version] (if set)
//
// Benchmark configuration format:
// https://go.googlesource.com/proposal/+/master/design/14313-benchmark-format.md#configuration-lines
//
// For default configuration printed, see: [testing.(*B).Run()] in src/testing/benchmark.go
func PrintAdditionalBenchmarkConfig() {
	// test binaries are not built with module information, so [debug.ReadBuildInfo] will only give
	// the go version (which it ultimately gets via [runtime.Version]) and not provide the "vcs.revision" setting.

	for k, v := range map[string]string{
		// use dedicated os/arch fields (in addition to `OOS & GOARCH) to make this config header non-Go specific
		// and standardize the architecture field.
		"os":   runtime.GOOS,
		"arch": translateGOARCH(runtime.GOARCH),

		"goversion":  runtime.Version(),
		"num-cpu":    strconv.Itoa(runtime.NumCPU()),
		"start-time": time.Now().UTC().Format(time.RFC3339), // ISO 8601
		"command":    strings.Join(os.Args, " "),

		"branch":  version.Branch,
		"commit":  version.Commit,
		"version": version.Version,
	} {
		if k != "" && v != "" {
			fmt.Printf("%s: %s\n", k, v)
		}
	}
}

// translate GOARCH to more "official" values.
//
// mostly for weirdness with how go reports x86 architectures.
// see:
// - https://en.wikipedia.org/wiki/X86
// - https://en.wikipedia.org/wiki/X86-64
func translateGOARCH(s string) string {
	// from src/go/build/syslist.go in go repo
	switch s {
	case "386":
		return "x86"
	case "amd64":
		return "x86_64"
	}
	return s
}
