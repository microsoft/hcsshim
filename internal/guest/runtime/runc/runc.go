// +build linux

// Package runc defines an implementation of the Runtime interface which uses
// runC as the container runtime.
package runc

import (
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/guest/commonutils"
	"github.com/Microsoft/hcsshim/internal/guest/gcserr"
	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	containerFilesDir = "/var/run/gcsrunc"
	initPidFilename   = "initpid"
)

func setSubReaper(i int) error {
	return unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, uintptr(i), 0, 0, 0)
}

// runcRuntime is an implementation of the Runtime interface which uses runC as
// the container runtime.
type runcRuntime struct {
	runcLogBasePath string
}

var _ runtime.Runtime = &runcRuntime{}

type container struct {
	r    *runcRuntime
	id   string
	init *process
	// ownsPidNamespace indicates whether the container's init process is also
	// the init process for its pid namespace.
	ownsPidNamespace bool
}

func (c *container) ID() string {
	return c.id
}

func (c *container) Pid() int {
	return c.init.Pid()
}

func (c *container) Tty() *stdio.TtyRelay {
	return c.init.ttyRelay
}

func (c *container) PipeRelay() *stdio.PipeRelay {
	return c.init.pipeRelay
}

// process represents a process running in a container. It can either be a
// container's init process, or an exec process in a container.
type process struct {
	c         *container
	pid       int
	ttyRelay  *stdio.TtyRelay
	pipeRelay *stdio.PipeRelay
}

func (p *process) Pid() int {
	return p.pid
}

func (p *process) Tty() *stdio.TtyRelay {
	return p.ttyRelay
}

func (p *process) PipeRelay() *stdio.PipeRelay {
	return p.pipeRelay
}

// NewRuntime instantiates a new runcRuntime struct.
func NewRuntime(logBasePath string) (runtime.Runtime, error) {

	rtime := &runcRuntime{runcLogBasePath: logBasePath}
	if err := rtime.initialize(); err != nil {
		return nil, err
	}
	return rtime, nil
}

// initialize sets up any state necessary for the runcRuntime to function.
func (r *runcRuntime) initialize() error {
	paths := [2]string{containerFilesDir, r.runcLogBasePath}
	for _, p := range paths {
		_, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				if err := os.MkdirAll(p, 0700); err != nil {
					return errors.Wrapf(err, "failed making runC container files directory %s", p)
				}
			} else {
				return err
			}
		}
	}

	return nil
}

// CreateContainer creates a container with the given ID and the given
// bundlePath.
// bundlePath should be a path to an OCI bundle containing a config.json file
// and a rootfs for the container.
func (r *runcRuntime) CreateContainer(id string, bundlePath string, stdioSet *stdio.ConnectionSet) (c runtime.Container, err error) {
	c, err = r.runCreateCommand(id, bundlePath, stdioSet)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Start unblocks the container's init process created by the call to
// CreateContainer.
func (c *container) Start() error {
	logPath := c.r.getLogPath(c.id)
	args := []string{"start", c.id}
	cmd := createRuncCommand(logPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := getRuncLogError(logPath)
		c.r.cleanupContainer(c.id)
		return errors.Wrapf(err, "runc start failed with %v: %s", runcErr, string(out))
	}
	return nil
}

// ExecProcess executes a new process, represented as an OCI process struct,
// inside an already-running container.
func (c *container) ExecProcess(process *oci.Process, stdioSet *stdio.ConnectionSet) (p runtime.Process, err error) {
	p, err = c.runExecCommand(process, stdioSet)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Kill sends the specified signal to the container's init process.
func (c *container) Kill(signal syscall.Signal) error {
	logPath := c.r.getLogPath(c.id)
	args := []string{"kill"}
	if signal == syscall.SIGTERM || signal == syscall.SIGKILL {
		args = append(args, "--all")
	}
	args = append(args, c.id, strconv.Itoa(int(signal)))
	cmd := createRuncCommand(logPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(err.Error(), "os: process already finished") ||
			strings.Contains(err.Error(), "container not running") ||
			err == syscall.ESRCH {
			return gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
		}

		runcErr := getRuncLogError(logPath)
		return errors.Wrapf(err, "unknown runc error after kill %v: %s", runcErr, string(out))
	}
	return nil
}

// Delete deletes any state created for the container by either this wrapper or
// runC itself.
func (c *container) Delete() error {
	logPath := c.r.getLogPath(c.id)
	args := []string{"delete", c.id}
	cmd := createRuncCommand(logPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := getRuncLogError(logPath)
		return errors.Wrapf(err, "runc delete failed with %v: %s", runcErr, string(out))
	}
	if err := c.r.cleanupContainer(c.id); err != nil {
		return err
	}
	return nil
}

// Delete deletes any state created for the process by either this wrapper or
// runC itself.
func (p *process) Delete() error {
	if err := p.c.r.cleanupProcess(p.c.id, p.pid); err != nil {
		return err
	}
	return nil
}

// Pause suspends all processes running in the container.
func (c *container) Pause() error {
	logPath := c.r.getLogPath(c.id)
	args := []string{"pause", c.id}
	cmd := createRuncCommand(logPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := getRuncLogError(logPath)
		return errors.Wrapf(err, "runc pause failed with %v: %s", runcErr, string(out))
	}
	return nil
}

// Resume unsuspends processes running in the container.
func (c *container) Resume() error {
	logPath := c.r.getLogPath(c.id)
	args := []string{"resume", c.id}
	cmd := createRuncCommand(logPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := getRuncLogError(logPath)
		return errors.Wrapf(err, "runc resume failed with %v: %s", runcErr, string(out))
	}
	return nil
}

// GetState returns information about the given container.
func (c *container) GetState() (*runtime.ContainerState, error) {
	logPath := c.r.getLogPath(c.id)
	args := []string{"state", c.id}
	cmd := createRuncCommand(logPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := getRuncLogError(logPath)
		return nil, errors.Wrapf(err, "runc state failed with %v: %s", runcErr, string(out))
	}
	var state runtime.ContainerState
	if err := json.Unmarshal(out, &state); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal the state for container %s", c.id)
	}
	return &state, nil
}

// Exists returns true if the container exists, false if it doesn't
// exist.
// It should be noted that containers that have stopped but have not been
// deleted are still considered to exist.
func (c *container) Exists() (bool, error) {
	states, err := c.r.ListContainerStates()
	if err != nil {
		return false, err
	}
	// TODO: This is definitely not the most efficient way of doing this. See
	// about improving it in the future.
	for _, state := range states {
		if state.ID == c.id {
			return true, nil
		}
	}
	return false, nil
}

// ListContainerStates returns ContainerState structs for all existing
// containers, whether they're running or not.
func (r *runcRuntime) ListContainerStates() ([]runtime.ContainerState, error) {
	logPath := filepath.Join(r.runcLogBasePath, "global-runc.log")
	args := []string{"list", "-f", "json"}
	cmd := createRuncCommand(logPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := getRuncLogError(logPath)
		return nil, errors.Wrapf(err, "runc list failed with %v: %s", runcErr, string(out))
	}
	var states []runtime.ContainerState
	if err := json.Unmarshal(out, &states); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal the states for the container list")
	}
	return states, nil
}

// GetRunningProcesses gets only the running processes associated with the given
// container. This excludes zombie processes.
func (c *container) GetRunningProcesses() ([]runtime.ContainerProcessState, error) {
	pids, err := c.r.getRunningPids(c.id)
	if err != nil {
		return nil, err
	}

	pidMap := map[int]*runtime.ContainerProcessState{}
	// Initialize all processes with a pid and command, and mark correctly that
	// none of them are zombies. Default CreatedByRuntime to false.
	for _, pid := range pids {
		command, err := c.r.getProcessCommand(pid)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				// process has exited between getting the running pids above
				// and now, ignore error
				continue
			}
			return nil, err
		}
		pidMap[pid] = &runtime.ContainerProcessState{Pid: pid, Command: command, CreatedByRuntime: false, IsZombie: false}
	}

	// For each process state directory which corresponds to a running pid, set
	// that the process was created by the Runtime.
	processDirs, err := ioutil.ReadDir(filepath.Join(containerFilesDir, c.id))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read the contents of container directory %s", filepath.Join(containerFilesDir, c.id))
	}
	for _, processDir := range processDirs {
		if processDir.Name() != initPidFilename {
			pid, err := strconv.Atoi(processDir.Name())
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse string \"%s\" as pid", processDir.Name())
			}
			if _, ok := pidMap[pid]; ok {
				pidMap[pid].CreatedByRuntime = true
			}
		}
	}

	return c.r.pidMapToProcessStates(pidMap), nil
}

// GetAllProcesses gets all processes associated with the given container,
// including both running and zombie processes.
func (c *container) GetAllProcesses() ([]runtime.ContainerProcessState, error) {
	runningPids, err := c.r.getRunningPids(c.id)
	if err != nil {
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"cid":  c.id,
		"pids": runningPids,
	}).Debug("running container pids")

	pidMap := map[int]*runtime.ContainerProcessState{}
	// Initialize all processes with a pid and command, leaving CreatedByRuntime
	// and IsZombie at the default value of false.
	for _, pid := range runningPids {
		command, err := c.r.getProcessCommand(pid)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				// process has exited between getting the running pids above
				// and now, ignore error
				continue
			}
			return nil, err
		}
		pidMap[pid] = &runtime.ContainerProcessState{Pid: pid, Command: command, CreatedByRuntime: false, IsZombie: false}
	}

	processDirs, err := ioutil.ReadDir(filepath.Join(containerFilesDir, c.id))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read the contents of container directory %s", filepath.Join(containerFilesDir, c.id))
	}
	// Loop over every process state directory. Since these processes have
	// process state directories, CreatedByRuntime will be true for all of them.
	for _, processDir := range processDirs {
		if processDir.Name() != initPidFilename {
			pid, err := strconv.Atoi(processDir.Name())
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse string \"%s\" into pid", processDir.Name())
			}
			if c.r.processExists(pid) {
				// If the process exists in /proc and is in the pidMap, it must
				// be a running non-zombie.
				if _, ok := pidMap[pid]; ok {
					pidMap[pid].CreatedByRuntime = true
				} else {
					// Otherwise, since it's in /proc but not running, it must
					// be a zombie.
					command, err := c.r.getProcessCommand(pid)
					if err != nil {
						if errors.Is(err, unix.ENOENT) {
							// process has exited between checking that it exists and now, ignore error
							continue
						}
						return nil, err
					}
					pidMap[pid] = &runtime.ContainerProcessState{Pid: pid, Command: command, CreatedByRuntime: true, IsZombie: true}
				}
			}
		}
	}
	return c.r.pidMapToProcessStates(pidMap), nil
}

// getRunningPids gets the pids of all processes which runC recognizes as
// running.
func (r *runcRuntime) getRunningPids(id string) ([]int, error) {
	logPath := r.getLogPath(id)
	args := []string{"ps", "-f", "json", id}
	cmd := createRuncCommand(logPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := getRuncLogError(logPath)
		return nil, errors.Wrapf(err, "runc ps failed with %v: %s", runcErr, string(out))
	}
	var pids []int
	if err := json.Unmarshal(out, &pids); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal pids for container %s", id)
	}
	return pids, nil
}

// getProcessCommand gets the command line command and arguments for the process
// with the given pid.
func (r *runcRuntime) getProcessCommand(pid int) ([]string, error) {
	// Get the contents of the process's cmdline file. This file is formatted
	// with a null character after every argument. e.g. "ping google.com "
	data, err := ioutil.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read cmdline file for process %d", pid)
	}
	// Get rid of the \0 character at end.
	cmdString := strings.TrimSuffix(string(data), "\x00")
	return strings.Split(cmdString, "\x00"), nil
}

// pidMapToProcessStates is a helper function which converts a map from pid to
// ContainerProcessState to a slice of ContainerProcessStates.
func (r *runcRuntime) pidMapToProcessStates(pidMap map[int]*runtime.ContainerProcessState) []runtime.ContainerProcessState {
	processStates := make([]runtime.ContainerProcessState, len(pidMap))
	i := 0
	for _, processState := range pidMap {
		processStates[i] = *processState
		i++
	}
	return processStates
}

// waitOnProcess waits for the process to exit, and returns its exit code.
func (r *runcRuntime) waitOnProcess(pid int) (int, error) {
	process, err := os.FindProcess(pid)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to find process %d", pid)
	}
	state, err := process.Wait()
	if err != nil {
		return -1, errors.Wrapf(err, "failed waiting on process %d", pid)
	}

	status := state.Sys().(syscall.WaitStatus)
	if status.Signaled() {
		return 128 + int(status.Signal()), nil
	}
	return status.ExitStatus(), nil
}

func (p *process) Wait() (int, error) {
	exitCode, err := p.c.r.waitOnProcess(p.pid)

	l := logrus.WithField("cid", p.c.id)
	l.WithField("pid", p.pid).Debug("process wait completed")

	// If the init process for the container has exited, kill everything else in
	// the container. Runc uses the devices cgroup of the container ot determine
	// what other processes to kill.
	//
	// We don't issue the kill if the container owns its own pid namespace,
	// because in that case the container kernel will kill everything in the pid
	// namespace automatically (as the container init will be the pid namespace
	// init). This prevents a potential issue where two containers share cgroups
	// but have their own pid namespaces. If we didn't handle this case, runc
	// would kill the processes in both containers when trying to kill
	// either one of them.
	if p == p.c.init && !p.c.ownsPidNamespace {
		// If the init process of a pid namespace terminates, the kernel
		// terminates all other processes in the namespace with SIGKILL. We
		// simulate the same behavior.
		if err := p.c.Kill(syscall.SIGKILL); err != nil {
			l.WithError(err).Error("failed to terminate container after process wait")
		}
	}

	// Wait on the relay to drain any output that was already buffered.
	//
	// At this point, if this is the init process for the container, everything
	// else in the container has been killed, so the write ends of the stdio
	// relay will have been closed.
	//
	// If this is a container exec process instead, then it is possible the
	// relay waits will hang waiting for the write ends to close. This can occur
	// if the exec spawned any child processes that inherited its stdio.
	// Currently we do not do anything to avoid hanging in this case, but in the
	// future we could add special handling.
	if p.ttyRelay != nil {
		p.ttyRelay.Wait()
	}
	if p.pipeRelay != nil {
		p.pipeRelay.Wait()
	}
	return exitCode, err
}

// Wait waits on every non-init process in the container, and then performs a
// final wait on the init process. The exit code returned is the exit code
// acquired from waiting on the init process.
func (c *container) Wait() (int, error) {
	processes, err := c.GetAllProcesses()
	if err != nil {
		return -1, err
	}
	for _, process := range processes {
		// Only wait on non-init processes that were created with exec.
		if process.Pid != c.init.pid && process.CreatedByRuntime {
			// FUTURE-jstarks: Consider waiting on the child process's relays as
			// well (as in p.Wait()). This may not matter as long as the relays
			// finish "soon" after Wait() returns since HCS expects the stdio
			// connections to close before container shutdown can complete.
			logrus.WithFields(logrus.Fields{
				"cid": c.id,
				"pid": process.Pid,
			}).Debug("waiting on container exec process")
			c.r.waitOnProcess(process.Pid)
		}
	}
	exitCode, err := c.init.Wait()
	if err != nil {
		return -1, err
	}
	return exitCode, nil
}

// runCreateCommand sets up the arguments for calling runc create.
func (r *runcRuntime) runCreateCommand(id string, bundlePath string, stdioSet *stdio.ConnectionSet) (runtime.Container, error) {
	c := &container{r: r, id: id}
	if err := r.makeContainerDir(id); err != nil {
		return nil, err
	}
	// Create a temporary random directory to store the process's files.
	tempProcessDir, err := ioutil.TempDir(containerFilesDir, id)
	if err != nil {
		return nil, err
	}

	spec, err := ociSpecFromBundle(bundlePath)
	if err != nil {
		return nil, err
	}

	// Determine if the container owns its own pid namespace or not. Per the OCI
	// spec:
	// - If the spec has no entry for the pid namespace, the container inherits
	//   the runtime namespace (container does not own).
	// - If the spec has a pid namespace entry, but the path is empty, a new
	//   namespace will be created and used for the container (container owns).
	// - If there is a pid namespace entry with a path, the container uses the
	//   namespace at that path (container does not own).
	if spec.Linux != nil {
		for _, ns := range spec.Linux.Namespaces {
			if ns.Type == oci.PIDNamespace {
				c.ownsPidNamespace = ns.Path == ""
			}
		}
	}

	if spec.Process.Cwd != "/" {
		cwd := path.Join(bundlePath, "rootfs", spec.Process.Cwd)
		// Intentionally ignore the error.
		_ = os.MkdirAll(cwd, 0755)
	}

	args := []string{"create", "-b", bundlePath, "--no-pivot"}
	p, err := c.startProcess(tempProcessDir, spec.Process.Terminal, stdioSet, args...)
	if err != nil {
		return nil, err
	}

	// Write pid to initpid file for container.
	containerDir := r.getContainerDir(id)
	if err := ioutil.WriteFile(filepath.Join(containerDir, initPidFilename), []byte(strconv.Itoa(p.pid)), 0777); err != nil {
		return nil, err
	}

	c.init = p
	return c, nil
}

func ociSpecFromBundle(bundlePath string) (*oci.Spec, error) {
	configPath := filepath.Join(bundlePath, "config.json")
	configFile, err := os.Open(configPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open bundle config at %s", configPath)
	}
	defer configFile.Close()
	var spec *oci.Spec
	if err := commonutils.DecodeJSONWithHresult(configFile, &spec); err != nil {
		return nil, errors.Wrap(err, "failed to parse OCI spec")
	}
	return spec, nil
}

// runExecCommand sets up the arguments for calling runc exec.
func (c *container) runExecCommand(processDef *oci.Process, stdioSet *stdio.ConnectionSet) (p runtime.Process, err error) {
	// Create a temporary random directory to store the process's files.
	tempProcessDir, err := ioutil.TempDir(containerFilesDir, c.id)
	if err != nil {
		return nil, err
	}

	f, err := os.Create(filepath.Join(tempProcessDir, "process.json"))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create process.json file at %s", filepath.Join(tempProcessDir, "process.json"))
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(processDef); err != nil {
		return nil, errors.Wrap(err, "failed to encode JSON into process.json file")
	}

	args := []string{"exec"}
	args = append(args, "-d", "--process", filepath.Join(tempProcessDir, "process.json"))
	return c.startProcess(tempProcessDir, processDef.Terminal, stdioSet, args...)
}

// startProcess performs the operations necessary to start a container process
// and properly handle its stdio. This function is used by both CreateContainer
// and ExecProcess. For V2 container creation stdioSet will be nil, in this case
// it is expected that the caller starts the relay previous to calling Start on
// the container.
func (c *container) startProcess(tempProcessDir string, hasTerminal bool, stdioSet *stdio.ConnectionSet, initialArgs ...string) (p *process, err error) {
	args := initialArgs

	if err := setSubreaper(1); err != nil {
		return nil, errors.Wrapf(err, "failed to set process as subreaper for process in container %s", c.id)
	}
	if err := c.r.makeLogDir(c.id); err != nil {
		return nil, err
	}

	logPath := c.r.getLogPath(c.id)
	args = append(args, "--pid-file", filepath.Join(tempProcessDir, "pid"))

	var sockListener *net.UnixListener
	if hasTerminal {
		var consoleSockPath string
		sockListener, consoleSockPath, err = c.r.createConsoleSocket(tempProcessDir)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create console socket for container %s", c.id)
		}
		defer sockListener.Close()
		args = append(args, "--console-socket", consoleSockPath)
	}
	args = append(args, c.id)

	cmd := createRuncCommand(logPath, args...)

	var pipeRelay *stdio.PipeRelay
	if !hasTerminal {
		pipeRelay, err = stdio.NewPipeRelay(stdioSet)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create a pipe relay connection set for container %s", c.id)
		}
		fileSet, err := pipeRelay.Files()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get files for connection set for container %s", c.id)
		}
		// Closing the FileSet here is fine as that end of the pipes will have
		// already been copied into the child process.
		defer fileSet.Close()
		if fileSet.In != nil {
			cmd.Stdin = fileSet.In
		}
		if fileSet.Out != nil {
			cmd.Stdout = fileSet.Out
		}
		if fileSet.Err != nil {
			cmd.Stderr = fileSet.Err
		}
	}

	if err := cmd.Run(); err != nil {
		runcErr := getRuncLogError(logPath)
		return nil, errors.Wrapf(err, "failed to run runc create/exec call for container %s with %v", c.id, runcErr)
	}

	var ttyRelay *stdio.TtyRelay
	if hasTerminal {
		var master *os.File
		master, err = c.r.getMasterFromSocket(sockListener)
		if err != nil {
			cmd.Process.Kill()
			return nil, errors.Wrapf(err, "failed to get pty master for process in container %s", c.id)
		}
		// Keep master open for the relay unless there is an error.
		defer func() {
			if err != nil {
				master.Close()
			}
		}()
		ttyRelay = stdio.NewTtyRelay(stdioSet, master)
	}

	// Rename the process's directory to its pid.
	pid, err := c.r.readPidFile(filepath.Join(tempProcessDir, "pid"))
	if err != nil {
		return nil, err
	}
	if err := os.Rename(tempProcessDir, c.r.getProcessDir(c.id, pid)); err != nil {
		return nil, err
	}

	if ttyRelay != nil && stdioSet != nil {
		ttyRelay.Start()
	}
	if pipeRelay != nil && stdioSet != nil {
		pipeRelay.Start()
	}
	return &process{c: c, pid: pid, ttyRelay: ttyRelay, pipeRelay: pipeRelay}, nil
}

func (c *container) Update(resources interface{}) error {
	jsonResources, err := json.Marshal(resources)
	if err != nil {
		return err
	}
	logPath := c.r.getLogPath(c.id)
	args := []string{"update", "--resources", "-", c.id}
	cmd := createRuncCommand(logPath, args...)
	cmd.Stdin = strings.NewReader(string(jsonResources))
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := getRuncLogError(logPath)
		return errors.Wrapf(err, "runc update request %s failed with %v: %s", string(jsonResources), runcErr, string(out))
	}
	return nil
}
