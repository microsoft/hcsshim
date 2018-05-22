package gcs

import (
	"bufio"
	"encoding/json"
	"os"
	"path"
	"sync"

	"github.com/Microsoft/opengcs/service/gcs/gcserr"
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
		inCon:  inCon,
		outCon: outCon,
		errCon: errCon,
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
		return c.container.Pid(), err
	} else {
		p, err := c.container.ExecProcess(*process, stdioSet)
		if err != nil {
			return -1, err
		}
		return p.Pid(), nil
	}
}
