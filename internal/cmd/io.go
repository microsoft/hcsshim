package cmd

import (
	"context"
	"io"
	"net/url"

	"github.com/pkg/errors"
)

// UpstreamIO is an interface describing the IO to connect to above the shim.
// Depending on the callers settings there may be no opened IO.
type UpstreamIO interface {
	// Close closes all open io.
	//
	// This call is idempotent and safe to call multiple times.
	Close(ctx context.Context)
	// CloseStdin closes just `Stdin()` if open.
	//
	// This call is idempotent and safe to call multiple times.
	CloseStdin(ctx context.Context)
	// Stdin returns the open `stdin` reader. If `stdin` was never opened this
	// will return `nil`.
	Stdin() io.Reader
	// StdinPath returns the original path used to open the `Stdin()` reader.
	StdinPath() string
	// Stdout returns the open `stdout` writer. If `stdout` was never opened
	// this will return `nil`.
	Stdout() io.Writer
	// StdoutPath returns the original path used to open the `Stdout()` writer.
	StdoutPath() string
	// Stderr returns the open `stderr` writer. If `stderr` was never opened
	// this will return `nil`.
	Stderr() io.Writer
	// StderrPath returns the original path used to open the `Stderr()` writer.
	StderrPath() string
	// Terminal returns `true` if the connection is emulating a terminal. If
	// `true` `Stderr()` will always return `nil` and `StderrPath()` will always
	// return `""`.
	Terminal() bool
}

// NewUpstreamIO returns an UpstreamIO instance. Currently we only support named pipes and binary
// logging driver for container IO. When using binary logger `stdout` and `stderr` are assumed to be
// the same and the value of `stderr` is completely ignored.
func NewUpstreamIO(ctx context.Context, id string, stdout string, stderr string, stdin string, terminal bool) (UpstreamIO, error) {
	u, err := url.Parse(stdout)

	// Create IO with named pipes.
	if err != nil || u.Scheme == "" {
		return NewNpipeIO(ctx, stdin, stdout, stderr, terminal)
	}

	// Create IO for binary logging driver.
	if u.Scheme != "binary" {
		return nil, errors.Errorf("scheme must be 'binary', got: '%s'", u.Scheme)
	}

	return NewBinaryIO(ctx, id, u)
}
