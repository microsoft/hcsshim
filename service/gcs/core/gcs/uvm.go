package gcs

import (
	"bufio"
	"encoding/json"
	"os"
	"path"
	"sync"
	"syscall"

	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

type delayedVsockConnection struct {
	actualConnection transport.Connection
}

func newDelayedVsockConnetion() transport.Connection {
	return &delayedVsockConnection{}
}

func (d *delayedVsockConnection) Read(p []byte) (n int, err error) {
	if d.actualConnection != nil {
		return d.actualConnection.Read(p)
	}
	return 0, errors.New("not implemented")
}

func (d *delayedVsockConnection) Write(p []byte) (n int, err error) {
	if d.actualConnection != nil {
		return d.actualConnection.Write(p)
	}
	return 0, errors.New("not implemented")
}

func (d *delayedVsockConnection) Close() error {
	if d.actualConnection != nil {
		return d.actualConnection.Close()
	}
	return nil
}
func (d *delayedVsockConnection) CloseRead() error {
	if d.actualConnection != nil {
		return d.actualConnection.CloseRead()
	}
	return nil
}
func (d *delayedVsockConnection) CloseWrite() error {
	if d.actualConnection != nil {
		return d.actualConnection.CloseWrite()
	}
	return nil
}
func (d *delayedVsockConnection) File() (*os.File, error) {
	if d.actualConnection != nil {
		return d.actualConnection.File()
	}
	return nil, errors.New("not implemented")
}

type Host struct {
	containersMutex sync.Mutex
	containers      map[string]*Container

	// Rtime is the Runtime interface used by the GCS core.
	rtime runtime.Runtime
}

func NewHost(rtime runtime.Runtime) *Host {
	return &Host{rtime: rtime, containers: make(map[string]*Container)}
}

func (h *Host) getContainerLocked(id string) (*Container, error) {
	if c, ok := h.containers[id]; !ok {
		return nil, errors.WithStack(gcserr.NewContainerDoesNotExistError(id))
	} else {
		return c, nil
	}
}

func (h *Host) GetContainer(id string) (*Container, error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	return h.getContainerLocked(id)
}

func (h *Host) GetOrCreateContainer(id string, settings *prot.VMHostedContainerSettingsV2) (*Container, error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	c, err := h.getContainerLocked(id)
	if err == nil {
		return c, nil
	}

	// Container doesnt exit. Create it here
	// Create the BundlePath
	if err := os.MkdirAll(settings.OCIBundlePath, 0700); err != nil {
		return nil, errors.Wrapf(err, "failed to create OCIBundlePath: '%s'", settings.OCIBundlePath)
	}
	configFile := path.Join(settings.OCIBundlePath, "config.json")
	f, err := os.Create(configFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create config.json at: '%s'", configFile)
	}
	defer f.Close()
	writer := bufio.NewWriter(f)
	if err := json.NewEncoder(writer).Encode(settings.OCISpecification); err != nil {
		return nil, errors.Wrapf(err, "failed to write OCISpecification to config.json at: '%s'", configFile)
	}
	if err := writer.Flush(); err != nil {
		return nil, errors.Wrapf(err, "failed to flush writer for config.json at: '%s'", configFile)
	}

	inCon := new(delayedVsockConnection)
	outCon := new(delayedVsockConnection)
	errCon := new(delayedVsockConnection)
	c = &Container{
		initProcess: settings.OCISpecification.Process,
		initConnectionSet: &stdio.ConnectionSet{
			In:  inCon,
			Out: outCon,
			Err: errCon,
		},
		inCon:     inCon,
		outCon:    outCon,
		errCon:    errCon,
		processes: make(map[uint32]*Process),
	}
	con, err := h.rtime.CreateContainer(id, settings.OCIBundlePath, c.initConnectionSet)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create container")
	}
	c.container = con
	h.containers[id] = c
	return c, nil
}

type Container struct {
	container             runtime.Container
	initProcess           *oci.Process
	initConnectionSet     *stdio.ConnectionSet
	inCon, outCon, errCon *delayedVsockConnection

	processesMutex sync.Mutex
	processes      map[uint32]*Process
}

func (c *Container) ExecProcess(process *oci.Process, stdioSet *stdio.ConnectionSet) (int, error) {
	if process == nil {
		if stdioSet.In != nil {
			c.inCon.actualConnection = stdioSet.In
		}
		if stdioSet.Out != nil {
			c.outCon.actualConnection = stdioSet.Out
		}
		if stdioSet.Err != nil {
			c.errCon.actualConnection = stdioSet.Err
		}
		err := c.container.Start()
		pid := c.container.Pid()
		if err == nil {
			// Kind of odd but track the container init process in its own map.
			c.processesMutex.Lock()
			c.processes[uint32(pid)] = &Process{process: c.container, pid: pid}
			c.processesMutex.Unlock()
		}

		return pid, err
	} else {
		p, err := c.container.ExecProcess(*process, stdioSet)
		if err != nil {
			return -1, err
		}
		pid := p.Pid()
		c.processesMutex.Lock()
		c.processes[uint32(pid)] = &Process{process: p, pid: pid}
		c.processesMutex.Unlock()
		return pid, nil
	}
}

func (c *Container) GetProcess(pid uint32) (*Process, error) {
	c.processesMutex.Lock()
	defer c.processesMutex.Unlock()

	p, ok := c.processes[pid]
	if !ok {
		return nil, errors.WithStack(gcserr.NewProcessDoesNotExistError(int(pid)))
	}
	return p, nil
}

func (c *Container) Kill(signal oslayer.Signal) error {
	return c.container.Kill(signal)
}

func (c *Container) Wait() func() (int, error) {
	f := func() (int, error) {
		s, err := c.container.Wait()
		if err != nil {
			return -1, err
		}
		return s.ExitCode(), nil
	}
	return f
}

type Process struct {
	process  runtime.Process
	pid      int
	exitCode *int
}

func (p *Process) Kill(signal syscall.Signal) error {
	if err := syscall.Kill(int(p.pid), signal); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (p *Process) Wait() (chan int, chan bool) {
	exitCodeChan := make(chan int, 1)
	doneChan := make(chan bool)

	go func() {
		bgExitCodeChan := make(chan int, 1)
		go func() {
			state, err := p.process.Wait()
			if err != nil {
				bgExitCodeChan <- -1
			}
			bgExitCodeChan <- state.ExitCode()
		}()

		// Wait for the exit code or the caller to stop waiting.
		select {
		case exitCode := <-bgExitCodeChan:
			exitCodeChan <- exitCode

			// The caller got the exit code. Wait for them to tell us they have
			// issued the write
			select {
			case <-doneChan:
			}

		case <-doneChan:
		}
	}()
	return exitCodeChan, doneChan
}
