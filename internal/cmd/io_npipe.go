//go:build windows

package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/cenkalti/backoff/v4"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

// NewNpipeIO creates connected upstream io. It is the callers responsibility to validate that `if terminal == true`, `stderr == ""`. retryTimeout
// refers to the timeout used to try and reconnect to the server end of the named pipe if the connection is severed. A value of 0 for retryTimeout
// is treated as an infinite timeout.
func NewNpipeIO(ctx context.Context, stdin, stdout, stderr string, terminal bool, retryTimeout time.Duration) (_ UpstreamIO, err error) {
	log.G(ctx).WithFields(logrus.Fields{
		"stdin":    stdin,
		"stdout":   stdout,
		"stderr":   stderr,
		"terminal": terminal,
	}).Debug("NewNpipeIO")

	nio := &npipeio{
		stdin:    stdin,
		stdout:   stdout,
		stderr:   stderr,
		terminal: terminal,
	}
	defer func() {
		if err != nil {
			nio.Close(ctx)
		}
	}()

	if stdin != "" {
		c, err := winio.DialPipeContext(ctx, stdin)
		if err != nil {
			return nil, err
		}
		// We don't have any retry logic for stdin as there's no good way to detect that we'd even need to retry. If the process forwarding
		// stdin to the container (some client interface to exec a process in a container) exited, we'll get EOF which io.Copy treats as
		// success. For fifos on Linux it seems if all fd's for the write end of the pipe disappear, which is the same scenario, then
		// the read end will get EOF as well.
		nio.sin = c
	}
	if stdout != "" {
		c, err := winio.DialPipeContext(ctx, stdout)
		if err != nil {
			return nil, err
		}
		nio.sout = &nPipeRetryWriter{ctx, c, stdout, newBackOff(retryTimeout)}
	}
	if stderr != "" {
		c, err := winio.DialPipeContext(ctx, stderr)
		if err != nil {
			return nil, err
		}
		nio.serr = &nPipeRetryWriter{ctx, c, stderr, newBackOff(retryTimeout)}
	}
	return nio, nil
}

// nPipeRetryWriter is an io.Writer that wraps a net.Conn representing a named pipe connection. The retry logic is specifically only for
// disconnect scenarios (pipe broken, server went away etc.) to attempt to re-establish a connection, and is not for retrying writes on a busy pipe.
type nPipeRetryWriter struct {
	ctx context.Context
	net.Conn
	pipePath string
	backOff  backoff.BackOff
}

// newBackOff returns a new BackOff interface. The values chosen are fairly conservative, the main use is to get a somewhat random
// retry timeout on each ask. This can help avoid flooding a server all at once.
func newBackOff(timeout time.Duration) backoff.BackOff {
	return &backoff.ExponentialBackOff{
		// First backoff timeout will be somewhere in the 100 - 300 ms range given the default multiplier.
		InitialInterval:     time.Millisecond * 200,
		RandomizationFactor: backoff.DefaultRandomizationFactor,
		Multiplier:          backoff.DefaultMultiplier,
		// Set the max interval to a minute, seems like a sane value. We don't know how long the server will be down for, and if we reached
		// this point it's been down for quite awhile.
		MaxInterval: time.Minute * 1,
		// `backoff.ExponentialBackoff` treats a 0 timeout as infinite, which is ideal as it's the logic we desire.
		MaxElapsedTime: timeout,
		Stop:           backoff.Stop,
		Clock:          backoff.SystemClock,
	}
}

func (nprw *nPipeRetryWriter) Write(p []byte) (n int, err error) {
	var currBufPos int
	for {
		// p[currBufPos:] to handle a case where we wrote n bytes but got disconnected and now we just need to write the rest of the buffer. If this is the
		// first write then the current position is 0 so we just try and write the whole buffer as usual.
		n, err = nprw.Conn.Write(p[currBufPos:])
		currBufPos += n
		if err != nil {
			// If the error is one that we can discern calls for a retry, attempt to redial the pipe.
			if isDisconnectedErr(err) {
				// Log that we're going to retry establishing the connection.
				log.G(nprw.ctx).WithFields(logrus.Fields{
					"address":       nprw.pipePath,
					logrus.ErrorKey: err,
				}).Error("Named pipe disconnected, retrying dial")

				// Close the old conn first.
				nprw.Conn.Close()
				newConn, retryErr := nprw.retryDialPipe()
				if retryErr == nil {
					log.G(nprw.ctx).WithField("address", nprw.pipePath).Info("Succeeded in reconnecting to named pipe")

					nprw.Conn = newConn
					continue
				}
				err = retryErr
			}
		}
		return currBufPos, err
	}
}

// retryDialPipe is a helper to retry dialing a named pipe until the timeout of nprw.BackOff or a successful connection. This is mainly to
// assist in scenarios where the server end of the pipe has crashed/went away and is no longer accepting new connections but may
// come back online. The backoff used inside is to try and space out connections to the server as to not flood it all at once with connection
// attempts at the same interval.
func (nprw *nPipeRetryWriter) retryDialPipe() (net.Conn, error) {
	// Reset the backoff object as it starts ticking down when it's created. This also ensures we can re-use it in the event the server goes
	// away more than once.
	nprw.backOff.Reset()
	for {
		backOffTime := nprw.backOff.NextBackOff()
		// We don't simply use a context with a timeout and pass it to DialPipe because DialPipe only retries the connection (and thus makes use of
		// the timeout) if it sees that the pipe is busy. If the server isn't up/not listening it will just error out immediately and not make use
		// of the timeout passed. That's the case we're most likely in right now so we need our own retry logic on top.
		conn, err := winio.DialPipe(nprw.pipePath, nil)
		if err == nil {
			return conn, nil
		}
		// Next backoff would go over our timeout. We've tried once more above due to the ordering of this check, but now we need to bail out.
		if backOffTime == backoff.Stop {
			return nil, fmt.Errorf("reached timeout while retrying dial on %s", nprw.pipePath)
		}
		time.Sleep(backOffTime)
	}
}

// isDisconnectedErr is a helper to determine if the error received from writing to the server end of a named pipe indicates a disconnect/severed
// connection. This can be used to attempt a redial if it's expected that the server will come back online at some point.
func isDisconnectedErr(err error) bool {
	if serr, ok := err.(syscall.Errno); ok { //nolint:errorlint
		// Server went away/something went wrong.
		return serr == windows.ERROR_NO_DATA || serr == windows.ERROR_PIPE_NOT_CONNECTED || serr == windows.ERROR_BROKEN_PIPE
	}
	return false
}

var _ = (UpstreamIO)(&npipeio{})

type npipeio struct {
	// stdin, stdout, stderr are the original paths used to open the connections.
	//
	// They MUST be treated as readonly in the lifetime of the pipe io.
	stdin, stdout, stderr string
	// terminal is the original setting passed in on open.
	//
	// This MUST be treated as readonly in the lifetime of the pipe io.
	terminal bool

	// sin is the upstream `stdin` connection.
	//
	// `sin` MUST be treated as readonly in the lifetime of the pipe io after
	// the return from `NewNpipeIO`.
	sin       io.ReadCloser
	sinCloser sync.Once

	// sout and serr are the upstream `stdout` and `stderr` connections.
	//
	// `sout` and `serr` MUST be treated as readonly in the lifetime of the pipe
	// io after the return from `NewNpipeIO`.
	sout, serr   io.WriteCloser
	outErrCloser sync.Once
}

func (nio *npipeio) Close(ctx context.Context) {
	nio.sinCloser.Do(func() {
		if nio.sin != nil {
			log.G(ctx).Debug("npipeio::sinCloser")
			nio.sin.Close()
		}
	})
	nio.outErrCloser.Do(func() {
		if nio.sout != nil {
			log.G(ctx).Debug("npipeio::outErrCloser - stdout")
			nio.sout.Close()
		}
		if nio.serr != nil {
			log.G(ctx).Debug("npipeio::outErrCloser - stderr")
			nio.serr.Close()
		}
	})
}

func (nio *npipeio) CloseStdin(ctx context.Context) {
	nio.sinCloser.Do(func() {
		if nio.sin != nil {
			log.G(ctx).Debug("npipeio::sinCloser")
			nio.sin.Close()
		}
	})
}

func (nio *npipeio) Stdin() io.Reader {
	return nio.sin
}

func (nio *npipeio) StdinPath() string {
	return nio.stdin
}

func (nio *npipeio) Stdout() io.Writer {
	return nio.sout
}

func (nio *npipeio) StdoutPath() string {
	return nio.stdout
}

func (nio *npipeio) Stderr() io.Writer {
	return nio.serr
}

func (nio *npipeio) StderrPath() string {
	return nio.stderr
}

func (nio *npipeio) Terminal() bool {
	return nio.terminal
}

// CreatePipeAndListen is a helper function to create a pipe listener
// and accept connections. Returns the created pipe path on success.
//
// If `in` is true, `f` should implement io.Reader
// If `in` is false, `f` should implement io.Writer
func CreatePipeAndListen(f interface{}, in bool) (string, error) {
	p, l, err := CreateNamedPipeListener()
	if err != nil {
		return "", err
	}
	go func() {
		c, err := l.Accept()
		if err != nil {
			logrus.WithError(err).Error("failed to accept pipe")
			return
		}

		if in {
			_, _ = io.Copy(c, f.(io.Reader))
			c.Close()
		} else {
			_, _ = io.Copy(f.(io.Writer), c)
		}
	}()
	return p, nil
}

// CreateNamedPipeListener is a helper function to create and return a pipe listener
// and it's created path.
func CreateNamedPipeListener() (string, net.Listener, error) {
	g, err := guid.NewV4()
	if err != nil {
		return "", nil, err
	}
	p := `\\.\pipe\` + g.String()
	l, err := winio.ListenPipe(p, nil)
	if err != nil {
		return "", nil, err
	}
	return p, l, nil
}
