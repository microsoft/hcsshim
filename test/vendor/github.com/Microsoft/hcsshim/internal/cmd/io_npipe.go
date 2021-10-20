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
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

const defaultIOReconnectTimeout time.Duration = 10 * time.Second

// NewNpipeIO creates connected upstream io. It is the callers responsibility to
// validate that `if terminal == true`, `stderr == ""`.
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

	if retryTimeout == 0 {
		retryTimeout = defaultIOReconnectTimeout
	}

	if stdin != "" {
		c, err := winio.DialPipeContext(ctx, stdin)
		if err != nil {
			return nil, err
		}
		nio.sin = c
	}
	if stdout != "" {
		c, err := winio.DialPipeContext(ctx, stdout)
		if err != nil {
			return nil, err
		}
		nio.sout = &nPipeRetryWriter{c, stdout, retryTimeout}
	}
	if stderr != "" {
		c, err := winio.DialPipeContext(ctx, stderr)
		if err != nil {
			return nil, err
		}
		nio.serr = &nPipeRetryWriter{c, stderr, retryTimeout}
	}
	return nio, nil
}

type nPipeRetryWriter struct {
	net.Conn
	pipePath string
	timeout  time.Duration
}

func (nprw *nPipeRetryWriter) Write(p []byte) (n int, err error) {
	for {
		n, err = nprw.Conn.Write(p)
		if err != nil {
			// If the error is one that we can discern calls for a retry, attempt to redial the pipe.
			if isDisconnectedErr(err) {
				retryCtx, cancel := context.WithTimeout(context.Background(), nprw.timeout)
				defer cancel()

				// Close the old conn.
				nprw.Conn.Close()
				newConn, retryErr := retryDialPipe(retryCtx, nprw.pipePath)
				if retryErr == nil {
					nprw.Conn = newConn
					continue
				}
			}
		}
		return
	}
}

// isDisconnectedErr is a helper to determine if the error received from writing to the server end of a named pipe indicates a disconnect/severed
// connection. This can be used to attempt a redial if it's expected that the server will come back online at some point.
func isDisconnectedErr(err error) bool {
	if serr, ok := err.(syscall.Errno); ok {
		// Server went away/something went wrong.
		return serr == windows.ERROR_NO_DATA || serr == windows.ERROR_PIPE_NOT_CONNECTED || serr == windows.ERROR_BROKEN_PIPE
	}
	return false
}

// retryDialPipe is a helper to retry dialing a named pipe until context timeout or a successful connection. This is mainly to
// assist in scenarios where the server end of the pipe has crashed/went away and is no longer accepting new connections but may
// come back online.
func retryDialPipe(ctx context.Context, path string) (net.Conn, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("failed to reconnect to IO pipe within timeout: %w", ctx.Err())
		default:
			conn, err := winio.DialPipe(path, nil)
			if err == nil {
				return conn, nil
			}
			time.Sleep(time.Millisecond * 100)
		}
	}
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
