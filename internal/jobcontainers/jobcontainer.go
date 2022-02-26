package jobcontainers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/conpty"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/exec"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/queue"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/winapi"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
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
	id               string
	spec             *specs.Spec          // OCI spec used to create the container
	job              *jobobject.JobObject // Object representing the job object the container owns
	sandboxMount     string               // Path to where the sandbox is mounted on the host
	closedWaitOnce   sync.Once
	init             initProc
	token            windows.Token
	localUserAccount string
	startTimestamp   time.Time
	exited           chan struct{}
	waitBlock        chan struct{}
	waitError        error
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
func Create(ctx context.Context, id string, s *specs.Spec) (_ cow.Container, _ *resources.Resources, err error) {
	log.G(ctx).WithField("id", id).Debug("Creating job container")

	if s == nil {
		return nil, nil, errors.New("Spec must be supplied")
	}

	if id == "" {
		g, err := guid.NewV4()
		if err != nil {
			return nil, nil, err
		}
		id = g.String()
	}

	container := newJobContainer(id, s)

	// Create the job object all processes will run in.
	options := &jobobject.Options{
		Name:          fmt.Sprintf(jobContainerNameFmt, id),
		Notifications: true,
	}
	job, err := jobobject.Create(ctx, options)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create job object")
	}

	// Parity with how we handle process isolated containers. We set the same flag which
	// behaves the same way for a silo.
	if err := job.SetTerminateOnLastHandleClose(); err != nil {
		return nil, nil, errors.Wrap(err, "failed to set terminate on last handle close on job container")
	}
	container.job = job

	r := resources.NewContainerResources(id)
	defer func() {
		if err != nil {
			container.Close()
			_ = resources.ReleaseResources(ctx, r, nil, true)
		}
	}()

	sandboxPath := fmt.Sprintf(sandboxMountFormat, id)
	if err := mountLayers(ctx, id, s, sandboxPath); err != nil {
		return nil, nil, errors.Wrap(err, "failed to mount container layers")
	}
	container.sandboxMount = sandboxPath

	layers := layers.NewImageLayers(nil, "", s.Windows.LayerFolders, sandboxPath, false)
	r.SetLayers(layers)

	if err := setupMounts(s, container.sandboxMount); err != nil {
		return nil, nil, err
	}

	volumeGUIDRegex := `^\\\\\?\\(Volume)\{{0,1}[0-9a-fA-F]{8}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{12}(\}){0,1}\}(|\\)$`
	if matched, err := regexp.MatchString(volumeGUIDRegex, s.Root.Path); !matched || err != nil {
		return nil, nil, fmt.Errorf(`invalid container spec - Root.Path '%s' must be a volume GUID path in the format '\\?\Volume{GUID}\'`, s.Root.Path)
	}

	limits, err := specToLimits(ctx, id, s)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to convert OCI spec to job object limits")
	}

	// Set resource limits on the job object based off of oci spec.
	if err := job.SetResourceLimits(limits); err != nil {
		return nil, nil, errors.Wrap(err, "failed to set resource limits")
	}

	go container.waitBackground(ctx)
	return container, r, nil
}

// CreateProcess creates a process on the host, starts it, adds it to the containers
// job object and then waits for exit.
func (c *JobContainer) CreateProcess(ctx context.Context, config interface{}) (_ cow.Process, err error) {
	conf, ok := config.(*hcsschema.ProcessParameters)
	if !ok {
		return nil, errors.New("unsupported process config passed in")
	}

	// Replace any occurences of the sandbox mount point env variable in the commandline.
	// %CONTAINER_SANDBOX_MOUNTPOINT%\mybinary.exe -> C:\C\123456789\mybinary.exe
	commandLine, _ := c.replaceWithMountPoint(conf.CommandLine)

	removeDriveLetter := func(name string) string {
		// If just the letter and colon (C:) then replace with a single backslash. Else just trim the drive letter and leave the rest of the
		// path.
		if len(name) == 2 && name[1] == ':' {
			name = "\\"
		} else if len(name) > 2 && name[1] == ':' {
			name = name[2:]
		}
		return name
	}

	workDir := c.sandboxMount
	if conf.WorkingDirectory != "" {
		var changed bool
		// For now, we join the working directory requested with where the sandbox volume is located. It's expected that the default behavior
		// would be to treat all paths as relative to the volume.
		//
		// For example:
		// A working directory of C:\ would become C:\C\12345678\
		// A working directory of C:\work\dir would become C:\C\12345678\work\dir
		//
		// The below calls replaceWithMountPoint to replace any occurrences of the environment variable that points to where the container image
		// volume is mounted.
		workDir, changed = c.replaceWithMountPoint(conf.WorkingDirectory)
		// If the working directory was changed, that means the user supplied %CONTAINER_SANDBOX_MOUNT_POINT%\\my\dir or something similar.
		// In that case there's nothing left to do, as we don't want to join it with the mount point again.. If it *wasn't* changed, then we
		// need to join it with the mount point, as it's some normal path.
		if !changed {
			workDir = filepath.Join(c.sandboxMount, removeDriveLetter(workDir))
		}
	}

	// Reassign commandline here in case it needed to be quoted. For example if "foo bar baz" was supplied, and
	// "foo bar.exe" exists, then return: "\"foo bar\" baz"
	absPath, commandLine, err := getApplicationName(commandLine, workDir, os.Getenv("PATH"))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get application name from commandline %q", conf.CommandLine)
	}

	// If we haven't grabbed a token yet this is the init process being launched. Skip grabbing another token afterwards if we've already
	// done the work (c.token != 0), this would typically be for an exec being launched.
	if c.token == 0 {
		if inheritUserTokenIsSet(c.spec.Annotations) {
			c.token, err = openCurrentProcessToken()
			if err != nil {
				return nil, err
			}
		} else {
			c.token, err = c.processToken(ctx, conf.User)
			if err != nil {
				return nil, fmt.Errorf("failed to create user process token: %w", err)
			}
		}
	}

	env, err := defaultEnvBlock(c.token)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get default environment block")
	}

	// Convert environment map to a slice of environment variables in the form [Key1=val1, key2=val2]
	var envs []string
	for k, v := range conf.Environment {
		expanded, _ := c.replaceWithMountPoint(v)
		envs = append(envs, k+"="+expanded)
	}
	env = append(env, envs...)

	env = append(env, sandboxMountPointEnvVar+"="+c.sandboxMount)

	// exec.Cmd internally does its own path resolution and as part of this checks some well known file extensions on the file given (e.g. if
	// the user just provided /path/to/mybinary). CreateProcess is perfectly capable of launching an executable that doesn't have the .exe extension
	// so this adds an empty string entry to the end of what extensions GO checks against so that a binary with no extension can be launched.
	// The extensions are checked in order, so that if mybinary.exe and mybinary both existed in the same directory, mybinary.exe would be chosen.
	// This is mostly to handle a common Kubernetes test image named agnhost that has the main entrypoint as a binary named agnhost with no extension.
	// https://github.com/kubernetes/kubernetes/blob/d64e91878517b1208a0bce7e2b7944645ace8ede/test/images/agnhost/Dockerfile_windows
	if err := os.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD; "); err != nil {
		return nil, errors.Wrap(err, "failed to set PATHEXT")
	}

	var cpty *conpty.Pty
	if conf.EmulateConsole {
		cpty, err = conpty.Create(80, 20, 0)
		if err != nil {
			return nil, err
		}
	}

	cmd, err := exec.New(
		absPath,
		commandLine,
		exec.WithDir(workDir),
		exec.WithEnv(env),
		exec.WithToken(c.token),
		exec.WithJobObject(c.job),
		exec.WithConPty(cpty),
		exec.WithProcessFlags(windows.CREATE_BREAKAWAY_FROM_JOB),
		exec.WithStdio(conf.CreateStdOutPipe, conf.CreateStdErrPipe, conf.CreateStdInPipe),
	)
	if err != nil {
		return nil, err
	}
	process := newProcess(cmd, cpty)

	// Create process pipes if asked for.
	if conf.CreateStdInPipe {
		process.stdin = process.cmd.Stdin()
	}

	if conf.CreateStdOutPipe {
		process.stdout = process.cmd.Stdout()
	}

	if conf.CreateStdErrPipe {
		process.stderr = process.cmd.Stderr()
	}

	defer func() {
		if err != nil {
			process.Close()
		}
	}()

	if err = process.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start host process")
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

// Start starts the container. There's nothing to "start" for job containers, so this just
// sets the start timestamp.
func (c *JobContainer) Start(ctx context.Context) error {
	c.startTimestamp = time.Now()
	return nil
}

// Close free's up any resources (handles, temporary accounts).
func (c *JobContainer) Close() error {
	// Do not return the first error so we can finish cleaning up.

	var closeErr bool
	if err := c.job.Close(); err != nil {
		log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to close job object")
		closeErr = true
	}

	if err := c.token.Close(); err != nil {
		log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to close token")
		closeErr = true
	}

	// Delete the containers local account if one was created
	if c.localUserAccount != "" {
		if err := winapi.NetUserDel("", c.localUserAccount); err != nil {
			log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to delete local account")
			closeErr = true
		}
	}

	c.closedWaitOnce.Do(func() {
		c.waitError = hcs.ErrAlreadyClosed
		close(c.waitBlock)
	})
	if closeErr {
		return errors.New("failed to close one or more job container resources")
	}
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
	if len(types) == 0 {
		return nil, errors.New("no property types supplied for PropertiesV2 call")
	}
	if types[0] != hcsschema.PTStatistics {
		return nil, errors.New("PTStatistics is the only supported property type for job containers")
	}

	memInfo, err := c.job.QueryMemoryStats()
	if err != nil {
		return nil, errors.Wrap(err, "failed to query for job containers memory information")
	}

	processorInfo, err := c.job.QueryProcessorStats()
	if err != nil {
		return nil, errors.Wrap(err, "failed to query for job containers processor information")
	}

	storageInfo, err := c.job.QueryStorageStats()
	if err != nil {
		return nil, errors.Wrap(err, "failed to query for job containers storage information")
	}

	var privateWorkingSet uint64
	err = forEachProcessInfo(c.job, func(procInfo *winapi.SYSTEM_PROCESS_INFORMATION) {
		privateWorkingSet += uint64(procInfo.WorkingSetPrivateSize)
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get private working set for container")
	}

	return &hcsschema.Properties{
		Statistics: &hcsschema.Statistics{
			Timestamp:          time.Now(),
			Uptime100ns:        uint64(time.Since(c.startTimestamp)) / 100,
			ContainerStartTime: c.startTimestamp,
			Memory: &hcsschema.MemoryStats{
				MemoryUsageCommitBytes:            memInfo.JobMemory,
				MemoryUsageCommitPeakBytes:        memInfo.PeakJobMemoryUsed,
				MemoryUsagePrivateWorkingSetBytes: privateWorkingSet,
			},
			Processor: &hcsschema.ProcessorStats{
				RuntimeKernel100ns: uint64(processorInfo.TotalKernelTime),
				RuntimeUser100ns:   uint64(processorInfo.TotalUserTime),
				TotalRuntime100ns:  uint64(processorInfo.TotalKernelTime + processorInfo.TotalUserTime),
			},
			Storage: &hcsschema.StorageStats{
				ReadCountNormalized:  storageInfo.IoInfo.ReadOperationCount,
				ReadSizeBytes:        storageInfo.IoInfo.ReadTransferCount,
				WriteCountNormalized: storageInfo.IoInfo.WriteOperationCount,
				WriteSizeBytes:       storageInfo.IoInfo.WriteTransferCount,
			},
		},
	}, nil
}

// Properties returns properties relating to the job container. This is an HCS construct but
// to adhere to the interface for containers on Windows it is partially implemented. The only
// supported property is schema1.PropertyTypeProcessList.
func (c *JobContainer) Properties(ctx context.Context, types ...schema1.PropertyType) (*schema1.ContainerProperties, error) {
	if len(types) == 0 {
		return nil, errors.New("no property types supplied for Properties call")
	}
	if types[0] != schema1.PropertyTypeProcessList {
		return nil, errors.New("ProcessList is the only supported property type for job containers")
	}

	var processList []schema1.ProcessListItem
	err := forEachProcessInfo(c.job, func(procInfo *winapi.SYSTEM_PROCESS_INFORMATION) {
		proc := schema1.ProcessListItem{
			CreateTimestamp:              time.Unix(0, procInfo.CreateTime),
			ProcessId:                    uint32(procInfo.UniqueProcessID),
			ImageName:                    procInfo.ImageName.String(),
			UserTime100ns:                uint64(procInfo.UserTime),
			KernelTime100ns:              uint64(procInfo.KernelTime),
			MemoryCommitBytes:            uint64(procInfo.PrivatePageCount),
			MemoryWorkingSetPrivateBytes: uint64(procInfo.WorkingSetPrivateSize),
			MemoryWorkingSetSharedBytes:  uint64(procInfo.WorkingSetSize) - uint64(procInfo.WorkingSetPrivateSize),
		}
		processList = append(processList, proc)
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get process ")
	}

	return &schema1.ContainerProperties{ProcessList: processList}, nil
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

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := c.Shutdown(ctx); err != nil {
		_ = c.Terminate(ctx)
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

// For every process in the job `job`, run the function `work`. This can be used to grab/filter the SYSTEM_PROCESS_INFORMATION
// data from every process in a job.
func forEachProcessInfo(job *jobobject.JobObject, work func(*winapi.SYSTEM_PROCESS_INFORMATION)) error {
	procInfos, err := systemProcessInformation()
	if err != nil {
		return err
	}

	pids, err := job.Pids()
	if err != nil {
		return err
	}

	pidsMap := make(map[uint32]struct{})
	for _, pid := range pids {
		pidsMap[pid] = struct{}{}
	}

	for _, procInfo := range procInfos {
		if _, ok := pidsMap[uint32(procInfo.UniqueProcessID)]; ok {
			work(procInfo)
		}
	}
	return nil
}

// Get a slice of SYSTEM_PROCESS_INFORMATION for all of the processes running on the system.
func systemProcessInformation() ([]*winapi.SYSTEM_PROCESS_INFORMATION, error) {
	var (
		systemProcInfo *winapi.SYSTEM_PROCESS_INFORMATION
		procInfos      []*winapi.SYSTEM_PROCESS_INFORMATION
		// This happens to be the buffer size hcs uses but there's really no hard need to keep it
		// the same, it's just a sane default.
		size   = uint32(1024 * 512)
		bounds uintptr
	)
	for {
		b := make([]byte, size)
		systemProcInfo = (*winapi.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&b[0]))
		status := winapi.NtQuerySystemInformation(
			winapi.SystemProcessInformation,
			uintptr(unsafe.Pointer(systemProcInfo)),
			size,
			&size,
		)
		if winapi.NTSuccess(status) {
			// Cache the address of the end of our buffer so we can check we don't go past this
			// in some odd case.
			bounds = uintptr(unsafe.Pointer(&b[len(b)-1]))
			break
		} else if status != winapi.STATUS_INFO_LENGTH_MISMATCH {
			return nil, winapi.RtlNtStatusToDosError(status)
		}
	}

	for {
		if uintptr(unsafe.Pointer(systemProcInfo))+uintptr(systemProcInfo.NextEntryOffset) >= bounds {
			// The next entry is outside of the bounds of our buffer somehow, abort.
			return nil, errors.New("system process info entry exceeds allocated buffer")
		}
		procInfos = append(procInfos, systemProcInfo)
		if systemProcInfo.NextEntryOffset == 0 {
			break
		}
		systemProcInfo = (*winapi.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(uintptr(unsafe.Pointer(systemProcInfo)) + uintptr(systemProcInfo.NextEntryOffset)))
	}

	return procInfos, nil
}

// Takes a string and replaces any occurrences of CONTAINER_SANDBOX_MOUNT_POINT with where the containers' volume is mounted, as well as returning
// if the string actually contained the environment variable.
func (c *JobContainer) replaceWithMountPoint(str string) (string, bool) {
	newStr := strings.ReplaceAll(str, "%"+sandboxMountPointEnvVar+"%", c.sandboxMount[:len(c.sandboxMount)-1])
	newStr = strings.ReplaceAll(newStr, "$env:"+sandboxMountPointEnvVar, c.sandboxMount[:len(c.sandboxMount)-1])
	return newStr, str != newStr
}
