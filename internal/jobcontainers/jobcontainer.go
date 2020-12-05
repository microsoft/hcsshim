package jobcontainers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/queue"
	"github.com/Microsoft/hcsshim/internal/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

// Split arguments but ignore spaces in quotes.
//
// For example instead of:
// "\"Hello good\" morning world" --> ["\"Hello", "good\"", "morning", "world"]
// we get ["\"Hello good\"", "morning", "world"]
func splitArgs(cmdLine string) []string {
	r := regexp.MustCompile(`[^\s"]+|"([^"]*)"`)
	return r.FindAllString(cmdLine, -1)
}

// Convert environment map to a slice of environment variables in the form [Key1=val1, key2=val2]
func envMapToSlice(m map[string]string) []string {
	var s []string
	for k, v := range m {
		s = append(s, k+"="+v)
	}
	return s
}

const (
	jobContainerNameFmt = "JobContainer_%s"
	// Environment variable set in every process in the job detailing where the containers volume
	// is mounted on the host.
	sandboxMountPointEnvVar = "CONTAINER_SANDBOX_MOUNT_POINT"
)

type initProc struct {
	initDoOnce sync.Once
	proc       *JobProcess
	initBlock  chan struct{}
}

// JobContainer represents a lightweight container composed from a job object.
type JobContainer struct {
	id             string
	spec           *specs.Spec          // OCI spec used to create the container
	job            *jobobject.JobObject // Object representing the job object the container owns
	sandboxMount   string               // Path to where the sandbox is mounted on the host
	m              sync.Mutex
	closedWaitOnce sync.Once
	init           initProc
	startTimestamp time.Time
	exited         chan struct{}
	waitBlock      chan struct{}
	waitError      error
}

var _ cow.ProcessHost = &JobContainer{}
var _ cow.Container = &JobContainer{}

func newJobContainer(id string, s *specs.Spec) *JobContainer {
	return &JobContainer{
		id:        id,
		spec:      s,
		waitBlock: make(chan struct{}),
		exited:    make(chan struct{}),
		init:      initProc{initBlock: make(chan struct{})},
	}
}

// Create creates a new JobContainer from `s`.
func Create(ctx context.Context, id string, s *specs.Spec) (_ cow.Container, err error) {
	log.G(ctx).WithField("id", id).Debug("Creating job container")

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
		return nil, errors.Wrap(err, "failed to mount container layers")
	}

	volumeGUIDRegex := `^\\\\\?\\(Volume)\{{0,1}[0-9a-fA-F]{8}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{12}(\}){0,1}\}(|\\)$`
	if matched, err := regexp.MatchString(volumeGUIDRegex, s.Root.Path); !matched || err != nil {
		return nil, fmt.Errorf(`invalid container spec - Root.Path '%s' must be a volume GUID path in the format '\\?\Volume{GUID}\'`, s.Root.Path)
	}
	if s.Root.Path[len(s.Root.Path)-1] != '\\' {
		s.Root.Path += `\` // Be nice to clients and make sure well-formed for back-compat
	}

	container := newJobContainer(id, s)

	// Create the job object all processes will run in.
	options := &jobobject.Options{
		Name:          fmt.Sprintf(jobContainerNameFmt, id),
		Notifications: true,
	}
	job, err := jobobject.Create(ctx, options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create job object")
	}

	// Parity with how we handle process isolated containers. We set the same flag which
	// behaves the same way for a silo.
	if err := job.SetTerminateOnLastHandleClose(); err != nil {
		return nil, errors.Wrap(err, "failed to set terminate on last handle close on job container")
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

	limits, err := specToLimits(ctx, id, s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert OCI spec to job object limits")
	}

	// Set resource limits on the job object based off of oci spec.
	if err := job.SetResourceLimits(limits); err != nil {
		return nil, errors.Wrap(err, "failed to set resource limits")
	}

	// Setup directory sandbox volume will be mounted
	sandboxPath := fmt.Sprintf(sandboxMountFormat, id)
	if _, err := os.Stat(sandboxPath); os.IsNotExist(err) {
		if err := os.MkdirAll(sandboxPath, 0777); err != nil {
			return nil, errors.Wrap(err, "failed to create mounted folder")
		}
	}
	path = sandboxPath

	if err := mountSandboxVolume(ctx, path, s.Root.Path); err != nil {
		return nil, errors.Wrap(err, "failed to bind payload directory on host")
	}

	container.sandboxMount = path
	go container.waitBackground(ctx)
	return container, nil
}

// CreateProcess creates a process on the host, starts it, adds it to the containers
// job object and then waits for exit.
func (c *JobContainer) CreateProcess(ctx context.Context, config interface{}) (_ cow.Process, err error) {
	conf, ok := config.(*hcsschema.ProcessParameters)
	if !ok {
		return nil, errors.New("unsupported process config passed in")
	}

	if conf.EmulateConsole {
		return nil, errors.New("console emulation not supported for job containers")
	}

	absPath, commandLine, err := getApplicationName(conf.CommandLine, c.sandboxMount, os.Getenv("PATH"))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get application name from commandline %q", conf.CommandLine)
	}

	commandLine = strings.ReplaceAll(commandLine, "%"+sandboxMountPointEnvVar+"%", c.sandboxMount)
	commandLine = strings.ReplaceAll(commandLine, "$env:"+sandboxMountPointEnvVar, c.sandboxMount)

	var token windows.Token
	if getUserTokenInheritAnnotation(c.spec.Annotations) {
		token, err = openCurrentProcessToken()
		if err != nil {
			return nil, err
		}
	} else {
		token, err = processToken(conf.User)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create user process token")
		}
	}
	defer token.Close()

	env, err := defaultEnvBlock(token)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get default environment block")
	}
	env = append(env, envMapToSlice(conf.Environment)...)
	env = append(env, sandboxMountPointEnvVar+"="+c.sandboxMount)

	cmd := &exec.Cmd{
		Env:  env,
		Dir:  c.sandboxMount,
		Path: absPath,
		Args: splitArgs(commandLine),
		SysProcAttr: &syscall.SysProcAttr{
			CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
			Token:         syscall.Token(token),
		},
	}
	process := newProcess(cmd)

	// Create process pipes if asked for.
	if conf.CreateStdInPipe {
		stdin, err := process.cmd.StdinPipe()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create stdin pipe")
		}
		process.stdin = stdin
	}

	if conf.CreateStdOutPipe {
		stdout, err := process.cmd.StdoutPipe()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create stdout pipe")
		}
		process.stdout = stdout
	}

	if conf.CreateStdErrPipe {
		stderr, err := process.cmd.StderrPipe()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create stderr pipe")
		}
		process.stderr = stderr
	}

	defer func() {
		if err != nil {
			process.Close()
		}
	}()

	if err = process.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start host process")
	}

	if err = c.job.Assign(uint32(process.Pid())); err != nil {
		return nil, errors.Wrap(err, "failed to assign process to job object")
	}

	// Assign the first process made as the init process of the container.
	c.init.initDoOnce.Do(func() {
		c.init.proc = process
		close(c.init.initBlock)
	})

	// Wait for process exit
	go c.pollJobMsgs(ctx)
	go process.waitBackground(ctx)
	return process, nil
}

func (c *JobContainer) Modify(ctx context.Context, config interface{}) (err error) {
	return errors.New("modify not supported for job containers")
}

// Release unmounts all of the container layers. Safe to call multiple times, if no storage
// is mounted this call will just return nil.
func (c *JobContainer) Release(ctx context.Context) error {
	c.m.Lock()
	defer c.m.Unlock()

	log.G(ctx).WithFields(logrus.Fields{
		"id":   c.id,
		"path": c.sandboxMount,
	}).Warn("removing sandbox volume mount")

	if c.sandboxMount != "" {
		if err := removeSandboxMountPoint(ctx, c.sandboxMount); err != nil {
			return errors.Wrap(err, "failed to remove sandbox volume mount path")
		}
		if err := layers.UnmountContainerLayers(ctx, c.spec.Windows.LayerFolders, "", nil, layers.UnmountOperationAll); err != nil {
			return errors.Wrap(err, "failed to unmount container layers")
		}
		c.sandboxMount = ""
	}
	return nil
}

// Start starts the container. There's nothing to "start" for job containers, so this just
// sets the start timestamp.
func (c *JobContainer) Start(ctx context.Context) error {
	c.startTimestamp = time.Now()
	return nil
}

// Close closes any open handles.
func (c *JobContainer) Close() error {
	if err := c.job.Close(); err != nil {
		return err
	}
	c.closedWaitOnce.Do(func() {
		c.waitError = hcs.ErrAlreadyClosed
		close(c.waitBlock)
	})
	return nil
}

// ID returns the ID of the container. This is the name used to create the job object.
func (c *JobContainer) ID() string {
	return c.id
}

// Shutdown gracefully shuts down the container.
func (c *JobContainer) Shutdown(ctx context.Context) error {
	log.G(ctx).WithField("id", c.id).Debug("shutting down job container")

	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	return c.shutdown(ctx)
}

// shutdown will loop through all the pids in the container and send a signal to exit.
// If there are no processes in the container it will early return nil.
// If the all processes exited message is not received within the context timeout set, it will
// terminate the job.
func (c *JobContainer) shutdown(ctx context.Context) error {
	pids, err := c.job.Pids()
	if err != nil {
		return errors.Wrap(err, "failed to get pids in container")
	}

	if len(pids) == 0 {
		return nil
	}

	for _, pid := range pids {
		// If any process can't be signaled just wait until the timeout hits
		if err := signalProcess(pid, windows.CTRL_SHUTDOWN_EVENT); err != nil {
			log.G(ctx).WithField("pid", pid).Error("failed to signal process in job container")
		}
	}

	select {
	case <-c.exited:
	case <-ctx.Done():
		return c.Terminate(ctx)
	}
	return nil
}

// PropertiesV2 returns properties relating to the job container. This is an HCS construct but
// to adhere to the interface for containers on Windows it is partially implemented. The only
// supported property is schema2.PTStatistics.
func (c *JobContainer) PropertiesV2(ctx context.Context, types ...hcsschema.PropertyType) (*hcsschema.Properties, error) {
	return nil, errors.New("`PropertiesV2` call is not implemented for job containers")
}

// Properties is not implemented for job containers. This is just to satisfy the cow.Container interface.
func (c *JobContainer) Properties(ctx context.Context, types ...schema1.PropertyType) (*schema1.ContainerProperties, error) {
	return nil, errors.New("`Properties` call is not implemented for job containers")
}

// Terminate terminates the job object (kills every process in the job).
func (c *JobContainer) Terminate(ctx context.Context) error {
	log.G(ctx).WithField("id", c.id).Debug("terminating job container")

	if err := c.job.Terminate(1); err != nil {
		return errors.Wrap(err, "failed to terminate job container")
	}
	return nil
}

// Wait synchronously waits for the container to shutdown or terminate. If
// the container has already exited returns the previous error (if any).
func (c *JobContainer) Wait() error {
	<-c.waitBlock
	return c.waitError
}

func (c *JobContainer) waitBackground(ctx context.Context) {
	// Wait for there to be an init process assigned.
	<-c.init.initBlock

	// Once the init process finishes, if there's any other processes in the container we need to signal
	// them to exit.
	<-c.init.proc.waitBlock

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.Shutdown(ctx); err != nil {
		c.Terminate(ctx)
	}

	c.closedWaitOnce.Do(func() {
		c.waitError = c.init.proc.waitError
		close(c.waitBlock)
	})
}

// Polls for notifications from the job objects assigned IO completion port.
func (c *JobContainer) pollJobMsgs(ctx context.Context) {
	for {
		notif, err := c.job.PollNotification()
		if err != nil {
			// Queues closed or we somehow aren't registered to receive notifications. There won't be
			// any notifications arriving so we're safe to return.
			if err == queue.ErrQueueClosed || err == jobobject.ErrNotRegistered {
				return
			}
			log.G(ctx).WithError(err).Warn("error while polling for job container notification")
		}

		switch msg := notif.(type) {
		// All processes have exited. Close the waitblock so we can cleanup and then return.
		case jobobject.MsgAllProcessesExited:
			close(c.exited)
			return
		case jobobject.MsgUnimplemented:
		default:
			log.G(ctx).WithField("message", msg).Warn("unknown job object notification encountered")
		}
	}
}

// IsOCI - Just to satisfy the cow.ProcessHost interface. Follow the WCOW behavior
func (c *JobContainer) IsOCI() bool {
	return false
}

// OS returns the operating system name as a string. This should always be windows.
func (c *JobContainer) OS() string {
	return "windows"
}
