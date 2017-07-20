// Package realos defines the actual interface into operating system
// functionality.
package realos

import (
	"bufio"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/pkg/errors"
)

// realProcessExitState represents an oslayer.ProcessExitState which uses an
// os.ProcessState for its information.
type realProcessExitState struct {
	state *os.ProcessState
}

// NewProcessExitState returns a *realProcessExitState wrapping the given
// *os.ProcessState.
func NewProcessExitState(state *os.ProcessState) *realProcessExitState {
	return &realProcessExitState{state: state}
}
func (s *realProcessExitState) ExitCode() int {
	return s.state.Sys().(syscall.WaitStatus).ExitStatus()
}

type realFile struct {
	file *os.File
}

type realProcess struct {
	process *os.Process
}

func newProcess(process *os.Process) *realProcess {
	return &realProcess{process: process}
}
func (p *realProcess) Pid() int {
	return p.process.Pid
}

type realCmd struct {
	cmd *exec.Cmd
}

func newCmd(cmd *exec.Cmd) *realCmd {
	return &realCmd{cmd: cmd}
}
func (c *realCmd) SetDir(dir string) {
	c.cmd.Dir = dir
}
func (c *realCmd) SetEnv(env []string) {
	c.cmd.Env = env
}
func (c *realCmd) StdinPipe() (io.WriteCloser, error) {
	pipe, err := c.cmd.StdinPipe()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return pipe, nil
}
func (c *realCmd) StdoutPipe() (io.ReadCloser, error) {
	pipe, err := c.cmd.StdoutPipe()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return pipe, nil
}
func (c *realCmd) StderrPipe() (io.ReadCloser, error) {
	pipe, err := c.cmd.StderrPipe()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return pipe, nil
}
func (c *realCmd) SetStdin(stdin io.Reader) {
	c.cmd.Stdin = stdin
}
func (c *realCmd) SetStdout(stdout io.Writer) {
	c.cmd.Stdout = stdout
}
func (c *realCmd) SetStderr(stderr io.Writer) {
	c.cmd.Stderr = stderr
}
func (c *realCmd) ExitState() oslayer.ProcessExitState {
	return NewProcessExitState(c.cmd.ProcessState)
}
func (c *realCmd) Process() oslayer.Process {
	return newProcess(c.cmd.Process)
}
func (c *realCmd) Start() error {
	if err := c.cmd.Start(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (c *realCmd) Wait() error {
	if err := c.cmd.Wait(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (c *realCmd) Run() error {
	if err := c.cmd.Run(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (c *realCmd) Output() ([]byte, error) {
	out, err := c.cmd.Output()
	if err != nil {
		return out, errors.WithStack(err)
	}
	return out, nil
}
func (c *realCmd) CombinedOutput() ([]byte, error) {
	out, err := c.cmd.CombinedOutput()
	if err != nil {
		return out, errors.WithStack(err)
	}
	return out, nil
}

type realOS struct{}

// NewOS returns a *realOS OS interface implementation which calls into actual
// system OS functionality.
func NewOS() *realOS {
	return &realOS{}
}

// Filesystem
func (o *realOS) OpenFile(name string, flag int, perm os.FileMode) (oslayer.File, error) {
	file, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return file, nil
}
func (o *realOS) Command(name string, arg ...string) oslayer.Cmd {
	return newCmd(exec.Command(name, arg...))
}
func (o *realOS) MkdirAll(path string, perm os.FileMode) error {
	if err := os.MkdirAll(path, perm); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (o *realOS) RemoveAll(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (o *realOS) Create(name string) (oslayer.File, error) {
	file, err := os.Create(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return file, nil
}
func (o *realOS) ReadDir(dirname string) ([]os.FileInfo, error) {
	dirs, err := ioutil.ReadDir(dirname)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return dirs, nil
}
func (o *realOS) Mount(source string, target string, fstype string, flags uintptr, data string) (err error) {
	if err := syscall.Mount(source, target, fstype, flags, data); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (o *realOS) Unmount(target string, flags int) (err error) {
	if err := syscall.Unmount(target, flags); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (o *realOS) PathExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}
	return true, nil
}
func (o *realOS) PathIsMounted(name string) (bool, error) {
	mountinfoFile, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return false, errors.WithStack(err)
	}
	defer mountinfoFile.Close()

	scanner := bufio.NewScanner(mountinfoFile)
	for scanner.Scan() {
		tokens := strings.Fields(scanner.Text())
		dir1 := tokens[3]
		dir2 := tokens[4]
		if name == dir1 || name == dir2 {
			return true, nil
		}
	}
	return false, nil
}
func (o *realOS) Link(oldname, newname string) error {
	if err := os.Link(oldname, newname); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Processes
func (o *realOS) Kill(pid int, sig syscall.Signal) error {
	if err := syscall.Kill(pid, sig); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
