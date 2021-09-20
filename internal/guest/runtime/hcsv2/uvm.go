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

	"github.com/Microsoft/hcsshim/internal/guest/gcserr"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/guest/storage"
	"github.com/Microsoft/hcsshim/internal/guest/storage/overlay"
	"github.com/Microsoft/hcsshim/internal/guest/storage/pci"
	"github.com/Microsoft/hcsshim/internal/guest/storage/plan9"
	"github.com/Microsoft/hcsshim/internal/guest/storage/pmem"
	"github.com/Microsoft/hcsshim/internal/guest/storage/scsi"
	"github.com/Microsoft/hcsshim/internal/guest/transport"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
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

	// state required for the security policy enforcement
	policyMutex               sync.Mutex
	securityPolicyEnforcer    securitypolicy.SecurityPolicyEnforcer
	securityPolicyEnforcerSet bool
}

func NewHost(rtime runtime.Runtime, vsock transport.Transport) *Host {
	return &Host{
		containers:                make(map[string]*Container),
		externalProcesses:         make(map[int]*externalProcess),
		rtime:                     rtime,
		vsock:                     vsock,
		securityPolicyEnforcerSet: false,
		securityPolicyEnforcer:    &securitypolicy.OpenDoorSecurityPolicyEnforcer{},
	}
}

// SetSecurityPolicy takes a base64 encoded security policy
// and sets up our internal data structures we use to store
// said policy.
// The security policy is transmitted as json in an annotation,
// so we first have to remove the base64 encoding that allows
// the JSON based policy to be passed as a string. From there,
// we decode the JSON and setup our security policy state
func (h *Host) SetSecurityPolicy(base64Policy string) error {
	h.policyMutex.Lock()
	defer h.policyMutex.Unlock()
	if h.securityPolicyEnforcerSet {
		return errors.New("security policy has already been set")
	}

	// construct security policy state
	securityPolicyState, err := securitypolicy.NewSecurityPolicyState(base64Policy)
	if err != nil {
		return err
	}

	p, err := securitypolicy.NewSecurityPolicyEnforcer(*securityPolicyState)
	if err != nil {
		return err
	}

	h.securityPolicyEnforcer = p
	h.securityPolicyEnforcerSet = true

	return nil
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

func setupSandboxMountsPath(id string) (err error) {
	mountPath := getSandboxMountsDir(id)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create sandboxMounts dir in sandbox %v", id)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(mountPath)
		}
	}()

	return storage.MountRShared(mountPath)
}

func setupSandboxHugePageMountsPath(id string) error {
	mountPath := getSandboxHugePageMountsDir(id)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create hugepage Mounts dir in sandbox %v", id)
	}

	return storage.MountRShared(mountPath)
}

func (h *Host) CreateContainer(ctx context.Context, id string, settings *prot.VMHostedContainerSettingsV2) (_ *Container, err error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	if _, ok := h.containers[id]; ok {
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemAlreadyExists)
	}

	err = h.securityPolicyEnforcer.EnforceCreateContainerPolicy(id, settings.OCISpecification.Process.Args, settings.OCISpecification.Process.Env)

	if err != nil {
		return nil, errors.Wrapf(err, "container creation denied due to policy")
	}

	var namespaceID string
	criType, isCRI := settings.OCISpecification.Annotations["io.kubernetes.cri.container-type"]
	if isCRI {
		switch criType {
		case "sandbox":
			// Capture namespaceID if any because setupSandboxContainerSpec clears the Windows section.
			namespaceID = getNetworkNamespaceID(settings.OCISpecification)
			err = setupSandboxContainerSpec(ctx, id, settings.OCISpecification)
			if err != nil {
				return nil, err
			}
			defer func() {
				if err != nil {
					_ = os.RemoveAll(getSandboxRootDir(id))
				}
			}()

			if err = setupSandboxMountsPath(id); err != nil {
				return nil, err
			}

			if err = setupSandboxHugePageMountsPath(id); err != nil {
				return nil, err
			}
		case "container":
			sid, ok := settings.OCISpecification.Annotations["io.kubernetes.cri.sandbox-id"]
			if !ok || sid == "" {
				return nil, errors.Errorf("unsupported 'io.kubernetes.cri.sandbox-id': '%s'", sid)
			}
			if err := setupWorkloadContainerSpec(ctx, sid, id, settings.OCISpecification); err != nil {
				return nil, err
			}
			defer func() {
				if err != nil {
					_ = os.RemoveAll(getWorkloadRootDir(id))
				}
			}()
		default:
			return nil, errors.Errorf("unsupported 'io.kubernetes.cri.container-type': '%s'", criType)
		}
	} else {
		// Capture namespaceID if any because setupStandaloneContainerSpec clears the Windows section.
		namespaceID = getNetworkNamespaceID(settings.OCISpecification)
		if err := setupStandaloneContainerSpec(ctx, id, settings.OCISpecification); err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				_ = os.RemoveAll(getStandaloneRootDir(id))
			}
		}()
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
		return modifyMappedVirtualDisk(ctx, settings.RequestType, settings.Settings.(*prot.MappedVirtualDiskV2), h.securityPolicyEnforcer)
	case prot.MrtMappedDirectory:
		return modifyMappedDirectory(ctx, h.vsock, settings.RequestType, settings.Settings.(*prot.MappedDirectoryV2))
	case prot.MrtVPMemDevice:
		return modifyMappedVPMemDevice(ctx, settings.RequestType, settings.Settings.(*prot.MappedVPMemDeviceV2), h.securityPolicyEnforcer)
	case prot.MrtCombinedLayers:
		return modifyCombinedLayers(ctx, settings.RequestType, settings.Settings.(*prot.CombinedLayersV2), h.securityPolicyEnforcer)
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
	case prot.MrtSecurityPolicy:
		policy, ok := settings.Settings.(*securitypolicy.EncodedSecurityPolicy)
		if !ok {
			return errors.New("the request's settings are not of type EncodedSecurityPolicy")
		}

		return h.SetSecurityPolicy(policy.SecurityPolicy)
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

func modifyMappedVirtualDisk(ctx context.Context, rt prot.ModifyRequestType, mvd *prot.MappedVirtualDiskV2, securityPolicy securitypolicy.SecurityPolicyEnforcer) (err error) {
	switch rt {
	case prot.MreqtAdd:
		mountCtx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if mvd.MountPath != "" {
			return scsi.Mount(mountCtx, mvd.Controller, mvd.Lun, mvd.MountPath, mvd.ReadOnly, mvd.Encrypted, mvd.Options, mvd.VerityInfo, securityPolicy)
		}
		return nil
	case prot.MreqtRemove:
		if mvd.MountPath != "" {
			if err := scsi.Unmount(ctx, mvd.Controller, mvd.Lun, mvd.MountPath, mvd.Encrypted, mvd.VerityInfo, securityPolicy); err != nil {
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

func modifyMappedVPMemDevice(ctx context.Context, rt prot.ModifyRequestType, vpd *prot.MappedVPMemDeviceV2, securityPolicy securitypolicy.SecurityPolicyEnforcer) (err error) {
	switch rt {
	case prot.MreqtAdd:
		return pmem.Mount(ctx, vpd.DeviceNumber, vpd.MountPath, vpd.MappingInfo, vpd.VerityInfo, securityPolicy)
	case prot.MreqtRemove:
		return pmem.Unmount(ctx, vpd.DeviceNumber, vpd.MountPath, vpd.MappingInfo, vpd.VerityInfo, securityPolicy)
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

func modifyCombinedLayers(ctx context.Context, rt prot.ModifyRequestType, cl *prot.CombinedLayersV2, securityPolicy securitypolicy.SecurityPolicyEnforcer) (err error) {
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

		return overlay.Mount(ctx, layerPaths, upperdirPath, workdirPath, cl.ContainerRootPath, readonly, cl.ContainerId, securityPolicy)
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
