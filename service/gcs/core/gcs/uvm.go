package gcs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// UVMContainerID is the ContainerID that will be sent on any prot.MessageBase
// for V2 where the specific message is targeted at the UVM itself.
const UVMContainerID = "00000000-0000-0000-0000-000000000000"

// Host is the structure tracking all UVM host state including all containers
// and processes.
type Host struct {
	containersMutex sync.Mutex
	containers      map[string]*Container

	// Rtime is the Runtime interface used by the GCS core.
	rtime runtime.Runtime
	vsock transport.Transport

	// netNamespaces maps `NamespaceID` to the namespace opts
	netNamespaces map[string]*netNSOpts
	// networkNSToContainer is a map from `NamespaceID` to sandbox
	// `ContainerID`. If the map entry does not exist then the adapter is cached
	// in `netNamespaces` for addition when the sandbox is eventually created.
	networkNSToContainer sync.Map
}

func NewHost(rtime runtime.Runtime, vsock transport.Transport) *Host {
	return &Host{
		containers:    make(map[string]*Container),
		rtime:         rtime,
		vsock:         vsock,
		netNamespaces: make(map[string]*netNSOpts),
	}
}

func (h *Host) getContainerLocked(id string) (*Container, error) {
	if c, ok := h.containers[id]; !ok {
		return nil, errors.WithStack(gcserr.NewContainerDoesNotExistError(id))
	} else {
		return c, nil
	}
}

func (h *Host) GetAllProcessPids() []uint32 {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	pids := make([]uint32, 0)
	for _, c := range h.containers {
		c.processesMutex.Lock()
		for _, p := range c.processes {
			pids = append(pids, p.pid)
		}
		c.processesMutex.Unlock()
	}
	return pids
}

func (h *Host) GetContainer(id string) (*Container, error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	return h.getContainerLocked(id)
}

type netNSOpts struct {
	Adapters   []*prot.NetworkAdapterV2
	ResolvPath string
}

func (h *Host) CreateContainer(id string, settings *prot.VMHostedContainerSettingsV2) (*Container, error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	c, err := h.getContainerLocked(id)
	if err == nil {
		return c, nil
	}

	isSandboxOrStandalone := false
	if criType, ok := settings.OCISpecification.Annotations["io.kubernetes.cri.container-type"]; ok {
		// If the annotation is present it must be "sandbox"
		isSandboxOrStandalone = criType == "sandbox"
	} else {
		// No annotation declares a standalone LCOW
		isSandboxOrStandalone = true
	}

	// Check if we have a network namespace and if so generate the resolv.conf
	// so we can bindmount it into the container
	var networkNamespace string
	if settings.OCISpecification.Windows != nil &&
		settings.OCISpecification.Windows.Network != nil &&
		settings.OCISpecification.Windows.Network.NetworkNamespace != "" {
		networkNamespace = strings.ToLower(settings.OCISpecification.Windows.Network.NetworkNamespace)
	}
	if networkNamespace != "" {
		if isSandboxOrStandalone {
			// We have a network namespace. Generate the resolv.conf and add the bind mount.
			netopts := h.netNamespaces[networkNamespace]
			if netopts == nil || len(netopts.Adapters) == 0 {
				logrus.WithFields(logrus.Fields{
					"cid":   id,
					"netNS": networkNamespace,
				}).Warn("opengcs::CreateContainer - No adapters in namespace for sandbox")
			} else {
				logrus.WithFields(logrus.Fields{
					"cid":   id,
					"netNS": networkNamespace,
				}).Info("opengcs::CreateContainer - Generate sandbox resolv.conf")

				// TODO: Can we ever have more than one nic here at start?
				td, err := ioutil.TempDir("", "")
				if err != nil {
					return nil, errors.Wrap(err, "failed to create tmp directory")
				}
				resolvPath := filepath.Join(td, "resolv.conf")
				adp := netopts.Adapters[0]
				err = generateResolvConfFile(resolvPath, prot.NetworkAdapter{
					AdapterInstanceID: adp.ID,
					HostDNSServerList: adp.DNSServerList,
					HostDNSSuffix:     adp.DNSSuffix,
				})
				if err != nil {
					return nil, err
				}
				// Store the resolve path for all workload containers
				netopts.ResolvPath = resolvPath
			}
		}
		if netopts, ok := h.netNamespaces[networkNamespace]; ok && netopts.ResolvPath != "" {
			logrus.WithFields(logrus.Fields{
				"cid":        id,
				"netNS":      networkNamespace,
				"resolvPath": netopts.ResolvPath,
			}).Info("opengcs::CreateContainer - Adding /etc/resolv.conf mount")

			settings.OCISpecification.Mounts = append(settings.OCISpecification.Mounts, oci.Mount{
				Destination: "/etc/resolv.conf",
				Type:        "bind",
				Source:      netopts.ResolvPath,
				Options:     []string{"bind", "ro"},
			})
		}
	}

	// Clear the windows section of the config
	settings.OCISpecification.Windows = nil

	// Check if we need to do any capability/device mappings
	if settings.OCISpecification.Annotations["io.microsoft.virtualmachine.lcow.privileged"] == "true" {
		logrus.WithFields(logrus.Fields{
			"cid": id,
		}).Debugf("opengcs::CreateContainer - 'io.microsoft.virtualmachine.lcow.privileged' set for privileged container")

		// Add all host devices
		hostDevices, err := HostDevices()
		if err != nil {
			return nil, err
		}
		for _, hostDevice := range hostDevices {
			rd := oci.LinuxDevice{
				Path:  hostDevice.Path,
				Type:  string(hostDevice.Type),
				Major: hostDevice.Major,
				Minor: hostDevice.Minor,
				UID:   &hostDevice.Uid,
				GID:   &hostDevice.Gid,
			}
			if hostDevice.Major == 0 && hostDevice.Minor == 0 {
				// Invalid device, most likely a symbolic link, skip it.
				continue
			}
			found := false
			for i, dev := range settings.OCISpecification.Linux.Devices {
				if dev.Path == rd.Path {
					found = true
					settings.OCISpecification.Linux.Devices[i] = rd
					break
				}
				if dev.Type == rd.Type && dev.Major == rd.Major && dev.Minor == rd.Minor {
					logrus.WithFields(logrus.Fields{
						"cid": id,
					}).Warnf("opengcs::CreateContainer - The same type '%s', major '%d' and minor '%d', should not be used for multiple devices.", dev.Type, dev.Major, dev.Minor)
				}
			}
			if !found {
				settings.OCISpecification.Linux.Devices = append(settings.OCISpecification.Linux.Devices, rd)
			}
		}

		// Set the cgroup access
		settings.OCISpecification.Linux.Resources.Devices = []oci.LinuxDeviceCgroup{
			{
				Allow:  true,
				Access: "rwm",
			},
		}
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

	con, err := h.rtime.CreateContainer(id, settings.OCIBundlePath, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create container")
	}

	c = &Container{
		id:        id,
		vsock:     h.vsock,
		spec:      settings.OCISpecification,
		container: con,
		processes: make(map[uint32]*Process),
	}
	// Add the WG count for the init process
	c.processesWg.Add(1)
	c.initProcess = newProcess(c, settings.OCISpecification.Process, con.(runtime.Process), uint32(c.container.Pid()))

	// For the sandbox move all adapters into the network namespace
	if isSandboxOrStandalone && networkNamespace != "" {
		if netopts, ok := h.netNamespaces[networkNamespace]; ok {
			for _, a := range netopts.Adapters {
				err = c.AddNetworkAdapter(a)
				if err != nil {
					return nil, err
				}
			}
		}
		// Add a link for all HotAdd/Remove from NS id to this container.
		h.networkNSToContainer.Store(networkNamespace, id)
	}

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
			scsiName, err := scsiControllerLunToName(mvd.Controller, mvd.Lun)
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
				if err := os.MkdirAll(mvd.MountPath, 0700); err != nil {
					return errors.Wrapf(err, "failed to create directory for MappedVirtualDiskV2 %s", mvd.MountPath)
				}
				if err := ms.MountWithTimedRetry(mvd.MountPath); err != nil {
					return errors.Wrapf(err, "failed to mount directory for MappedVirtualDiskV2 %s", mvd.MountPath)
				}
			}
			return nil
		}
		remove = func(setting interface{}) error {
			mvd := setting.(*prot.MappedVirtualDiskV2)
			if mvd.MountPath != "" {
				if err := unmountPath(mvd.MountPath, true); err != nil {
					return errors.Wrapf(err, "failed to hot remove MappedVirtualDiskV2 path: '%s'", mvd.MountPath)
				}
			}
			return unplugSCSIDisk(fmt.Sprintf("0:0:%d:%d", mvd.Controller, mvd.Lun))
		}
	case prot.MrtMappedDirectory:
		add = func(setting interface{}) error {
			md := setting.(*prot.MappedDirectoryV2)
			return mountPlan9Share(h.vsock, md.MountPath, md.ShareName, md.Port, md.ReadOnly)
		}
		remove = func(setting interface{}) error {
			md := setting.(*prot.MappedDirectoryV2)
			return unmountPath(md.MountPath, true)
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
			return mountLayer(vpd.MountPath, ms)
		}
		remove = func(setting interface{}) error {
			vpd := setting.(*prot.MappedVPMemDeviceV2)
			return unmountPath(vpd.MountPath, true)
		}
	case prot.MrtCombinedLayers:
		add = func(setting interface{}) error {
			cl := setting.(*prot.CombinedLayersV2)
			if cl.ContainerRootPath == "" {
				return errors.New("cannot combine layers with empty ContainerRootPath")
			}
			if err := os.MkdirAll(cl.ContainerRootPath, 0700); err != nil {
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

			return mountOverlay(layerPaths, upperdirPath, workdirPath, cl.ContainerRootPath, mountOptions)
		}
		remove = func(setting interface{}) error {
			cl := setting.(*prot.CombinedLayersV2)
			return unmountPath(cl.ContainerRootPath, true)
		}
	case prot.MrtNetwork:
		add = func(setting interface{}) error {
			na := setting.(*prot.NetworkAdapterV2)
			na.ID = strings.ToLower(na.ID)
			na.NamespaceID = strings.ToLower(na.NamespaceID)
			if cidraw, ok := h.networkNSToContainer.Load(na.NamespaceID); ok {
				// The container has already been created. Get it and add this
				// adapter in real time.
				c, err := h.GetContainer(cidraw.(string))
				if err != nil {
					return err
				}
				return c.AddNetworkAdapter(na)
			}
			netopts, ok := h.netNamespaces[na.NamespaceID]
			if !ok {
				netopts = &netNSOpts{}
				h.netNamespaces[na.NamespaceID] = netopts
			}
			netopts.Adapters = append(netopts.Adapters, na)
			return nil
		}
		remove = func(setting interface{}) error {
			na := setting.(*prot.NetworkAdapterV2)
			na.ID = strings.ToLower(na.ID)
			na.NamespaceID = strings.ToLower(na.NamespaceID)
			if cidraw, ok := h.networkNSToContainer.Load(na.NamespaceID); ok {
				// The container was previously created or is still. Remove the
				// network. If the container is not found we just remove the
				// namespace reference.
				if c, err := h.GetContainer(cidraw.(string)); err == nil {
					return c.RemoveNetworkAdapter(na.ID)
				}
			} else {
				if netopts, ok := h.netNamespaces[na.NamespaceID]; ok {
					var i int
					var a *prot.NetworkAdapterV2
					for i, a = range netopts.Adapters {
						if na.ID == a.ID {
							break
						}
					}
					if a != nil {
						netopts.Adapters = append(netopts.Adapters[:i], netopts.Adapters[i+1:]...)
					}
				}
			}
			return nil
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
	syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
}

type Container struct {
	id    string
	vsock transport.Transport

	spec *oci.Spec

	container   runtime.Container
	initProcess *Process

	processesMutex sync.Mutex
	processesWg    sync.WaitGroup
	processes      map[uint32]*Process
}

func (c *Container) Start(conSettings stdio.ConnectionSettings) (int, error) {
	logrus.WithFields(logrus.Fields{
		"cid": c.id,
	}).Info("opengcs::Container::Start")

	stdioSet, err := stdio.Connect(c.vsock, conSettings)
	if err != nil {
		return -1, err
	}
	if c.initProcess.spec.Terminal {
		ttyr := c.container.Tty()
		ttyr.ReplaceConnectionSet(stdioSet)
		ttyr.Start()
	} else {
		pr := c.container.PipeRelay()
		pr.ReplaceConnectionSet(stdioSet)
		pr.CloseUnusedPipes()
		pr.Start()
	}
	err = c.container.Start()
	if err != nil {
		stdioSet.Close()
	}
	return int(c.initProcess.pid), err
}

func (c *Container) ExecProcess(process *oci.Process, conSettings stdio.ConnectionSettings) (int, error) {
	logrus.WithFields(logrus.Fields{
		"cid": c.id,
	}).Info("opengcs::Container::ExecProcess")

	stdioSet, err := stdio.Connect(c.vsock, conSettings)
	if err != nil {
		return -1, err
	}

	// Increment the waiters before the call so that WaitContainer cannot complete in a race
	// with adding a new process. When the process exits it will decrement this count.
	c.processesMutex.Lock()
	c.processesWg.Add(1)
	c.processesMutex.Unlock()

	p, err := c.container.ExecProcess(process, stdioSet)
	if err != nil {
		// We failed to exec any process. Remove our early count increment.
		c.processesMutex.Lock()
		c.processesWg.Done()
		c.processesMutex.Unlock()
		stdioSet.Close()
		return -1, err
	}

	pid := p.Pid()
	c.processesMutex.Lock()
	c.processes[uint32(pid)] = newProcess(c, process, p, uint32(pid))
	c.processesMutex.Unlock()
	return pid, nil
}

// GetProcess returns the *Process with the matching 'pid'. If the 'pid' does
// not exit returns error.
func (c *Container) GetProcess(pid uint32) (*Process, error) {
	logrus.WithFields(logrus.Fields{
		"cid": c.id,
		"pid": pid,
	}).Info("opengcs::Container::GetProcess")

	if c.initProcess.pid == pid {
		return c.initProcess, nil
	}

	c.processesMutex.Lock()
	defer c.processesMutex.Unlock()

	p, ok := c.processes[pid]
	if !ok {
		return nil, errors.WithStack(gcserr.NewProcessDoesNotExistError(int(pid)))
	}
	return p, nil
}

// Kill sends 'signal' to the container process.
func (c *Container) Kill(signal syscall.Signal) error {
	logrus.WithFields(logrus.Fields{
		"cid":    c.id,
		"signal": signal,
	}).Info("opengcs::Container::Kill")

	return c.container.Kill(signal)
}

// Wait waits for all processes exec'ed to finish as well as the init process
// representing the container.
func (c *Container) Wait() int {
	logrus.WithFields(logrus.Fields{
		"cid": c.id,
	}).Info("opengcs::Container::Wait")

	c.processesWg.Wait()
	return c.initProcess.exitCode
}

// AddNetworkAdapter adds `a` to the network namespace held by this container.
func (c *Container) AddNetworkAdapter(a *prot.NetworkAdapterV2) error {
	log := logrus.WithFields(logrus.Fields{
		"cid":               c.id,
		"adapterInstanceID": a.ID,
	})
	log.Info("opengcs::Container::AddNetworkAdapter")

	// TODO: netnscfg is not coded for v2 but since they are almost the same
	// just convert the parts of the adapter here.
	v1Adapter := &prot.NetworkAdapter{
		NatEnabled:         a.IPAddress != "",
		AllocatedIPAddress: a.IPAddress,
		HostIPAddress:      a.GatewayAddress,
		HostIPPrefixLength: a.PrefixLength,
		EnableLowMetric:    a.EnableLowMetric,
		EncapOverhead:      a.EncapOverhead,
	}

	cfg, err := json.Marshal(v1Adapter)
	if err != nil {
		return errors.Wrap(err, "failed to marshal adapter struct to JSON")
	}

	ifname, err := instanceIDToName(a.ID, true)
	if err != nil {
		return err
	}

	out, err := exec.Command("netnscfg",
		"-if", ifname,
		"-nspid", strconv.Itoa(c.container.Pid()),
		"-cfg", string(cfg)).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to configure adapter cid: %s, aid: %s, if id: %s %s", c.id, a.ID, ifname, string(out))
	}
	return nil
}

// RemoveNetworkAdapter removes the network adapter `id` from the network
// namespace held by this container.
func (c *Container) RemoveNetworkAdapter(id string) error {
	logrus.WithFields(logrus.Fields{
		"cid":               c.id,
		"adapterInstanceID": id,
	}).Info("opengcs::Container::RemoveNetworkAdapter")

	// TODO: JTERRY75 - Implement removal if we ever need to support hot remove.
	return errors.New("not implemented")
}

// Process is a struct that defines the lifetime and operations associated with
// an oci.Process.
type Process struct {
	spec *oci.Process
	// cid is the container id that owns this process.
	cid string

	process runtime.Process
	pid     uint32
	// This is only valid post the exitWg
	exitCode int
	exitWg   sync.WaitGroup

	// Used to allow addtion/removal to the writersWg after an initial wait has
	// already been issued. It is not safe to call Add/Done without holding this
	// lock.
	writersSyncRoot sync.Mutex
	// Used to track the number of writers that need to finish
	// before the process can be marked for cleanup.
	writersWg sync.WaitGroup
	// Used to track the 1st caller to the writersWg that successfully
	// acknowledges it wrote the exit response.
	writersCalled bool
}

// newProcess returns a Process struct that has been initialized with an
// outstanding wait for process exit, and post exit an outstanding wait for
// process cleanup to release all resources once at least 1 waiter has
// successfully written the exit response.
func newProcess(c *Container, spec *oci.Process, process runtime.Process, pid uint32) *Process {
	p := &Process{
		spec:    spec,
		process: process,
		cid:     c.id,
		pid:     pid,
	}
	p.exitWg.Add(1)
	p.writersWg.Add(1)
	go func() {
		// Wait for the process to exit
		exitCode, err := p.process.Wait()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"cid":           c.id,
				"pid":           pid,
				logrus.ErrorKey: err,
			}).Error("opengcs::Process - failed to wait for runc process")
		}
		p.exitCode = exitCode
		logrus.WithFields(logrus.Fields{
			"cid":      c.id,
			"pid":      pid,
			"exitCode": p.exitCode,
		}).Info("opengcs::Process - process exited")

		// Free any process waiters
		p.exitWg.Done()
		// Decrement any container process count waiters
		c.processesMutex.Lock()
		c.processesWg.Done()
		c.processesMutex.Unlock()

		// Schedule the removal of this process object from the map once at
		// least one waiter has read the result
		go func() {
			p.writersWg.Wait()
			c.processesMutex.Lock()

			logrus.WithFields(logrus.Fields{
				"cid": c.id,
				"pid": pid,
			}).Debug("opengcs::Process - all waiters have completed, removing process")

			delete(c.processes, p.pid)
			c.processesMutex.Unlock()
		}()
	}()
	return p
}

// Kill sends 'signal' to the process.
func (p *Process) Kill(signal syscall.Signal) error {
	logrus.WithFields(logrus.Fields{
		"cid":    p.cid,
		"pid":    p.pid,
		"signal": signal,
	}).Info("opengcs::Process::Kill")

	if err := syscall.Kill(int(p.pid), signal); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ResizeConsole resizes the tty to `height`x`width` for the process.
func (p *Process) ResizeConsole(height, width uint16) error {
	logrus.WithFields(logrus.Fields{
		"cid":    p.cid,
		"pid":    p.pid,
		"height": height,
		"width":  width,
	}).Info("opengcs::Process::ResizeConsole")

	tty := p.process.Tty()
	if tty == nil {
		return fmt.Errorf("pid: %d, is not a tty and cannot be resized", p.pid)
	}
	return tty.ResizeConsole(height, width)
}

// Wait returns a channel that can be used to wait for the process to exit and
// gather the exit code. The second channel must be signaled from the caller
// when the caller has completed its use of this call to Wait.
func (p *Process) Wait() (<-chan int, chan<- bool) {
	log := logrus.WithFields(logrus.Fields{
		"cid": p.cid,
		"pid": p.pid,
	})
	log.Info("opengcs::Process::Wait")

	exitCodeChan := make(chan int, 1)
	doneChan := make(chan bool)

	// Increment our waiters for this waiter
	p.writersSyncRoot.Lock()
	p.writersWg.Add(1)
	p.writersSyncRoot.Unlock()

	go func() {
		bgExitCodeChan := make(chan int, 1)
		go func() {
			p.exitWg.Wait()
			bgExitCodeChan <- p.exitCode
		}()

		// Wait for the exit code or the caller to stop waiting.
		select {
		case exitCode := <-bgExitCodeChan:
			exitCodeChan <- exitCode

			// The caller got the exit code. Wait for them to tell us they have
			// issued the write
			select {
			case <-doneChan:
				p.writersSyncRoot.Lock()
				// Decrement this waiter
				log.Debug("opengcs::Process::Wait - wait completed, releasing wait count")

				p.writersWg.Done()
				if !p.writersCalled {
					// We have at least 1 response for the exit code for this
					// process. Decrement the release waiter that will free the
					// process resources when the writersWg hits 0
					log.Debug("opengcs::Process::Wait - first wait completed, releasing first wait count")

					p.writersCalled = true
					p.writersWg.Done()
				}
				p.writersSyncRoot.Unlock()
			}

		case <-doneChan:
			// In this case the caller timed out before the process exited. Just
			// decrement the waiter but since no exit code we just deal with our
			// waiter.
			p.writersSyncRoot.Lock()
			log.Debug("opengcs::Process::Wait - wait canceled before exit, releasing wait count")

			p.writersWg.Done()
			p.writersSyncRoot.Unlock()
		}
	}()
	return exitCodeChan, doneChan
}
