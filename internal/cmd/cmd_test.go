// build +windows

package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/cow"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

type localProcessHost struct {
}

type localProcess struct {
	p                     *os.Process
	state                 *os.ProcessState
	ch                    chan struct{}
	stdin, stdout, stderr *os.File
}

func (h *localProcessHost) OS() string {
	return "windows"
}

func (h *localProcessHost) IsOCI() bool {
	return false
}

func (h *localProcessHost) CreateProcess(ctx context.Context, cfg interface{}) (_ cow.Process, err error) {
	params := cfg.(*hcsschema.ProcessParameters)
	lp := &localProcess{ch: make(chan struct{})}
	defer func() {
		if err != nil {
			lp.Close()
		}
	}()
	var stdin, stdout, stderr *os.File
	if params.CreateStdInPipe {
		stdin, lp.stdin, err = os.Pipe()
		if err != nil {
			return nil, err
		}
		defer stdin.Close()
	}
	if params.CreateStdOutPipe {
		lp.stdout, stdout, err = os.Pipe()
		if err != nil {
			return nil, err
		}
		defer stdout.Close()
	}
	if params.CreateStdErrPipe {
		lp.stderr, stderr, err = os.Pipe()
		if err != nil {
			return nil, err
		}
		defer stderr.Close()
	}
	path := strings.Split(params.CommandLine, " ")[0] // should be fixed for non-test use...
	if ppath, err := exec.LookPath(path); err == nil {
		path = ppath
	}
	lp.p, err = os.StartProcess(path, nil, &os.ProcAttr{
		Files: []*os.File{stdin, stdout, stderr},
		Sys: &syscall.SysProcAttr{
			CmdLine: params.CommandLine,
		},
	})
	if err != nil {
		return nil, err
	}
	go func() {
		lp.state, _ = lp.p.Wait()
		close(lp.ch)
	}()
	return lp, nil
}

func (p *localProcess) Close() error {
	if p.p != nil {
		_ = p.p.Release()
	}
	if p.stdin != nil {
		p.stdin.Close()
	}
	if p.stdout != nil {
		p.stdout.Close()
	}
	if p.stderr != nil {
		p.stderr.Close()
	}
	return nil
}

func (p *localProcess) CloseStdin(ctx context.Context) error {
	return p.stdin.Close()
}

func (p *localProcess) CloseStdout(ctx context.Context) error {
	return p.stdout.Close()
}

func (p *localProcess) CloseStderr(ctx context.Context) error {
	return p.stderr.Close()
}

func (p *localProcess) ExitCode() (int, error) {
	select {
	case <-p.ch:
		return p.state.ExitCode(), nil
	default:
		return -1, errors.New("not exited")
	}
}

func (p *localProcess) Kill(ctx context.Context) (bool, error) {
	return true, p.p.Kill()
}

func (p *localProcess) Signal(ctx context.Context, _ interface{}) (bool, error) {
	return p.Kill(ctx)
}

func (p *localProcess) Pid() int {
	return p.p.Pid
}

func (p *localProcess) ResizeConsole(ctx context.Context, x, y uint16) error {
	return errors.New("not supported")
}

func (p *localProcess) Stdio() (io.Writer, io.Reader, io.Reader) {
	return p.stdin, p.stdout, p.stderr
}

func (p *localProcess) Wait() error {
	<-p.ch
	return nil
}

func TestCmdExitCode(t *testing.T) {
	cmd := Command(&localProcessHost{}, "cmd", "/c", "exit", "/b", "64")
	err := cmd.Run()
	if e, ok := err.(*ExitError); !ok || e.ExitCode() != 64 {
		t.Fatal("expected exit code 64, got ", err)
	}
}

func TestCmdOutput(t *testing.T) {
	cmd := Command(&localProcessHost{}, "cmd", "/c", "echo", "hello")
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "hello\r\n" {
		t.Fatalf("got %q", string(output))
	}
}

func TestCmdContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	cmd := CommandContext(ctx, &localProcessHost{}, "cmd", "/c", "pause")
	r, w := io.Pipe()
	cmd.Stdin = r
	err := cmd.Start()
	if err != nil {
		t.Fatal(err)
	}
	_ = cmd.Process.Wait()
	w.Close()
	err = cmd.Wait()
	if e, ok := err.(*ExitError); !ok || e.ExitCode() != 1 || ctx.Err() == nil {
		t.Fatal(err)
	}
}

func TestCmdStdin(t *testing.T) {
	cmd := Command(&localProcessHost{}, "findstr", "x*")
	cmd.Stdin = bytes.NewBufferString("testing 1 2 3")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "testing 1 2 3\r\n" {
		t.Fatalf("got %q", string(out))
	}
}

func TestCmdStdinBlocked(t *testing.T) {
	cmd := Command(&localProcessHost{}, "cmd", "/c", "pause")
	r, w := io.Pipe()
	defer r.Close()
	go func() {
		b := []byte{'\n'}
		_, _ = w.Write(b)
	}()
	cmd.Stdin = r
	_, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
}

type stuckIoProcessHost struct {
	cow.ProcessHost
}

type stuckIoProcess struct {
	cow.Process
	stdin, pstdout, pstderr *io.PipeWriter
	pstdin, stdout, stderr  *io.PipeReader
}

func (h *stuckIoProcessHost) CreateProcess(ctx context.Context, cfg interface{}) (cow.Process, error) {
	p, err := h.ProcessHost.CreateProcess(ctx, cfg)
	if err != nil {
		return nil, err
	}
	sp := &stuckIoProcess{
		Process: p,
	}
	sp.pstdin, sp.stdin = io.Pipe()
	sp.stdout, sp.pstdout = io.Pipe()
	sp.stderr, sp.pstderr = io.Pipe()
	return sp, nil
}

func (p *stuckIoProcess) Stdio() (io.Writer, io.Reader, io.Reader) {
	return p.stdin, p.stdout, p.stderr
}

func (p *stuckIoProcess) Close() error {
	p.stdin.Close()
	p.stdout.Close()
	p.stderr.Close()
	return p.Process.Close()
}

func TestCmdStuckIo(t *testing.T) {
	cmd := Command(&stuckIoProcessHost{&localProcessHost{}}, "cmd", "/c", "echo", "hello")
	cmd.CopyAfterExitTimeout = time.Millisecond * 200
	_, err := cmd.Output()
	if err != io.ErrClosedPipe {
		t.Fatal(err)
	}
}
