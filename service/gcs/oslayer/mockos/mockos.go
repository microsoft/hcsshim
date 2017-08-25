// Package mockos defines a mock interface into operating system functionality.
package mockos

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/transport"
)

type mockReadWriteCloser struct {
	*bytes.Buffer
}

// NewMockReadWriteCloser returns a mockReadWriteCloser over an empty buffer.
func NewMockReadWriteCloser() transport.Connection {
	return &mockReadWriteCloser{Buffer: new(bytes.Buffer)}
}
func (c *mockReadWriteCloser) Close() error {
	return nil
}
func (c *mockReadWriteCloser) CloseRead() error {
	return nil
}
func (c *mockReadWriteCloser) CloseWrite() error {
	return nil
}
func (c *mockReadWriteCloser) File() (*os.File, error) {
	return nil, errors.New("not implemented")
}

type mockProcessExitState struct {
	exitCode int
}

// NewProcessExitState returns a mockProcessExitState with the given exit
// code.
func NewProcessExitState(exitCode int) oslayer.ProcessExitState {
	return &mockProcessExitState{exitCode: exitCode}
}
func (s *mockProcessExitState) ExitCode() int {
	return s.exitCode
}

type mockFile struct {
	name string
	flag int
	perm os.FileMode
}

func newFile(name string, flag int, perm os.FileMode) *mockFile {
	return &mockFile{name: name, flag: flag, perm: perm}
}
func (f *mockFile) Read(p []byte) (n int, err error) {
	return len(p), nil
}
func (f *mockFile) Write(p []byte) (n int, err error) {
	return len(p), nil
}
func (f *mockFile) Close() error {
	return nil
}

type mockProcess struct {
	pid int
}

func newProcess(pid int) *mockProcess {
	return &mockProcess{pid: pid}
}
func (p *mockProcess) Pid() int {
	return p.pid
}

type mockCmd struct {
	name string
	arg  []string
}

func newCmd(name string, arg ...string) *mockCmd {
	return &mockCmd{name: name, arg: arg}
}
func (c *mockCmd) SetDir(dir string)   {}
func (c *mockCmd) SetEnv(env []string) {}
func (c *mockCmd) StdinPipe() (io.WriteCloser, error) {
	return NewMockReadWriteCloser(), nil
}
func (c *mockCmd) StdoutPipe() (io.ReadCloser, error) {
	return NewMockReadWriteCloser(), nil
}
func (c *mockCmd) StderrPipe() (io.ReadCloser, error) {
	return NewMockReadWriteCloser(), nil
}
func (c *mockCmd) SetStdin(stdin io.Reader)   {}
func (c *mockCmd) SetStdout(stdout io.Writer) {}
func (c *mockCmd) SetStderr(stderr io.Writer) {}
func (c *mockCmd) ExitState() oslayer.ProcessExitState {
	return NewProcessExitState(123)
}
func (c *mockCmd) Process() oslayer.Process {
	return newProcess(101)
}
func (c *mockCmd) Start() error {
	return nil
}
func (c *mockCmd) Wait() error {
	return nil
}
func (c *mockCmd) Run() error {
	return nil
}
func (c *mockCmd) Output() ([]byte, error) {
	return []byte{0, 1, 2}, nil
}
func (c *mockCmd) CombinedOutput() ([]byte, error) {
	return []byte{0, 1, 2}, nil
}

type mockFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
	sys     interface{}
}

func newFileInfo(name string) *mockFileInfo {
	return &mockFileInfo{name: name}
}
func (i *mockFileInfo) Name() string {
	return i.name
}
func (i *mockFileInfo) Size() int64 {
	return i.size
}
func (i *mockFileInfo) Mode() os.FileMode {
	return i.mode
}
func (i *mockFileInfo) ModTime() time.Time {
	return i.modTime
}
func (i *mockFileInfo) IsDir() bool {
	return i.isDir
}
func (i *mockFileInfo) Sys() interface{} {
	return i.sys
}

type mockOS struct {
}

// NewOS returns a mockOS, which mocks out operating system functionality.
func NewOS() oslayer.OS {
	return &mockOS{}
}

// Filesystem
func (o *mockOS) OpenFile(name string, flag int, perm os.FileMode) (oslayer.File, error) {
	return newFile(name, flag, perm), nil
}
func (o *mockOS) Command(name string, arg ...string) oslayer.Cmd {
	return newCmd(name, arg...)
}
func (o *mockOS) MkdirAll(path string, perm os.FileMode) error {
	return nil
}
func (o *mockOS) RemoveAll(path string) error {
	return nil
}
func (o *mockOS) Create(name string) (oslayer.File, error) {
	return newFile(name, 0, 0), nil
}
func (o *mockOS) ReadDir(dirname string) ([]os.FileInfo, error) {
	infos := []os.FileInfo{
		newFileInfo(filepath.Join(dirname, "a")),
	}
	return infos, nil
}
func (o *mockOS) Mount(source string, target string, fstype string, flags uintptr, data string) (err error) {
	return nil
}
func (o *mockOS) Unmount(target string, flags int) (err error) {
	return nil
}
func (o *mockOS) PathExists(name string) (bool, error) {
	return true, nil
}
func (o *mockOS) PathIsMounted(name string) (bool, error) {
	return true, nil
}
func (o *mockOS) Link(oldname, newname string) error {
	return nil
}

// Processes
func (o *mockOS) Kill(pid int, sig syscall.Signal) error {
	return nil
}
