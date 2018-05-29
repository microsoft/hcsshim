package gcs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

const UVMContainerID = "00000000-0000-0000-0000-000000000000"

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
	osl   oslayer.OS
	vsock transport.Transport
}

func NewHost(rtime runtime.Runtime, osl oslayer.OS, vsock transport.Transport) *Host {
	return &Host{rtime: rtime, osl: osl, vsock: vsock, containers: make(map[string]*Container)}
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
	if err := h.osl.MkdirAll(settings.OCIBundlePath, 0700); err != nil {
		return nil, errors.Wrapf(err, "failed to create OCIBundlePath: '%s'", settings.OCIBundlePath)
	}
	configFile := path.Join(settings.OCIBundlePath, "config.json")
	f, err := h.osl.Create(configFile)
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

func (h *Host) ModifyHostSettings(settings *prot.ModifySettingRequest) error {
	type modifyFunc func(interface{}) error

	requestTypeFn := func(req prot.ModifyRequestType, setting interface{}, add, remove, update modifyFunc) error {
		switch req {
		case prot.MreqtAdd:
			if add != nil {
				return add(setting)
			}
			break
		case prot.MreqtRemove:
			if remove != nil {
				return remove(setting)
			}
			break
		case prot.MreqtUpdate:
			if update != nil {
				return update(setting)
			}
			break
		}

		return errors.Errorf("the RequestType \"%s\" is not supported", req)
	}

	var add modifyFunc
	var remove modifyFunc
	var update modifyFunc

	switch settings.ResourceType {
	case prot.MrtMappedVirtualDisk:
		add = func(setting interface{}) error {
			mvd := setting.(*prot.MappedVirtualDiskV2)
			scsiName, err := scsiControllerLunToName(h.osl, mvd.Controller, mvd.Lun)
			if err != nil {
				return errors.Wrapf(err, "failed to create MappedVirtualDiskV2")
			}
			ms := mountSpec{
				Source:     scsiName,
				FileSystem: defaultFileSystem,
				Flags:      uintptr(0),
			}
			if mvd.ReadOnly {
				ms.Flags |= syscall.MS_RDONLY
				ms.Options = append(ms.Options, mountOptionNoLoad)
			}
			if mvd.MountPath != "" {
				if err := h.osl.MkdirAll(mvd.MountPath, 0700); err != nil {
					return errors.Wrapf(err, "failed to create directory for MappedVirtualDiskV2 %s", mvd.MountPath)
				}
				if err := ms.MountWithTimedRetry(h.osl, mvd.MountPath); err != nil {
					return errors.Wrapf(err, "failed to mount directory for MappedVirtualDiskV2 %s", mvd.MountPath)
				}
			}
			return nil
		}
		remove = func(setting interface{}) error {
			mvd := setting.(*prot.MappedVirtualDiskV2)
			if mvd.MountPath != "" {
				if err := unmountPath(h.osl, mvd.MountPath, true); err != nil {
					return errors.Wrapf(err, "failed to hot remove MappedVirtualDiskV2 path: '%s'", mvd.MountPath)
				}
			}
			return h.osl.UnplugSCSIDisk(fmt.Sprintf("0:0:%d:%d", mvd.Controller, mvd.Lun))
		}
	case prot.MrtMappedDirectory:
		add = func(setting interface{}) error {
			md := setting.(*prot.MappedDirectoryV2)
			return mountPlan9Share(h.osl, h.vsock, md.MountPath, md.ShareName, md.Port, md.ReadOnly)
		}
		remove = func(setting interface{}) error {
			md := setting.(*prot.MappedDirectoryV2)
			return unmountPath(h.osl, md.MountPath, true)
		}
	case prot.MrtVPMemDevice:
		add = func(setting interface{}) error {
			vpd := setting.(*prot.MappedVPMemDeviceV2)
			ms := &mountSpec{
				Source:     "/dev/pmem" + strconv.FormatUint(uint64(vpd.DeviceNumber), 10),
				FileSystem: defaultFileSystem,
				Flags:      syscall.MS_RDONLY,
				Options:    []string{mountOptionNoLoad, mountOptionDax},
			}
			return mountLayer(h.osl, vpd.MountPath, ms)
		}
		remove = func(setting interface{}) error {
			vpd := setting.(*prot.MappedVPMemDeviceV2)
			return unmountPath(h.osl, vpd.MountPath, true)
		}
	case prot.MrtCombinedLayers:
		add = func(setting interface{}) error {
			cl := setting.(*prot.CombinedLayersV2)
			if cl.ContainerRootPath == "" {
				return errors.New("cannot combine layers with empty ContainerRootPath")
			}
			if err := h.osl.MkdirAll(cl.ContainerRootPath, 0700); err != nil {
				return errors.Wrapf(err, "failed to create ContainerRootPath directory '%s'", cl.ContainerRootPath)
			}

			layerPaths := make([]string, len(cl.Layers))
			for i, layer := range cl.Layers {
				layerPaths[i] = layer.Path
			}

			var upperdirPath string
			var workdirPath string
			var mountOptions uintptr
			if cl.ScratchPath == "" {
				// The user did not pass a scratch path. Mount overlay as readonly.
				mountOptions |= syscall.O_RDONLY
			} else {
				upperdirPath = filepath.Join(cl.ScratchPath, "upper")
				workdirPath = filepath.Join(cl.ScratchPath, "work")
			}

			return mountOverlay(h.osl, layerPaths, upperdirPath, workdirPath, cl.ContainerRootPath, mountOptions)
		}
		remove = func(setting interface{}) error {
			cl := setting.(*prot.CombinedLayersV2)
			return unmountPath(h.osl, cl.ContainerRootPath, true)
		}
	default:
		return errors.Errorf("the resource type \"%s\" is not supported", settings.ResourceType)
	}

	if err := requestTypeFn(settings.RequestType, settings.Settings, add, remove, update); err != nil {
		return errors.Wrapf(err, "Failed to modify ResourceType: \"%s\"", settings.ResourceType)
	}
	return nil
}

// Shutdown terminates this UVM. This is a destructive call and will destroy all
// state that has not been cleaned before calling this function.
func (h *Host) Shutdown() {
	h.osl.Shutdown()
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
				logrus.Debugf("*Process.Wait on PID: %d, failed with error: %v", p.pid, err)
				bgExitCodeChan <- -1
				return
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
