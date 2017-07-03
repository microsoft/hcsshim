// Package realos defines the actual interface into operating system
// functionality.
package realos

import (
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
)

// realProcessExitState represents an oslayer.ProcessExitState which uses an
// os.ProcessState for its information.
type realProcessExitState struct {
	state *os.ProcessState
}

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

type realLink struct {
	link netlink.Link
}

func (l *realLink) Name() string {
	return l.link.Attrs().Name
}
func (l *realLink) Index() int {
	return l.link.Attrs().Index
}
func (l *realLink) SetUp() error {
	if err := netlink.LinkSetUp(l.link); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (l *realLink) SetDown() error {
	if err := netlink.LinkSetDown(l.link); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (l *realLink) SetNamespace(namespace oslayer.Namespace) error {
	fd := int(*namespace.(*realNamespace).nsHandle)
	if err := netlink.LinkSetNsFd(l.link, fd); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (l *realLink) Addrs(family int) ([]oslayer.Addr, error) {
	addrs, err := netlink.AddrList(l.link, family)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	addrsToReturn := make([]oslayer.Addr, len(addrs))
	for _, addr := range addrs {
		addrsToReturn = append(addrsToReturn, newAddr(&addr))
	}
	return addrsToReturn, nil
}
func (l *realLink) AddAddr(addr oslayer.Addr) error {
	if err := netlink.AddrAdd(l.link, addr.(*realAddr).addr); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (l *realLink) GatewayRoutes(family int) ([]oslayer.Route, error) {
	filter := &netlink.Route{LinkIndex: l.Index(), Dst: nil}
	routes, err := netlink.RouteListFiltered(family, filter, netlink.RT_FILTER_OIF|netlink.RT_FILTER_DST)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	routesToReturn := make([]oslayer.Route, len(routes))
	for _, route := range routes {
		routesToReturn = append(routesToReturn, newRoute(&route))
	}
	return routesToReturn, nil
}

type realAddr struct {
	addr *netlink.Addr
}

func newAddr(addr *netlink.Addr) *realAddr {
	return &realAddr{addr: addr}
}
func (a *realAddr) IP() net.IP {
	return a.addr.IP
}
func (a *realAddr) String() string {
	return a.addr.String()
}

type realRoute struct {
	route *netlink.Route
}

func newRoute(route *netlink.Route) *realRoute {
	return &realRoute{route: route}
}
func (r *realRoute) Gw() oslayer.Addr {
	ip := r.route.Gw
	mask := net.IPv4Mask(255, 255, 255, 255)
	return newAddr(&netlink.Addr{IPNet: &net.IPNet{IP: ip, Mask: mask}})
}
func (r *realRoute) Metric() int {
	return r.route.Priority
}
func (r *realRoute) SetMetric(metric int) {
	r.route.Priority = metric
}
func (r *realRoute) LinkIndex() int {
	return r.route.LinkIndex
}

type realNamespace struct {
	nsHandle *netns.NsHandle
}

func newNamespace(nsHandle *netns.NsHandle) *realNamespace {
	return &realNamespace{nsHandle: nsHandle}
}
func (n *realNamespace) Close() error {
	if err := n.nsHandle.Close(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type realOS struct{}

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
		} else {
			return false, errors.WithStack(err)
		}
	}
	return true, nil
}
func (o *realOS) PathIsMounted(name string) (bool, error) {
	// mountpoint has exit status 0 if the mountpoint exists, 1 if not or if
	// encountering some other error.
	err := exec.Command("mountpoint", "-q", name).Run()
	if err != nil {
		_, ok := err.(*exec.ExitError)
		if ok {
			return false, nil
		} else {
			return false, errors.WithStack(err)
		}
	} else {
		return true, err
	}
}
func (o *realOS) Link(oldname, newname string) error {
	if err := os.Link(oldname, newname); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Networking
func (o *realOS) GetLinkByName(name string) (oslayer.Link, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &realLink{link: link}, nil
}
func (o *realOS) GetLinkByIndex(index int) (oslayer.Link, error) {
	link, err := netlink.LinkByIndex(index)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &realLink{link: link}, nil
}
func (o *realOS) GetCurrentNamespace() (oslayer.Namespace, error) {
	nsHandle, err := netns.Get()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return newNamespace(&nsHandle), nil
}
func (o *realOS) SetCurrentNamespace(namespace oslayer.Namespace) error {
	if err := netns.Set(*namespace.(*realNamespace).nsHandle); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (o *realOS) GetNamespaceFromPid(pid int) (oslayer.Namespace, error) {
	nsHandle, err := netns.GetFromPid(pid)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return newNamespace(&nsHandle), nil
}
func (o *realOS) NewRoute(scope uint8, linkIndex int, gateway oslayer.Addr) oslayer.Route {
	netlinkRoute := &netlink.Route{Scope: netlink.Scope(scope), LinkIndex: linkIndex, Gw: gateway.IP()}
	return newRoute(netlinkRoute)
}
func (o *realOS) AddRoute(route oslayer.Route) error {
	netlinkRoute := route.(*realRoute).route
	if err := netlink.RouteAdd(netlinkRoute); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (os *realOS) AddGatewayRoute(gw oslayer.Addr, link oslayer.Link, metric int) error {
	out, err := exec.Command("route", "add", "default", "gw", gw.IP().String(), "dev", link.Name(), "metric", strconv.Itoa(metric)).CombinedOutput()
	if err != nil {
		return errors.Errorf("%s", out)
	}
	return nil
}
func (o *realOS) ParseAddr(s string) (oslayer.Addr, error) {
	addr, err := netlink.ParseAddr(s)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return newAddr(addr), nil
}

// Processes
func (o *realOS) Kill(pid int, sig syscall.Signal) error {
	if err := syscall.Kill(pid, sig); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
