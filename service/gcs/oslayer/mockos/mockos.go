// Package mockos defines a mock interface into operating system functionality.
package mockos

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
)

type mockReadWriteCloser struct {
	*bytes.Buffer
}

func NewMockReadWriteCloser() *mockReadWriteCloser {
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

type mockProcessExitState struct {
	exitCode int
}

func NewProcessExitState(exitCode int) *mockProcessExitState {
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

type mockNetworkInterface struct {
	link    oslayer.Link
	addr    oslayer.Addr
	gateway oslayer.Route
}

func (i *mockNetworkInterface) Link() oslayer.Link {
	return i.link
}
func (i *mockNetworkInterface) Addr() oslayer.Addr {
	return i.addr
}
func (i *mockNetworkInterface) Gateway() oslayer.Route {
	return i.gateway
}

type mockLink struct {
	name  string
	index int
	addrs []oslayer.Addr
}

func newLink(name string, index int, addrs []oslayer.Addr) *mockLink {
	return &mockLink{
		name:  name,
		index: index,
		addrs: addrs,
	}
}
func (l *mockLink) Name() string {
	return l.name
}
func (l *mockLink) Index() int {
	return l.index
}
func (l *mockLink) SetUp() error {
	return nil
}
func (l *mockLink) SetDown() error {
	return nil
}
func (l *mockLink) SetNamespace(namespace oslayer.Namespace) error {
	return nil
}
func (l *mockLink) Addrs(family int) ([]oslayer.Addr, error) {
	return l.addrs, nil
}
func (l *mockLink) AddAddr(addr oslayer.Addr) error {
	l.addrs = append(l.addrs, addr)
	return nil
}
func (l *mockLink) GatewayRoutes(family int) ([]oslayer.Route, error) {
	return []oslayer.Route{
		newRoute(newAddr()),
	}, nil
}

type mockAddr struct{}

func newAddr() *mockAddr {
	return &mockAddr{}
}
func (a *mockAddr) IP() net.IP {
	return net.ParseIP("0.0.0.0")
}
func (a *mockAddr) String() string {
	return ""
}

type mockRoute struct {
	gw     oslayer.Addr
	metric int
}

func newRoute(gw oslayer.Addr) *mockRoute {
	return &mockRoute{gw: gw}
}
func (r *mockRoute) Gw() oslayer.Addr {
	return r.gw
}
func (r *mockRoute) Metric() int {
	return r.metric
}
func (r *mockRoute) SetMetric(metric int) {
	r.metric = metric
}
func (r *mockRoute) LinkIndex() int {
	return 0
}

type mockNamespace struct{}

func newNamespace() *mockNamespace {
	return &mockNamespace{}
}
func (n *mockNamespace) Close() error {
	return nil
}

type mockOS struct {
	CurrentNamespace oslayer.Namespace
}

func NewOS() *mockOS {
	return &mockOS{
		CurrentNamespace: newNamespace(),
	}
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

// Networking
func (o *mockOS) GetLinkByName(name string) (oslayer.Link, error) {
	return newLink(name, 0, []oslayer.Addr{newAddr()}), nil
}
func (o *mockOS) GetLinkByIndex(index int) (oslayer.Link, error) {
	return newLink("", index, []oslayer.Addr{newAddr()}), nil
}
func (o *mockOS) GetCurrentNamespace() (oslayer.Namespace, error) {
	return o.CurrentNamespace, nil
}
func (o *mockOS) SetCurrentNamespace(namespace oslayer.Namespace) error {
	o.CurrentNamespace = namespace
	return nil
}
func (o *mockOS) GetNamespaceFromPid(pid int) (oslayer.Namespace, error) {
	return newNamespace(), nil
}
func (o *mockOS) NewRoute(scope uint8, linkIndex int, gateway oslayer.Addr) oslayer.Route {
	return newRoute(gateway)
}
func (o *mockOS) AddRoute(route oslayer.Route) error {
	return nil
}
func (o *mockOS) AddGatewayRoute(gw oslayer.Addr, link oslayer.Link, metric int) error {
	return nil
}
func (o *mockOS) ParseAddr(s string) (oslayer.Addr, error) {
	return newAddr(), nil
}

// Processes
func (o *mockOS) Kill(pid int, sig syscall.Signal) error {
	return nil
}
