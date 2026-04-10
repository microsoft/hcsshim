//go:build windows && (lcow || wcow)

package process

import (
	"github.com/opencontainers/runtime-spec/specs-go"
)

// CreateOptions holds the parameters for creating a new process.
type CreateOptions struct {
	// Bundle is the path to the OCI bundle directory.
	Bundle string

	// Spec is the OCI process spec for exec processes. For init processes
	// the spec is passed as part of the container config and this is nil.
	Spec *specs.Process

	// Terminal indicates whether the process should allocate a pseudo-TTY.
	// When true, Stderr must be empty because the PTY multiplexes both
	// stdout and stderr onto a single stream.
	Terminal bool

	// Stdin is the named-pipe path for the process's standard input.
	Stdin string

	// Stdout is the named-pipe path for the process's standard output.
	Stdout string

	// Stderr is the named-pipe path for the process's standard error.
	Stderr string
}
