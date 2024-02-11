//go:build windows

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/hcs"
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
func NewUpstreamIO(ctx context.Context, id, stdout, stderr, stdin string, terminal bool, ioRetryTimeout time.Duration) (UpstreamIO, error) {
	u, err := url.Parse(stdout)

	// Create IO with named pipes.
	if err != nil || u.Scheme == "" {
		return NewNpipeIO(ctx, stdin, stdout, stderr, terminal, ioRetryTimeout)
	}

	// Create IO for binary logging driver.
	if u.Scheme != "binary" {
		return nil, fmt.Errorf("scheme must be 'binary', got: '%s'", u.Scheme)
	}

	return NewBinaryIO(ctx, id, u)
}

// isClosedIOErr checks if the error is from the underlying file or pipe already being closed.
func isClosedIOErr(err error) bool {
	for _, e := range []error{
		os.ErrClosed,
		net.ErrClosed,
		io.ErrClosedPipe,
		winio.ErrFileClosed,
		hcs.ErrAlreadyClosed,
	} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// relayIO is a glorified io.Copy that also logs when the copy has completed.
//
// It will ignore errors raised during the copy from attempting to read from
// (or write to) a closed io.Reader (or Writer, respectively).
// Ideally, this would not be necessary, since the command's stdout and stderr would
// send an EOF first before closing, but that is not always the case (eg, [jobcontainer.JobProcess]
// uses unnamed pipes, which do not support EOF).
// Additionally, we do not prevent writing to the stdin of a closed Cmd, so there could be a race
// between reading the upstream stdin, the command finishing, and attempting to write to the command's
// stdin writer.
//
// See [isClosedIOErr] for the errors that are ignored.
func relayIO(w io.Writer, r io.Reader, log *logrus.Entry, name string) (int64, error) {
	n, err := io.Copy(w, r)
	if log != nil {
		lvl := logrus.DebugLevel
		log = log.WithFields(logrus.Fields{
			"file":  name,
			"bytes": n,
		})
		if isClosedIOErr(err) {
			log.WithError(err).Trace("ignoring closed IO error")
			err = nil
		}
		if err != nil {
			lvl = logrus.ErrorLevel
			log = log.WithError(err)
		}
		log.Log(lvl, "Cmd IO relay complete")
	}
	return n, err
}
