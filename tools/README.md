# Tools

This package contains imports to various tools used (eg, via `//go:generate`) within the hcsshim repo,
allowing them to be versioned and ensuring their dependencies match what the shim use
(specifically for auto-generated protobuf code).

Calls to `go run <cmd/import/path>` (or `//go:generate go run <cmd/import/path>`) for go executables
included here  will use the version specified in `go.mod` and build the executable from vendored code.

Using a dedicate package prevents callers who import `github.com/Microsoft/hcsshim` from including these
tools in their dependencies.

Based on golang [guidance].

## Adding Dependencies

To add a new dependency, add a `_ "cmd/import/path"` to `tools.go`, and then tidy and vendor the repo.

In general executables used in auto-generating code (eg, `protobuild`, `protoc-gen-go-*`, and co.), or testing
(eg, `gotestsum`, `benchstat`) should be included here.

[guidance]: https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
