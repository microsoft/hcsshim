package hcsshim

import (
	"encoding/json"
	"errors"
	"runtime"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
)

type container struct {
	handle hcsSystem
	id     string
}

type containerProperties struct {
	Id                string
	Name              string
	SystemType        string
	Owner             string
	SiloGuid          string `json:",omitempty"`
	IsDummy           bool   `json:",omitempty"`
	RuntimeId         string `json:",omitempty"`
	Stopped           bool   `json:",omitempty"`
	ExitType          string `json:",omitempty"`
	AreUpdatesPending bool   `json:",omitempty"`
}

// CreateContainer creates a new container with the given configuration but does not start it.
func CreateContainer(id string, c *ContainerConfig) (Container, error) {
	title := "HCSShim::CreateContainer"
	logrus.Debugf(title+" id=%s", id)
	var (
		handle  hcsSystem
		resultp *uint16
	)

	configurationb, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	configuration := string(configurationb)

	err = hcsCreateComputeSystemTP5(id, configuration, &handle, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		err = makeErrorf(err, title, "id=%s configuration=%s", id, configuration)
		logrus.Error(err)
		return nil, err
	}

	container := &container{
		handle: handle,
		id:     id,
	}

	logrus.Debugf(title+" succeeded id=%s handle=%d", id, container.handle)
	runtime.SetFinalizer(container, closeContainer)
	return container, nil
}

// OpenContainer opens an existing container by ID.
func OpenContainer(id string) (Container, error) {
	title := "HCSShim::OpenContainer"
	logrus.Debugf(title+" id=%s", id)
	var (
		handle  hcsSystem
		resultp *uint16
	)

	err := hcsOpenComputeSystem(id, &handle, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		err = makeErrorf(err, title, "id=%s", id)
		logrus.Error(err)
		return nil, err
	}

	container := &container{
		handle: handle,
		id:     id,
	}

	logrus.Debugf(title+" succeeded id=%s handle=%d", id, handle)
	runtime.SetFinalizer(container, closeContainer)
	return container, nil
}

// Start synchronously starts the container.
func (container *container) Start() error {
	title := "HCSShim::Container::Start"
	logrus.Debugf(title+" id=%s", container.id)
	var (
		//options string
		resultp *uint16
	)

	err := hcsStartComputeSystemTP5(container.handle, nil, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		err = makeErrorf(err, title, "%d", container.handle)
		logrus.Error(err)
		return err
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

// Shutdown requests a container shutdown, but it may not actually be shutdown until Wait() succeeds.
func (container *container) Shutdown() error {
	title := "HCSShim::Container::Shutdown"
	logrus.Debugf(title+" id=%s", container.id)
	var (
		//options string
		resultp *uint16
	)

	err := hcsShutdownComputeSystemTP5(container.handle, nil, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		err = makeError(err, title, "")
		logrus.Error(err)
		return err
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

// Terminate requests a container terminate, but it may not actually be terminated until Wait() succeeds.
func (container *container) Terminate() error {
	title := "HCSShim::Container::Terminate"
	logrus.Debugf(title+" id=%s", container.id)
	var (
		//options string
		resultp *uint16
	)

	err := hcsTerminateComputeSystemTP5(container.handle, nil, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		err = makeError(err, title, "")
		logrus.Error(err)
		return err
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

// Wait synchronously waits for the container to shutdown or terminate.
func (container *container) Wait() error {
	_, err := container.waitTimeoutInternal(syscall.INFINITE)
	return err
}

// WaitTimeout synchronously waits for the container to terminate or the duration to elapse. It
// returns true if the event occurs successfully without timeout.
func (container *container) WaitTimeout(timeout time.Duration) (bool, error) {
	return waitTimeoutHelper(container, timeout)
}

func (container *container) hcsWait(exitEvent *syscall.Handle, result **uint16) error {
	return hcsCreateComputeSystemWait(container.handle, exitEvent, result)
}

func (container *container) waitTimeoutInternal(timeout uint32) (bool, error) {
	return waitTimeoutInternalHelper(container, timeout)
}

func (container *container) properties() (*containerProperties, error) {
	title := "HCSShim::Container::Properties"
	logrus.Debugf(title+" id=%s", container.id)
	var (
		resultp     *uint16
		propertiesp *uint16
	)

	err := hcsGetComputeSystemProperties(container.handle, "", &propertiesp, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		err = makeError(err, title, "")
		logrus.Error(err)
		return nil, err
	}

	if propertiesp == nil {
		return nil, errors.New("Unexpected result from hcsGetComputeSystemProperties, properties should never be nil")
	}
	propertiesRaw := convertAndFreeCoTaskMemBytes(propertiesp)

	properties := &containerProperties{}
	if err := json.Unmarshal(propertiesRaw, properties); err != nil {
		return nil, err
	}

	logrus.Debugf(title+" succeeded id=%s properties=%s", container.id, propertiesRaw)
	return properties, nil
}

// HasPendingUpdates returns true if the container has updates pending to install
func (container *container) HasPendingUpdates() (bool, error) {
	title := "HCSShim::Container::HasPendingUpdates"
	logrus.Debugf(title+" id=%s", container.id)
	properties, err := container.properties()
	if err != nil {
		return false, err
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return properties.AreUpdatesPending, nil
}

// Pause pauses the execution of the container. This feature is not enabled in TP5.
func (container *container) Pause() error {
	title := "HCSShim::Container::Pause"
	logrus.Debugf(title+" id=%s", container.id)
	var (
		//options string
		resultp *uint16
	)

	err := hcsPauseComputeSystemTP5(container.handle, nil, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		err = makeError(err, title, "")
		logrus.Error(err)
		return err
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

// Resume resumes the execution of the container. This feature is not enabled in TP5.
func (container *container) Resume() error {
	title := "HCSShim::Container::Resume"
	logrus.Debugf(title+" id=%s", container.id)
	var (
		//options string
		resultp *uint16
	)

	err := hcsResumeComputeSystemTP5(container.handle, nil, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		err = makeError(err, title, "")
		logrus.Error(err)
		return err
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

// CreateProcess launches a new process within the container.
func (container *container) CreateProcess(c *ProcessConfig) (Process, error) {
	title := "HCSShim::Container::CreateProcess"
	logrus.Debugf(title+" id=%s", container.id)
	var (
		processInfo   hcsProcessInformation
		processHandle hcsProcess
		resultp       *uint16
	)

	// If we are not emulating a console, ignore any console size passed to us
	if !c.EmulateConsole {
		c.ConsoleSize[0] = 0
		c.ConsoleSize[1] = 0
	}

	configurationb, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	configuration := string(configurationb)

	err = hcsCreateProcess(container.handle, configuration, &processInfo, &processHandle, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		err = makeError(err, title, "")
		logrus.Error(err)
		return nil, err
	}

	logrus.Debugf("Created process returned: %s", processInfo)

	process := &process{
		handle:    processHandle,
		processID: int(processInfo.ProcessId),
		cachedPipes: &cachedPipes{
			stdIn:  processInfo.StdInput,
			stdOut: processInfo.StdOutput,
			stdErr: processInfo.StdError,
		},
	}

	logrus.Debugf(title+" succeeded id=%s processid=%s", container.id, process.processID)
	runtime.SetFinalizer(process, closeProcess)
	return process, nil
}

// OpenProcess gets an interface to an existing process within the container.
func (container *container) OpenProcess(pid int) (Process, error) {
	title := "HCSShim::Container::OpenProcess"
	logrus.Debugf(title+" id=%s", container.id)
	var (
		processHandle hcsProcess
		resultp       *uint16
	)

	err := hcsOpenProcess(container.handle, uint32(pid), &processHandle, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		err = makeError(err, title, "")
		logrus.Error(err)
		return nil, err
	}

	process := &process{
		handle:    processHandle,
		processID: pid,
	}

	logrus.Debugf(title+" succeeded id=%s processid=%s", container.id, process.processID)
	runtime.SetFinalizer(process, closeProcess)
	return process, nil
}

// title returns a string representing the object title
func (container *container) title() string {
	return "Container"
}

// Close cleans up any state associated with the container but does not terminate or wait for it.
func (container *container) Close() error {
	title := "HCSShim::Container::Close"
	logrus.Debugf(title+" id=%s", container.id)

	// Don't double free this
	if container.handle == 0 {
		return nil
	}

	if err := hcsCloseComputeSystem(container.handle); err != nil {
		err = makeError(err, title, "")
		logrus.Error(err)
		return err
	}

	container.handle = 0

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

// closeContainer wraps container.Close for use by a finalizer
func closeContainer(container *container) {
	container.Close()
}
