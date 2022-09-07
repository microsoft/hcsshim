//go:build linux
// +build linux

package runc

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/logfields"
)

type container struct {
	r    *runcRuntime
	id   string
	init *process
	// ownsPidNamespace indicates whether the container's init process is also
	// the init process for its pid namespace.
	ownsPidNamespace bool
}

var _ runtime.Container = &container{}

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

// Start unblocks the container's init process created by the call to
// CreateContainer.
func (c *container) Start() error {
	logPath := c.r.getLogPath(c.id)
	args := []string{"start", c.id}
	cmd := runcCommandLog(logPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := getRuncLogError(logPath)
		c.r.cleanupContainer(c.id) //nolint:errcheck
		return errors.Wrapf(runcErr, "runc start failed with %v: %s", err, string(out))
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
	logrus.WithField(logfields.ContainerID, c.id).Debug("runc::container::Kill")
	args := []string{"kill"}
	if signal == syscall.SIGTERM || signal == syscall.SIGKILL {
		args = append(args, "--all")
	}
	args = append(args, c.id, strconv.Itoa(int(signal)))
	cmd := runcCommand(args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := parseRuncError(string(out))
		return errors.Wrapf(runcErr, "unknown runc error after kill %v: %s", err, string(out))
	}
	return nil
}

// Delete deletes any state created for the container by either this wrapper or
// runC itself.
func (c *container) Delete() error {
	logrus.WithField(logfields.ContainerID, c.id).Debug("runc::container::Delete")
	cmd := runcCommand("delete", c.id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := parseRuncError(string(out))
		return errors.Wrapf(runcErr, "runc delete failed with %v: %s", err, string(out))
	}
	return c.r.cleanupContainer(c.id)
}

// Pause suspends all processes running in the container.
func (c *container) Pause() error {
	cmd := runcCommand("pause", c.id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := parseRuncError(string(out))
		return errors.Wrapf(runcErr, "runc pause failed with %v: %s", err, string(out))
	}
	return nil
}

// Resume unsuspends processes running in the container.
func (c *container) Resume() error {
	logPath := c.r.getLogPath(c.id)
	args := []string{"resume", c.id}
	cmd := runcCommandLog(logPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := getRuncLogError(logPath)
		return errors.Wrapf(runcErr, "runc resume failed with %v: %s", err, string(out))
	}
	return nil
}

// GetState returns information about the given container.
func (c *container) GetState() (*runtime.ContainerState, error) {
	cmd := runcCommand("state", c.id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := parseRuncError(string(out))
		return nil, errors.Wrapf(runcErr, "runc state failed with %v: %s", err, string(out))
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
	// use global path because container may not exist
	cmd := runcCommand("state", c.id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := parseRuncError(string(out))
		if errors.Is(runcErr, runtime.ErrContainerDoesNotExist) {
			return false, nil
		}
		return false, errors.Wrapf(runcErr, "runc state failed with %v: %s", err, string(out))
	}
	return true, nil
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
	processDirs, err := os.ReadDir(filepath.Join(containerFilesDir, c.id))
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

	processDirs, err := os.ReadDir(filepath.Join(containerFilesDir, c.id))
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

// GetInitProcess gets the init processes associated with the given container,
// including both running and zombie processes.
func (c *container) GetInitProcess() (runtime.Process, error) {
	if c.init == nil {
		return nil, errors.New("container has no init process")
	}
	return c.init, nil
}

// Wait waits on every non-init process in the container, and then performs a
// final wait on the init process. The exit code returned is the exit code
// acquired from waiting on the init process.
func (c *container) Wait() (int, error) {
	entity := logrus.WithField(logfields.ContainerID, c.id)
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
			entity.WithField(logfields.ProcessID, process.Pid).Debug("waiting on container exec process")
			_, _ = c.r.waitOnProcess(process.Pid)
		}
	}
	exitCode, err := c.init.Wait()
	entity.Debug("runc::container::init process wait completed")
	if err != nil {
		return -1, err
	}
	return exitCode, nil
}

// runExecCommand sets up the arguments for calling runc exec.
func (c *container) runExecCommand(processDef *oci.Process, stdioSet *stdio.ConnectionSet) (p runtime.Process, err error) {
	// Create a temporary random directory to store the process's files.
	tempProcessDir, err := os.MkdirTemp(containerFilesDir, c.id)
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
func (c *container) startProcess(
	tempProcessDir string,
	hasTerminal bool,
	stdioSet *stdio.ConnectionSet, initialArgs ...string,
) (p *process, err error) {
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

	cmd := runcCommandLog(logPath, args...)

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
		return nil, errors.Wrapf(runcErr, "failed to run runc create/exec call for container %s with %v", c.id, err)
	}

	var ttyRelay *stdio.TtyRelay
	if hasTerminal {
		var master *os.File
		master, err = c.r.getMasterFromSocket(sockListener)
		if err != nil {
			_ = cmd.Process.Kill()
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
	cmd := runcCommand("update", "--resources", "-", c.id)
	cmd.Stdin = strings.NewReader(string(jsonResources))
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := parseRuncError(string(out))
		return errors.Wrapf(runcErr, "runc update request %s failed with %v: %s", string(jsonResources), err, string(out))
	}
	return nil
}
