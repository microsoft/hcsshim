//go:build linux
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
	"github.com/Microsoft/hcsshim/internal/guest/policy"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	"github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/guest/storage"
	"github.com/Microsoft/hcsshim/internal/guest/storage/overlay"
	"github.com/Microsoft/hcsshim/internal/guest/storage/pci"
	"github.com/Microsoft/hcsshim/internal/guest/storage/plan9"
	"github.com/Microsoft/hcsshim/internal/guest/storage/pmem"
	"github.com/Microsoft/hcsshim/internal/guest/storage/scsi"
	"github.com/Microsoft/hcsshim/internal/guest/transport"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/mattn/go-shellwords"
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
		securityPolicyEnforcer:    &securitypolicy.ClosedDoorSecurityPolicyEnforcer{},
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

	hostData, err := securitypolicy.NewSecurityPolicyDigest(base64Policy)
	if err != nil {
		return err
	}

	if err := validateHostData(hostData[:]); err != nil {
		return err
	}

	if err := p.ExtendDefaultMounts(policy.DefaultCRIMounts()); err != nil {
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

func (h *Host) GetCreatedContainer(id string) (*Container, error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	if c, ok := h.containers[id]; !ok {
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	} else {
		if c.getStatus() != containerCreated {
			return nil, fmt.Errorf("container is not in state \"created\": %w",
				gcserr.NewHresultError(gcserr.HrVmcomputeInvalidState))
		}
		return c, nil
	}
}

func (h *Host) AddContainer(id string, c *Container) error {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	if _, ok := h.containers[id]; ok {
		return gcserr.NewHresultError(gcserr.HrVmcomputeSystemAlreadyExists)
	}
	h.containers[id] = c
	return nil
}

func setupSandboxMountsPath(id string) (err error) {
	mountPath := spec.SandboxMountsDir(id)
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
	mountPath := spec.HugePagesMountsDir(id)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create hugepage Mounts dir in sandbox %v", id)
	}

	return storage.MountRShared(mountPath)
}

func (h *Host) CreateContainer(ctx context.Context, id string, settings *prot.VMHostedContainerSettingsV2) (_ *Container, err error) {
	criType, isCRI := settings.OCISpecification.Annotations[annotations.KubernetesContainerType]
	c := &Container{
		id:        id,
		vsock:     h.vsock,
		spec:      settings.OCISpecification,
		isSandbox: criType == "sandbox",
		exitType:  prot.NtUnexpectedExit,
		processes: make(map[uint32]*containerProcess),
		status:    containerCreating,
	}

	if err := h.AddContainer(id, c); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			h.RemoveContainer(id)
		}
	}()

	err = h.securityPolicyEnforcer.EnforceCreateContainerPolicy(
		id,
		settings.OCISpecification.Process.Args,
		settings.OCISpecification.Process.Env,
		settings.OCISpecification.Process.Cwd,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "container creation denied due to policy")
	}

	var namespaceID string
	// for sandbox container sandboxID is same as container id
	sandboxID := id
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
					_ = os.RemoveAll(spec.SandboxRootDir(id))
				}
			}()

			if err = setupSandboxMountsPath(id); err != nil {
				return nil, err
			}

			if err = setupSandboxHugePageMountsPath(id); err != nil {
				return nil, err
			}

			if err := policy.ExtendPolicyWithNetworkingMounts(id, h.securityPolicyEnforcer, settings.OCISpecification); err != nil {
				return nil, err
			}
		case "container":
			sid, ok := settings.OCISpecification.Annotations[annotations.KubernetesSandboxID]
			sandboxID = sid
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
			if err := policy.ExtendPolicyWithNetworkingMounts(sandboxID, h.securityPolicyEnforcer, settings.OCISpecification); err != nil {
				return nil, err
			}
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

	if err := h.securityPolicyEnforcer.EnforceMountPolicy(sandboxID, id, settings.OCISpecification); err != nil {
		return nil, err
	}
	// Export security policy as one of the process's environment variables so that application and sidecar
	// containers can have access to it. The security policy is required by containers which need to extract
	// init-time claims found in the security policy.
	//
	// We append the variable after the security policy enforcing logic completes so as to bypass it; the
	// security policy variable cannot be included in the security policy as its value is not available
	// security policy construction time.
	if policyEnforcer, ok := (h.securityPolicyEnforcer).(*securitypolicy.StandardSecurityPolicyEnforcer); ok {
		secPolicyEnv := fmt.Sprintf("SECURITY_POLICY=%s", policyEnforcer.EncodedSecurityPolicy)
		settings.OCISpecification.Process.Env = append(settings.OCISpecification.Process.Env, secPolicyEnv)
	}

	// Sandbox mount paths need to be resolved in the spec before expected mounts policy can be enforced.
	if err = h.securityPolicyEnforcer.EnforceExpectedMountsPolicy(id, settings.OCISpecification); err != nil {
		return nil, errors.Wrapf(err, "container creation denied due to policy")
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
	init, err := con.GetInitProcess()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get container init process")
	}

	c.container = con
	c.initProcess = newProcess(c, settings.OCISpecification.Process, init, uint32(c.container.Pid()), true)

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

	c.setStatus(containerCreated)
	return c, nil
}

func (h *Host) modifyHostSettings(ctx context.Context, containerID string, req *guestrequest.ModificationRequest) error {
	switch req.ResourceType {
	case guestresource.ResourceTypeMappedVirtualDisk:
		return modifyMappedVirtualDisk(ctx, req.RequestType, req.Settings.(*guestresource.LCOWMappedVirtualDisk), h.securityPolicyEnforcer)
	case guestresource.ResourceTypeMappedDirectory:
		return modifyMappedDirectory(ctx, h.vsock, req.RequestType, req.Settings.(*guestresource.LCOWMappedDirectory))
	case guestresource.ResourceTypeVPMemDevice:
		return modifyMappedVPMemDevice(ctx, req.RequestType, req.Settings.(*guestresource.LCOWMappedVPMemDevice), h.securityPolicyEnforcer)
	case guestresource.ResourceTypeCombinedLayers:
		return modifyCombinedLayers(ctx, req.RequestType, req.Settings.(*guestresource.LCOWCombinedLayers), h.securityPolicyEnforcer)
	case guestresource.ResourceTypeNetwork:
		return modifyNetwork(ctx, req.RequestType, req.Settings.(*guestresource.LCOWNetworkAdapter))
	case guestresource.ResourceTypeVPCIDevice:
		return modifyMappedVPCIDevice(ctx, req.RequestType, req.Settings.(*guestresource.LCOWMappedVPCIDevice))
	case guestresource.ResourceTypeContainerConstraints:
		c, err := h.GetCreatedContainer(containerID)
		if err != nil {
			return err
		}
		return c.modifyContainerConstraints(ctx, req.RequestType, req.Settings.(*guestresource.LCOWContainerConstraints))
	case guestresource.ResourceTypeSecurityPolicy:
		policy, ok := req.Settings.(*securitypolicy.EncodedSecurityPolicy)
		if !ok {
			return errors.New("the request's settings are not of type EncodedSecurityPolicy")
		}

		return h.SetSecurityPolicy(policy.SecurityPolicy)
	default:
		return errors.Errorf("the ResourceType \"%s\" is not supported for UVM", req.ResourceType)
	}
}

func (h *Host) modifyContainerSettings(ctx context.Context, containerID string, req *guestrequest.ModificationRequest) error {
	c, err := h.GetCreatedContainer(containerID)
	if err != nil {
		return err
	}

	switch req.ResourceType {
	case guestresource.ResourceTypeContainerConstraints:
		return c.modifyContainerConstraints(ctx, req.RequestType, req.Settings.(*guestresource.LCOWContainerConstraints))
	default:
		return errors.Errorf("the ResourceType \"%s\" is not supported for containers", req.ResourceType)
	}
}

func (h *Host) ModifySettings(ctx context.Context, containerID string, req *guestrequest.ModificationRequest) error {
	if containerID == UVMContainerID {
		return h.modifyHostSettings(ctx, containerID, req)
	}
	return h.modifyContainerSettings(ctx, containerID, req)
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

func newInvalidRequestTypeError(rt guestrequest.RequestType) error {
	return errors.Errorf("the RequestType %q is not supported", rt)
}

func modifyMappedVirtualDisk(ctx context.Context, rt guestrequest.RequestType, mvd *guestresource.LCOWMappedVirtualDisk, securityPolicy securitypolicy.SecurityPolicyEnforcer) (err error) {
	switch rt {
	case guestrequest.RequestTypeAdd:
		mountCtx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if mvd.MountPath != "" {
			return scsi.Mount(mountCtx, mvd.Controller, mvd.Lun, mvd.MountPath, mvd.ReadOnly, mvd.Encrypted, mvd.Options, mvd.VerityInfo, securityPolicy)
		}
		return nil
	case guestrequest.RequestTypeRemove:
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

func modifyMappedDirectory(ctx context.Context, vsock transport.Transport, rt guestrequest.RequestType, md *guestresource.LCOWMappedDirectory) (err error) {
	switch rt {
	case guestrequest.RequestTypeAdd:
		return plan9.Mount(ctx, vsock, md.MountPath, md.ShareName, uint32(md.Port), md.ReadOnly)
	case guestrequest.RequestTypeRemove:
		return storage.UnmountPath(ctx, md.MountPath, true)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyMappedVPMemDevice(ctx context.Context, rt guestrequest.RequestType, vpd *guestresource.LCOWMappedVPMemDevice, securityPolicy securitypolicy.SecurityPolicyEnforcer) (err error) {
	switch rt {
	case guestrequest.RequestTypeAdd:
		return pmem.Mount(ctx, vpd.DeviceNumber, vpd.MountPath, vpd.MappingInfo, vpd.VerityInfo, securityPolicy)
	case guestrequest.RequestTypeRemove:
		return pmem.Unmount(ctx, vpd.DeviceNumber, vpd.MountPath, vpd.MappingInfo, vpd.VerityInfo, securityPolicy)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyMappedVPCIDevice(ctx context.Context, rt guestrequest.RequestType, vpciDev *guestresource.LCOWMappedVPCIDevice) error {
	switch rt {
	case guestrequest.RequestTypeAdd:
		return pci.WaitForPCIDeviceFromVMBusGUID(ctx, vpciDev.VMBusGUID)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyCombinedLayers(ctx context.Context, rt guestrequest.RequestType, cl *guestresource.LCOWCombinedLayers, securityPolicy securitypolicy.SecurityPolicyEnforcer) (err error) {
	switch rt {
	case guestrequest.RequestTypeAdd:
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

		return overlay.MountLayer(ctx, layerPaths, upperdirPath, workdirPath, cl.ContainerRootPath, readonly, cl.ContainerID, securityPolicy)
	case guestrequest.RequestTypeRemove:
		return storage.UnmountPath(ctx, cl.ContainerRootPath, true)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyNetwork(ctx context.Context, rt guestrequest.RequestType, na *guestresource.LCOWNetworkAdapter) (err error) {
	switch rt {
	case guestrequest.RequestTypeAdd:
		ns := getOrAddNetworkNamespace(na.NamespaceID)
		if err := ns.AddAdapter(ctx, na); err != nil {
			return err
		}
		// This code doesnt know if the namespace was already added to the
		// container or not so it must always call `Sync`.
		return ns.Sync(ctx)
	case guestrequest.RequestTypeRemove:
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
