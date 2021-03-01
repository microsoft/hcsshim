// +build linux

package hcsv2

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Microsoft/opengcs/internal/storage"
	"github.com/Microsoft/opengcs/internal/storage/overlay"
	"github.com/Microsoft/opengcs/internal/storage/pci"
	"github.com/Microsoft/opengcs/internal/storage/plan9"
	"github.com/Microsoft/opengcs/internal/storage/pmem"
	"github.com/Microsoft/opengcs/internal/storage/scsi"
	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/pkg/errors"
)

// UVMContainerID is the ContainerID that will be sent on any prot.MessageBase
// for V2 where the specific message is targeted at the UVM itself.
const UVMContainerID = "00000000-0000-0000-0000-000000000000"

// Host is the structure tracking all UVM host state including all containers
// and processes.
type Host struct {
	containersMutex sync.Mutex
	containers      map[string]*Container

	externalProcessesMutex sync.Mutex
	externalProcesses      map[int]*externalProcess

	// Rtime is the Runtime interface used by the GCS core.
	rtime runtime.Runtime
	vsock transport.Transport
}

func NewHost(rtime runtime.Runtime, vsock transport.Transport) *Host {
	return &Host{
		containers:        make(map[string]*Container),
		externalProcesses: make(map[int]*externalProcess),
		rtime:             rtime,
		vsock:             vsock,
	}
}

func (h *Host) RemoveContainer(id string) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	delete(h.containers, id)
}

func (h *Host) getContainerLocked(id string) (*Container, error) {
	if c, ok := h.containers[id]; !ok {
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	} else {
		return c, nil
	}
}

func (h *Host) GetContainer(id string) (*Container, error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	return h.getContainerLocked(id)
}

func setupSandboxMountsPath(id string) error {
	mountPath := getSandboxMountsDir(id)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create sandboxMounts dir in sandbox %v", id)
	}

	return storage.MountRShared(mountPath)
}

func (h *Host) CreateContainer(ctx context.Context, id string, settings *prot.VMHostedContainerSettingsV2) (_ *Container, err error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	if _, ok := h.containers[id]; ok {
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemAlreadyExists)
	}

	var namespaceID string
	criType, isCRI := settings.OCISpecification.Annotations["io.kubernetes.cri.container-type"]
	if isCRI {
		switch criType {
		case "sandbox":
			// Capture namespaceID if any because setupSandboxContainerSpec clears the Windows section.
			namespaceID = getNetworkNamespaceID(settings.OCISpecification)
			err = setupSandboxContainerSpec(ctx, id, settings.OCISpecification)
			defer func() {
				if err != nil {
					defer os.RemoveAll(getSandboxRootDir(id))
				}
			}()
			err = setupSandboxMountsPath(id)
		case "container":
			sid, ok := settings.OCISpecification.Annotations["io.kubernetes.cri.sandbox-id"]
			if !ok || sid == "" {
				return nil, errors.Errorf("unsupported 'io.kubernetes.cri.sandbox-id': '%s'", sid)
			}
			err = setupWorkloadContainerSpec(ctx, sid, id, settings.OCISpecification)
			defer func() {
				if err != nil {
					defer os.RemoveAll(getWorkloadRootDir(id))
				}
			}()
		default:
			err = errors.Errorf("unsupported 'io.kubernetes.cri.container-type': '%s'", criType)
		}
	} else {
		// Capture namespaceID if any because setupStandaloneContainerSpec clears the Windows section.
		namespaceID = getNetworkNamespaceID(settings.OCISpecification)
		err = setupStandaloneContainerSpec(ctx, id, settings.OCISpecification)
		defer func() {
			if err != nil {
				os.RemoveAll(getStandaloneRootDir(id))
			}
		}()
	}
	if err != nil {
		return nil, err
	}

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

	c := &Container{
		id:        id,
		vsock:     h.vsock,
		spec:      settings.OCISpecification,
		isSandbox: criType == "sandbox",
		container: con,
		exitType:  prot.NtUnexpectedExit,
		processes: make(map[uint32]*containerProcess),
	}
	c.initProcess = newProcess(c, settings.OCISpecification.Process, con.(runtime.Process), uint32(c.container.Pid()), true)

	// Sandbox or standalone, move the networks to the container namespace
	if criType == "sandbox" || !isCRI {
		ns, err := getNetworkNamespace(namespaceID)
		if isCRI && err != nil {
			return nil, err
		}
		// standalone is not required to have a networking namespace setup
		if ns != nil {
			if err := ns.AssignContainerPid(ctx, c.container.Pid()); err != nil {
				return nil, err
			}
			if err := ns.Sync(ctx); err != nil {
				return nil, err
			}
		}
	}

	h.containers[id] = c
	return c, nil
}

func (h *Host) modifyHostSettings(ctx context.Context, containerID string, settings *prot.ModifySettingRequest) error {
	switch settings.ResourceType {
	case prot.MrtMappedVirtualDisk:
		return modifyMappedVirtualDisk(ctx, settings.RequestType, settings.Settings.(*prot.MappedVirtualDiskV2))
	case prot.MrtMappedDirectory:
		return modifyMappedDirectory(ctx, h.vsock, settings.RequestType, settings.Settings.(*prot.MappedDirectoryV2))
	case prot.MrtVPMemDevice:
		return modifyMappedVPMemDevice(ctx, settings.RequestType, settings.Settings.(*prot.MappedVPMemDeviceV2))
	case prot.MrtCombinedLayers:
		return modifyCombinedLayers(ctx, settings.RequestType, settings.Settings.(*prot.CombinedLayersV2))
	case prot.MrtNetwork:
		return modifyNetwork(ctx, settings.RequestType, settings.Settings.(*prot.NetworkAdapterV2))
	case prot.MrtVPCIDevice:
		return modifyMappedVPCIDevice(ctx, settings.RequestType, settings.Settings.(*prot.MappedVPCIDeviceV2))
	case prot.MrtContainerConstraints:
		c, err := h.GetContainer(containerID)
		if err != nil {
			return err
		}
		return c.modifyContainerConstraints(ctx, settings.RequestType, settings.Settings.(*prot.ContainerConstraintsV2))
	default:
		return errors.Errorf("the ResourceType \"%s\" is not supported for UVM", settings.ResourceType)
	}
}

func (h *Host) modifyContainerSettings(ctx context.Context, containerID string, settings *prot.ModifySettingRequest) error {
	c, err := h.GetContainer(containerID)
	if err != nil {
		return err
	}

	switch settings.ResourceType {
	case prot.MrtContainerConstraints:
		return c.modifyContainerConstraints(ctx, settings.RequestType, settings.Settings.(*prot.ContainerConstraintsV2))
	default:
		return errors.Errorf("the ResourceType \"%s\" is not supported for containers", settings.ResourceType)
	}
}

func (h *Host) ModifySettings(ctx context.Context, containerID string, settings *prot.ModifySettingRequest) error {
	if containerID == UVMContainerID {
		return h.modifyHostSettings(ctx, containerID, settings)
	}
	return h.modifyContainerSettings(ctx, containerID, settings)
}

// Shutdown terminates this UVM. This is a destructive call and will destroy all
// state that has not been cleaned before calling this function.
func (h *Host) Shutdown() {
	syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
}

// RunExternalProcess runs a process in the utility VM.
func (h *Host) RunExternalProcess(ctx context.Context, params prot.ProcessParameters, conSettings stdio.ConnectionSettings) (_ int, err error) {
	var stdioSet *stdio.ConnectionSet
	stdioSet, err = stdio.Connect(h.vsock, conSettings)
	if err != nil {
		return -1, err
	}
	defer func() {
		if err != nil {
			stdioSet.Close()
		}
	}()

	args := params.CommandArgs
	if len(args) == 0 {
		args, err = processParamCommandLineToOCIArgs(params.CommandLine)
		if err != nil {
			return -1, err
		}
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = params.WorkingDirectory
	cmd.Env = processParamEnvToOCIEnv(params.Environment)

	var relay *stdio.TtyRelay
	if params.EmulateConsole {
		// Allocate a console for the process.
		var (
			master      *os.File
			consolePath string
		)
		master, consolePath, err = stdio.NewConsole()
		if err != nil {
			return -1, errors.Wrap(err, "failed to create console for external process")
		}
		defer func() {
			if err != nil {
				master.Close()
			}
		}()

		var console *os.File
		console, err = os.OpenFile(consolePath, os.O_RDWR|syscall.O_NOCTTY, 0777)
		if err != nil {
			return -1, errors.Wrap(err, "failed to open console file for external process")
		}
		defer console.Close()

		relay = stdio.NewTtyRelay(stdioSet, master)
		cmd.Stdin = console
		cmd.Stdout = console
		cmd.Stderr = console
		// Make the child process a session leader and adopt the pty as
		// the controlling terminal.
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid:  true,
			Setctty: true,
			Ctty:    syscall.Stdin,
		}
	} else {
		var fileSet *stdio.FileSet
		fileSet, err = stdioSet.Files()
		if err != nil {
			return -1, errors.Wrap(err, "failed to set cmd stdio")
		}
		defer fileSet.Close()
		defer stdioSet.Close()
		cmd.Stdin = fileSet.In
		cmd.Stdout = fileSet.Out
		cmd.Stderr = fileSet.Err
	}

	onRemove := func(pid int) {
		h.externalProcessesMutex.Lock()
		delete(h.externalProcesses, pid)
		h.externalProcessesMutex.Unlock()
	}
	p, err := newExternalProcess(ctx, cmd, relay, onRemove)
	if err != nil {
		return -1, err
	}

	h.externalProcessesMutex.Lock()
	h.externalProcesses[p.Pid()] = p
	h.externalProcessesMutex.Unlock()
	return p.Pid(), nil
}

func (h *Host) GetExternalProcess(pid int) (Process, error) {
	h.externalProcessesMutex.Lock()
	defer h.externalProcessesMutex.Unlock()

	p, ok := h.externalProcesses[pid]
	if !ok {
		return nil, gcserr.NewHresultError(gcserr.HrErrNotFound)
	}
	return p, nil
}

func newInvalidRequestTypeError(rt prot.ModifyRequestType) error {
	return errors.Errorf("the RequestType \"%s\" is not supported", rt)
}

func modifyMappedVirtualDisk(ctx context.Context, rt prot.ModifyRequestType, mvd *prot.MappedVirtualDiskV2) (err error) {
	switch rt {
	case prot.MreqtAdd:
		mountCtx, cancel := context.WithTimeout(ctx, time.Second*4)
		defer cancel()
		if mvd.MountPath != "" {
			return scsi.Mount(mountCtx, mvd.Controller, mvd.Lun, mvd.MountPath, mvd.ReadOnly)
		}
		return nil
	case prot.MreqtRemove:
		if mvd.MountPath != "" {
			if err := storage.UnmountPath(ctx, mvd.MountPath, true); err != nil {
				return err
			}
		}
		return scsi.UnplugDevice(ctx, mvd.Controller, mvd.Lun)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyMappedDirectory(ctx context.Context, vsock transport.Transport, rt prot.ModifyRequestType, md *prot.MappedDirectoryV2) (err error) {
	switch rt {
	case prot.MreqtAdd:
		return plan9.Mount(ctx, vsock, md.MountPath, md.ShareName, md.Port, md.ReadOnly)
	case prot.MreqtRemove:
		return storage.UnmountPath(ctx, md.MountPath, true)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyMappedVPMemDevice(ctx context.Context, rt prot.ModifyRequestType, vpd *prot.MappedVPMemDeviceV2) (err error) {
	switch rt {
	case prot.MreqtAdd:
		if vpd.MappingInfo != nil {
			return pmem.MountDM(ctx, vpd.DeviceNumber, vpd.MappingInfo.DeviceOffsetInBytes, vpd.MappingInfo.DeviceSizeInBytes, vpd.MountPath)
		}
		return pmem.Mount(ctx, vpd.DeviceNumber, vpd.MountPath)
	case prot.MreqtRemove:
		if vpd.MappingInfo != nil {
			return pmem.UnmountDM(ctx, vpd.DeviceNumber, vpd.MappingInfo.DeviceOffsetInBytes, vpd.MappingInfo.DeviceSizeInBytes, vpd.MountPath)
		}
		return storage.UnmountPath(ctx, vpd.MountPath, true)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyMappedVPCIDevice(ctx context.Context, rt prot.ModifyRequestType, vpciDev *prot.MappedVPCIDeviceV2) error {
	switch rt {
	case prot.MreqtAdd:
		return pci.WaitForPCIDeviceFromVMBusGUID(ctx, vpciDev.VMBusGUID)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyCombinedLayers(ctx context.Context, rt prot.ModifyRequestType, cl *prot.CombinedLayersV2) (err error) {
	switch rt {
	case prot.MreqtAdd:
		layerPaths := make([]string, len(cl.Layers))
		for i, layer := range cl.Layers {
			layerPaths[i] = layer.Path
		}

		var upperdirPath string
		var workdirPath string
		readonly := false
		if cl.ScratchPath == "" {
			// The user did not pass a scratch path. Mount overlay as readonly.
			readonly = true
		} else {
			upperdirPath = filepath.Join(cl.ScratchPath, "upper")
			workdirPath = filepath.Join(cl.ScratchPath, "work")
		}

		return overlay.Mount(ctx, layerPaths, upperdirPath, workdirPath, cl.ContainerRootPath, readonly)
	case prot.MreqtRemove:
		return storage.UnmountPath(ctx, cl.ContainerRootPath, true)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyNetwork(ctx context.Context, rt prot.ModifyRequestType, na *prot.NetworkAdapterV2) (err error) {
	switch rt {
	case prot.MreqtAdd:
		ns := getOrAddNetworkNamespace(na.NamespaceID)
		if err := ns.AddAdapter(ctx, na); err != nil {
			return err
		}
		// This code doesnt know if the namespace was already added to the
		// container or not so it must always call `Sync`.
		return ns.Sync(ctx)
	case prot.MreqtRemove:
		ns := getOrAddNetworkNamespace(na.ID)
		if err := ns.RemoveAdapter(ctx, na.ID); err != nil {
			return err
		}
		return nil
	default:
		return newInvalidRequestTypeError(rt)
	}
}

// processParamCommandLineToOCIArgs converts a CommandLine field from
// ProcessParameters (a space separate argument string) into an array of string
// arguments which can be used by an oci.Process.
func processParamCommandLineToOCIArgs(commandLine string) ([]string, error) {
	args, err := shellwords.Parse(commandLine)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse command line string \"%s\"", commandLine)
	}
	return args, nil
}

// processParamEnvToOCIEnv converts an Environment field from ProcessParameters
// (a map from environment variable to value) into an array of environment
// variable assignments (where each is in the form "<variable>=<value>") which
// can be used by an oci.Process.
func processParamEnvToOCIEnv(environment map[string]string) []string {
	environmentList := make([]string, 0, len(environment))
	for k, v := range environment {
		// TODO: Do we need to escape things like quotation marks in
		// environment variable values?
		environmentList = append(environmentList, fmt.Sprintf("%s=%s", k, v))
	}
	return environmentList
}
