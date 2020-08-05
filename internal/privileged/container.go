package privileged

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/hcsoci"

	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/windows"
)

// HostJobContainer represents the Windows equivalent of a privileged container.
// This is a process (or set of processes) run on the host in a job object. The usual
// namespace and network isolation don't apply here. Resource constraints and user
// information are still retrieved from the oci spec and set on the job object
// (if there are any).
type HostJobContainer struct {
	id             string
	spec           *specs.Spec
	job            *jobObject // Object representing the job object the container is in
	sandboxMount   string     // path to where the sandbox is mounted on the host
	closedWaitOnce sync.Once
	initDoOnce     sync.Once
	handleLock     sync.Mutex // lock to be held for any operations involving handles
	waitBlock      chan struct{}
	exitBlock      chan struct{}
	initBlock      chan struct{}
	waitError      error
	exitError      error
}

var _ cow.ProcessHost = &HostJobContainer{}
var _ cow.Container = &HostJobContainer{}

func newHostJobContainer(id string, s *specs.Spec) *HostJobContainer {
	return &HostJobContainer{
		id:        id,
		spec:      s,
		waitBlock: make(chan struct{}),
		exitBlock: make(chan struct{}),
		initBlock: make(chan struct{}),
	}
}

// CreateContainer creates a new HostJobContainer
func CreateContainer(ctx context.Context, id string, s *specs.Spec) (_ cow.Container, err error) {
	log.G(ctx).WithField("id", id).Debug("Creating privileged container")

	if s == nil {
		return nil, errors.New("Spec must be supplied")
	}

	if id == "" {
		g, err := guid.NewV4()
		if err != nil {
			return nil, err
		}
		id = g.String()
	}

	if err := mountLayers(ctx, s); err != nil {
		return nil, fmt.Errorf("failed to mount container layers: %s", err)
	}

	const volumeGUIDRegex = `^\\\\\?\\(Volume)\{{0,1}[0-9a-fA-F]{8}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{12}(\}){0,1}\}(|\\)$`
	if matched, err := regexp.MatchString(volumeGUIDRegex, s.Root.Path); !matched || err != nil {
		return nil, fmt.Errorf(`invalid container spec - Root.Path '%s' must be a volume GUID path in the format '\\?\Volume{GUID}\'`, s.Root.Path)
	}
	if s.Root.Path[len(s.Root.Path)-1] != '\\' {
		s.Root.Path += `\` // Be nice to clients and make sure well-formed for back-compat
	}

	container := newHostJobContainer(id, s)

	// Create the job object all processes will run in. Has no contraints on it
	// at this point.
	job, err := createJobObject(id)
	if err != nil {
		return nil, fmt.Errorf("failed to create job object: %s", err)
	}
	container.job = job

	var path string
	defer func() {
		if err != nil {
			container.Close()
			if path != "" {
				removeSandboxMountPoint(ctx, path)
			}
		}
	}()

	// Set resource limits on the job object based off of oci spec.
	if err := job.setResourceLimits(ctx, specToLimits(ctx, s)); err != nil {
		return nil, fmt.Errorf("failed to set resource limits: %s", err)
	}

	// Setup directory sandbox volume will be mounted
	sandboxPath := fmt.Sprintf(sandboxMountFormat, id)
	if _, err := os.Stat(sandboxPath); err != nil {
		if err := os.MkdirAll(sandboxPath, 0777); err != nil {
			return nil, fmt.Errorf("failed to create mounted folder: %s", err)
		}
	}
	path = sandboxPath

	if err := mountSandboxVolume(ctx, path, s.Root.Path); err != nil {
		return nil, fmt.Errorf("failed to bind payload directory on host: %s", err)
	}

	container.sandboxMount = path
	go container.waitBackground()
	return container, nil
}

// CreateProcess creates a process on the host, starts it, adds it to the containers
// job object and then waits for exit.
func (c *HostJobContainer) CreateProcess(ctx context.Context, config interface{}) (_ cow.Process, err error) {
	conf, ok := config.(*hcsschema.ProcessParameters)
	if !ok {
		return nil, errors.New("unsupported process config passed in")
	}

	path, args := seperateArgs(conf.CommandLine)
	absPath, err := findExecutable(path, c.sandboxMount)
	if err != nil {
		return nil, fmt.Errorf("failed to find executable: %s", err)
	}

	token, err := processToken(conf.User)
	if err != nil {
		return nil, fmt.Errorf("failed to create process token: %s", err)
	}

	cmd := &exec.Cmd{
		Env:  os.Environ(),
		Dir:  c.sandboxMount,
		Path: absPath,
		Args: args,
		SysProcAttr: &syscall.SysProcAttr{
			CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
			Token:         syscall.Token(token),
		},
	}

	process := newProcess(cmd)

	var (
		stdin  io.WriteCloser
		stdout io.ReadCloser
		stderr io.ReadCloser
	)
	// Create process pipes if asked for.
	if conf.CreateStdInPipe {
		stdin, err = process.cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stdin pipe: %s", err)
		}
		process.stdin = stdin
	}

	if conf.CreateStdOutPipe {
		stdout, err = process.cmd.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout pipe: %s", err)
		}
		process.stdout = stdout
	}

	if conf.CreateStdErrPipe {
		stderr, err = process.cmd.StderrPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stderr pipe: %s", err)
		}
		process.stderr = stderr
	}

	defer func() {
		if err != nil {
			process.Close()
			c.Release(ctx)
		}
	}()

	// TODO (dcantah): Start this suspended, add to job, then unsuspend
	if err = process.start(); err != nil {
		return nil, fmt.Errorf("failed to start host process: %s", err)
	}

	if err = c.job.assign(process); err != nil {
		return nil, fmt.Errorf("failed to assign process to job object: %s", err)
	}

	// Close initBlock, and only do this once for the init process. This signifies
	// that we can start listening for the IOCP notification that all processes have
	// exited in a job. This notification is what will close the container waitBlock
	// if it hasn't been already by Close.
	c.initDoOnce.Do(func() {
		close(c.initBlock)
	})

	// Wait for process exit
	go c.pollJobNotifs(ctx)
	go process.waitBackground(ctx)
	return process, nil
}

// Release releases any resources (just unmounts the sandbox for now)
// Safe to call multiple times, if no storage is mounted just returns nil.
func (c *HostJobContainer) Release(ctx context.Context) error {
	if c.sandboxMount != "" {
		if err := removeSandboxMountPoint(ctx, c.sandboxMount); err != nil {
			log.G(ctx).WithFields(logrus.Fields{
				"id":   c.id,
				"path": c.sandboxMount,
			}).Warn("failed to remove sandbox volume mount")
			return fmt.Errorf("failed to remove sandbox volume mount path: %s", err)
		}
		if err := hcsoci.UnmountContainerLayers(ctx, c.spec.Windows.LayerFolders, "", nil, hcsoci.UnmountOperationAll); err != nil {
			log.G(ctx).WithFields(logrus.Fields{
				"id":   c.id,
				"path": c.spec.Windows.LayerFolders,
			}).Warn("failed to unmount container layers")
			return fmt.Errorf("failed to unmount container layers: %s", err)
		}
		c.sandboxMount = ""
	}
	return nil
}

// Start starts the container. This is a noop as there isn't anything to
// start really.
func (c *HostJobContainer) Start(ctx context.Context) error {
	return nil
}

// Close closes any open handles.
func (c *HostJobContainer) Close() error {
	c.handleLock.Lock()
	defer c.handleLock.Unlock()
	if err := c.job.close(); err != nil {
		return err
	}
	c.closedWaitOnce.Do(func() {
		c.waitError = hcs.ErrAlreadyClosed
		close(c.waitBlock)
	})
	return nil
}

// ID returns the ID of the container (also its job object name)
func (c *HostJobContainer) ID() string {
	return c.id
}

// Shutdown gracefully shuts down the container.
func (c *HostJobContainer) Shutdown(ctx context.Context) error {
	log.G(ctx).WithField("id", c.id).Debug("shutting down host job container")
	c.handleLock.Lock()
	defer c.handleLock.Unlock()
	return c.job.shutdown(ctx)
}

// PropertiesV2 is not implemented for privileged containers. This is just to satisfy the cow.Container interface.
func (c *HostJobContainer) PropertiesV2(ctx context.Context, types ...hcsschema.PropertyType) (*hcsschema.Properties, error) {
	return nil, errors.New("propertiesV2 call not implemented for privileged containers")
}

// Properties is not implemented for privileged containers. This is just to satisfy the cow.Container interface.
func (c *HostJobContainer) Properties(ctx context.Context, types ...schema1.PropertyType) (*schema1.ContainerProperties, error) {
	return nil, errors.New("properties call not implemented for privileged containers")
}

// Terminate terminates the job object (kills every process in the job).
func (c *HostJobContainer) Terminate(ctx context.Context) error {
	log.G(ctx).WithField("id", c.id).Debug("terminating host job container")
	c.handleLock.Lock()
	defer c.handleLock.Unlock()
	if err := c.job.terminate(); err != nil {
		log.G(ctx).WithField("id", c.id).Error("failed to terminate container job object")
		return err
	}
	return nil
}

// Wait synchronously waits for the container to shutdown or terminate. If
// the container has already exited returns the previous error (if any).
func (c *HostJobContainer) Wait() error {
	<-c.waitBlock
	return c.waitError
}

func (c *HostJobContainer) waitBackground() {
	<-c.initBlock
	<-c.exitBlock
	c.closedWaitOnce.Do(func() {
		c.waitError = nil
		close(c.waitBlock)
	})
}

// Polls for notifications from the job objects assigned IO completion port.
func (c *HostJobContainer) pollJobNotifs(ctx context.Context) {
	if c.job == nil {
		return
	}
	for {
		if c.job.iocpHandle != 0 {
			code, err := c.job.pollIOCP()
			if err != nil {
				// TODO (dcantah): Continue here or return?
				log.G(ctx).WithError(err).Error("failed to poll IOCP")
				continue
			}
			switch code {
			// Notification that all processes in the job have exited. Close exited
			// channel to free up container wait.
			case winapi.JOB_OBJECT_MSG_ACTIVE_PROCESS_ZERO:
				close(c.exitBlock)
			default:
			}
		} else {
			log.G(ctx).WithFields(logrus.Fields{
				"ID": c.ID(),
			}).Debug("IOCP handle is 0. Polling completed")
			return
		}
	}
}

// IsOCI - Just to satisfy the cow.ProcessHost interface. Follow the WCOW behavior
func (c *HostJobContainer) IsOCI() bool {
	return false
}

// OS returns the operating system name as a string. This should always be windows
func (c *HostJobContainer) OS() string {
	return "windows"
}
