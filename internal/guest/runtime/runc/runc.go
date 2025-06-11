//go:build linux
// +build linux

package runc

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/commonutils"
	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
)

const (
	containerFilesDir = "/var/run/gcsrunc"
	initPidFilename   = "initpid"
)

func setSubreaper(i int) error {
	return unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, uintptr(i), 0, 0, 0)
}

// NewRuntime instantiates a new runcRuntime struct.
func NewRuntime(logBasePath string) (runtime.Runtime, error) {
	rtime := &runcRuntime{runcLogBasePath: logBasePath}
	if err := rtime.initialize(); err != nil {
		return nil, err
	}
	return rtime, nil
}

// runcRuntime is an implementation of the Runtime interface which uses runC as
// the container runtime.
type runcRuntime struct {
	runcLogBasePath string
}

var _ runtime.Runtime = &runcRuntime{}

// initialize sets up any state necessary for the runcRuntime to function.
func (r *runcRuntime) initialize() error {
	paths := [2]string{containerFilesDir, r.runcLogBasePath}
	for _, p := range paths {
		_, err := os.Stat(p)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			if err := os.MkdirAll(p, 0700); err != nil {
				return errors.Wrapf(err, "failed making runC container files directory %s", p)
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

// ListContainerStates returns ContainerState structs for all existing
// containers, whether they're running or not.
func (*runcRuntime) ListContainerStates() ([]runtime.ContainerState, error) {
	cmd := runcCommand("list", "-f", "json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := parseRuncError(string(out))
		return nil, errors.Wrapf(runcErr, "runc list failed with %v: %s", err, string(out))
	}
	var states []runtime.ContainerState
	if err := json.Unmarshal(out, &states); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal the states for the container list")
	}
	return states, nil
}

// getRunningPids gets the pids of all processes which runC recognizes as
// running.
func (*runcRuntime) getRunningPids(id string) ([]int, error) {
	cmd := runcCommand("ps", "-f", "json", id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		runcErr := parseRuncError(string(out))
		return nil, errors.Wrapf(runcErr, "runc ps failed with %v: %s", err, string(out))
	}
	var pids []int
	if err := json.Unmarshal(out, &pids); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal pids for container %s", id)
	}
	return pids, nil
}

// getProcessCommand gets the command line command and arguments for the process
// with the given pid.
func (*runcRuntime) getProcessCommand(pid int) ([]string, error) {
	// Get the contents of the process's cmdline file. This file is formatted
	// with a null character after every argument. e.g. "ping google.com "
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read cmdline file for process %d", pid)
	}
	// Get rid of the \0 character at end.
	cmdString := strings.TrimSuffix(string(data), "\x00")
	return strings.Split(cmdString, "\x00"), nil
}

// pidMapToProcessStates is a helper function which converts a map from pid to
// ContainerProcessState to a slice of ContainerProcessStates.
func (*runcRuntime) pidMapToProcessStates(pidMap map[int]*runtime.ContainerProcessState) []runtime.ContainerProcessState {
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

// runCreateCommand sets up the arguments for calling runc create.
func (r *runcRuntime) runCreateCommand(id string, bundlePath string, stdioSet *stdio.ConnectionSet) (runtime.Container, error) {
	c := &container{r: r, id: id}
	if err := r.makeContainerDir(id); err != nil {
		return nil, err
	}
	// Create a temporary random directory to store the process's files.
	tempProcessDir, err := os.MkdirTemp(containerFilesDir, id)
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
	if err := os.WriteFile(filepath.Join(containerDir, initPidFilename), []byte(strconv.Itoa(p.pid)), 0777); err != nil {
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
