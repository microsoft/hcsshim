package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/containerd/containerd/namespaces"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/log"
)

const (
	binaryPipeFmt         = `\\.\pipe\binary-%s-%s`
	binaryCmdWaitTimeout  = 10 * time.Second
	binaryCmdStartTimeout = 10 * time.Second
)

// NewBinaryIO runs a custom binary process for pluggable shim logging driver.
//
// Container's IO will be redirected to the logging driver via named pipes, which are
// passed as "CONTAINER_STDOUT", "CONTAINER_STDERR" environment variables. The logging
// driver MUST dial a wait pipe passed via "CONTAINER_WAIT" environment variable AND CLOSE
// it to indicate that it's ready to consume the IO. For customer's convenience container ID
// and namespace are also passed via "CONTAINER_ID" and "CONTAINER_NAMESPACE".
//
// The path to the logging driver can be provided via a URL's host/path. Additional arguments
// can be passed to the logger via URL query params
func NewBinaryIO(ctx context.Context, id string, uri *url.URL) (_ UpstreamIO, err error) {
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		ns = namespaces.Default
	}

	var stdoutPipe, stderrPipe, waitPipe io.ReadWriteCloser

	stdoutPipePath := fmt.Sprintf(binaryPipeFmt, id, "stdout")
	stdoutPipe, err = openNPipe(stdoutPipePath)
	if err != nil {
		return nil, err
	}

	stderrPipePath := fmt.Sprintf(binaryPipeFmt, id, "stderr")
	stderrPipe, err = openNPipe(stderrPipePath)
	if err != nil {
		return nil, err
	}

	waitPipePath := fmt.Sprintf(binaryPipeFmt, id, "wait")
	waitPipe, err = openNPipe(waitPipePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := waitPipe.Close(); err != nil {
			log.G(ctx).WithError(err).Errorf("error closing wait pipe: %s", waitPipePath)
		}
	}()

	envs := []string{
		"CONTAINER_ID=" + id,
		"CONTAINER_NAMESPACE=" + ns,
		"CONTAINER_STDOUT=" + stdoutPipePath,
		"CONTAINER_STDERR=" + stderrPipePath,
		"CONTAINER_WAIT=" + waitPipePath,
	}
	cmd, err := newBinaryCmd(ctx, uri, envs)
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	errCh := make(chan error, 1)
	// Wait for logging driver to signal to the wait pipe that it's ready to consume IO
	go func() {
		b := make([]byte, 1)
		if _, err := waitPipe.Read(b); err != nil && err != io.EOF {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err = <-errCh:
		if err != nil {
			return nil, errors.Wrap(err, "failed to start binary logger")
		}
	case <-time.After(binaryCmdStartTimeout):
		return nil, errors.New("failed to start binary logger: timeout")
	}

	log.G(ctx).WithFields(logrus.Fields{
		"containerID":        id,
		"containerNamespace": ns,
		"binaryCmd":          cmd.String(),
		"binaryProcessID":    cmd.Process.Pid,
	}).Debug("binary io process started")

	return &binaryIO{
		cmd:    cmd,
		stdout: stdoutPipePath,
		sout:   stdoutPipe,
		stderr: stderrPipePath,
		serr:   stderrPipe,
	}, nil
}

// sanitizePath parses the URL object and returns a clean path to the logging driver
func sanitizePath(uri *url.URL) string {
	path := filepath.Clean(uri.Path)

	if strings.Contains(path, `:\`) {
		return strings.TrimPrefix(path, "\\")
	}

	return path
}

func newBinaryCmd(ctx context.Context, uri *url.URL, envs []string) (*exec.Cmd, error) {
	if uri.Path == "" {
		return nil, errors.New("no logging driver path provided")
	}

	var args []string
	for k, vs := range uri.Query() {
		args = append(args, k)
		if len(vs) > 0 && vs[0] != "" {
			args = append(args, vs[0])
		}
	}

	execPath := sanitizePath(uri)

	cmd := exec.CommandContext(ctx, execPath, args...)
	cmd.Env = append(cmd.Env, envs...)

	return cmd, nil
}

var _ UpstreamIO = &binaryIO{}

// Implements UpstreamIO interface to enable shim pluggable logging
type binaryIO struct {
	cmd *exec.Cmd

	binaryCloser sync.Once

	stdout, stderr string

	sout, serr io.ReadWriteCloser
	soutCloser sync.Once
}

// Close named pipes for container stdout and stderr and wait for the binary process to finish.
func (b *binaryIO) Close(ctx context.Context) {
	b.soutCloser.Do(func() {
		if b.sout != nil {
			err := b.sout.Close()
			if err != nil {
				log.G(ctx).WithError(err).Errorf("error while closing stdout npipe")
			}
		}
		if b.serr != nil {
			err := b.serr.Close()
			if err != nil {
				log.G(ctx).WithError(err).Errorf("error while closing stderr npipe")
			}
		}
	})
	b.binaryCloser.Do(func() {
		done := make(chan error, 1)
		go func() {
			done <- b.cmd.Wait()
		}()

		select {
		case err := <-done:
			if err != nil {
				log.G(ctx).WithError(err).Errorf("error while waiting for binary cmd to finish")
			}
		case <-time.After(binaryCmdWaitTimeout):
			log.G(ctx).Errorf("timeout while waiting for binaryIO process to finish. Killing")
			err := b.cmd.Process.Kill()
			if err != nil {
				log.G(ctx).WithError(err).Errorf("error while killing binaryIO process")
			}
		}
	})
}

func (b *binaryIO) CloseStdin(_ context.Context) {}

func (b *binaryIO) Stdin() io.Reader {
	return nil
}

func (b *binaryIO) StdinPath() string {
	return ""
}

func (b *binaryIO) Stdout() io.Writer {
	return b.sout
}

func (b *binaryIO) StdoutPath() string {
	return b.stdout
}

func (b *binaryIO) Stderr() io.Writer {
	return b.serr
}

func (b *binaryIO) StderrPath() string {
	return b.stderr
}

func (b *binaryIO) Terminal() bool {
	return false
}

type pipe struct {
	l      net.Listener
	con    net.Conn
	conErr error
	conWg  sync.WaitGroup
}

func openNPipe(path string) (io.ReadWriteCloser, error) {
	l, err := winio.ListenPipe(path, nil)
	if err != nil {
		return nil, err
	}

	p := &pipe{l: l}
	p.conWg.Add(1)

	go func() {
		defer p.conWg.Done()
		c, err := l.Accept()
		if err != nil {
			p.conErr = err
			return
		}
		p.con = c
	}()
	return p, nil
}

func (p *pipe) Write(b []byte) (int, error) {
	p.conWg.Wait()
	if p.conErr != nil {
		return 0, errors.Wrap(p.conErr, "connection error")
	}
	return p.con.Write(b)
}

func (p *pipe) Read(b []byte) (int, error) {
	p.conWg.Wait()
	if p.conErr != nil {
		return 0, errors.Wrap(p.conErr, "connection error")
	}
	return p.con.Read(b)
}

func (p *pipe) Close() error {
	if err := p.l.Close(); err != nil {
		log.G(context.TODO()).WithError(err).Debug("error closing pipe listener")
	}
	p.conWg.Wait()
	if p.con != nil {
		return p.con.Close()
	}
	return p.conErr
}
