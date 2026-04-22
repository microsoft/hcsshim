//go:build linux
// +build linux

package hcsv2

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	cgroup1stats "github.com/containerd/cgroups/v3/cgroup1/stats"
	"github.com/mattn/go-shellwords"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	"github.com/Microsoft/hcsshim/internal/debug"
	"github.com/Microsoft/hcsshim/internal/guest/cgroup"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	specGuest "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/guest/storage"
	"github.com/Microsoft/hcsshim/internal/guest/storage/overlay"
	"github.com/Microsoft/hcsshim/internal/guest/storage/pci"
	"github.com/Microsoft/hcsshim/internal/guest/storage/plan9"
	"github.com/Microsoft/hcsshim/internal/guest/storage/pmem"
	"github.com/Microsoft/hcsshim/internal/guest/storage/scsi"
	"github.com/Microsoft/hcsshim/internal/guest/transport"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/verity"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

// UVMContainerID is the ContainerID that will be sent on any prot.MessageBase
// for V2 where the specific message is targeted at the UVM itself.
const UVMContainerID = "00000000-0000-0000-0000-000000000000"

// Prevent path traversal via malformed container / sandbox IDs.  Container IDs
// can be either UVMContainerID, or a 64 character hex string. This is also used
// to check that sandbox IDs (which is also used in paths) are valid, which has
// the same format.
const validContainerIDRegexRaw = `[0-9a-fA-F]{64}`

var validContainerIDRegex = regexp.MustCompile("^" + validContainerIDRegexRaw + "$")

// idType just changes the error message
func checkValidContainerID(id string, idType string) error {
	if id == UVMContainerID || validContainerIDRegex.MatchString(id) {
		return nil
	}

	return errors.Errorf("invalid %s id: %s (must match %s)", idType, id, validContainerIDRegex.String())
}

// uvmPod tracks pod-level state within the UVM.
type uvmPod struct {
	sandboxID        string
	networkNamespace string
	cgroupPath       string
	cgroupControl    cgroup.Manager
	containers       map[string]bool
	createdAt        time.Time
}

// Host is the structure tracking all UVM host state including all containers
// and processes.
type Host struct {
	containersMutex sync.Mutex
	containers      map[string]*Container

	externalProcessesMutex sync.Mutex
	externalProcesses      map[int]*externalProcess

	// pods tracks all pod state. Guarded by podsMutex.
	podsMutex sync.Mutex
	pods      map[string]*uvmPod

	// sandboxRoots maps sandboxID to the resolved sandbox root directory.
	// Populated via registerSandboxRoot during sandbox creation using
	// the host-provided OCIBundlePath as source of truth.
	// Lock ordering: containersMutex -> podsMutex -> sandboxRootsMutex (never reverse).
	sandboxRootsMutex sync.RWMutex
	sandboxRoots      map[string]string

	rtime            runtime.Runtime
	vsock            transport.Transport
	devNullTransport transport.Transport

	// state required for the security policy enforcement
	securityOptions *securitypolicy.SecurityOptions

	// hostMounts keeps the state of currently mounted devices and file systems,
	// which is used for GCS hardening.
	hostMounts *hostMounts
}

func NewHost(rtime runtime.Runtime, vsock transport.Transport, initialEnforcer securitypolicy.SecurityPolicyEnforcer, logWriter io.Writer) *Host {
	securityPolicyOptions := securitypolicy.NewSecurityOptions(
		initialEnforcer,
		false,
		"",
		logWriter,
	)
	return &Host{
		containers:        make(map[string]*Container),
		externalProcesses: make(map[int]*externalProcess),
		pods:              make(map[string]*uvmPod),
		sandboxRoots:      make(map[string]string),
		rtime:             rtime,
		vsock:             vsock,
		devNullTransport:  &transport.DevNullTransport{},
		hostMounts:        newHostMounts(),
		securityOptions:   securityPolicyOptions,
	}
}

// registerSandboxRoot stores the resolved sandbox root directory for a given sandbox ID.
// For virtual pods, it derives the shared root from OCIBundlePath's parent directory.
func (h *Host) registerSandboxRoot(sandboxID, ociBundlePath, virtualPodID string) (string, error) {
	var sandboxRoot string

	if virtualPodID != "" {
		// Validate virtualPodID to prevent path traversal.
		cleanID := filepath.Clean(virtualPodID)
		if filepath.IsAbs(cleanID) || strings.Contains(cleanID, "..") {
			return "", errors.Errorf("invalid virtual pod ID %q: path traversal attempt", virtualPodID)
		}
		sandboxRoot = filepath.Join(filepath.Dir(ociBundlePath), "virtual-pods", cleanID)
	} else {
		sandboxRoot = ociBundlePath
	}

	h.sandboxRootsMutex.Lock()
	defer h.sandboxRootsMutex.Unlock()
	h.sandboxRoots[sandboxID] = sandboxRoot

	logrus.WithFields(logrus.Fields{
		"sandboxID":   sandboxID,
		"sandboxRoot": sandboxRoot,
	}).Debug("registered sandbox root")

	return sandboxRoot, nil
}

// resolveSandboxRoot returns the resolved sandbox root for the given sandbox ID.
// Falls back to legacy path derivation if no mapping exists.
func (h *Host) resolveSandboxRoot(sandboxID string) string {
	h.sandboxRootsMutex.RLock()
	root, ok := h.sandboxRoots[sandboxID]
	h.sandboxRootsMutex.RUnlock()
	if ok {
		return root
	}
	// Fallback to legacy derivation for backwards compatibility.
	// TODO: remove fallback after shim v1 sunset
	fallback := specGuest.SandboxRootDir(sandboxID)
	logrus.WithFields(logrus.Fields{
		"sandboxID": sandboxID,
		"fallback":  fallback,
	}).Warn("sandbox root not found in mapping, falling back to legacy path derivation")
	return fallback
}

// unregisterSandboxRoot removes the sandbox root mapping for a given sandbox ID.
func (h *Host) unregisterSandboxRoot(sandboxID string) {
	h.sandboxRootsMutex.Lock()
	defer h.sandboxRootsMutex.Unlock()
	delete(h.sandboxRoots, sandboxID)
}

func (h *Host) SecurityPolicyEnforcer() securitypolicy.SecurityPolicyEnforcer {
	return h.securityOptions.PolicyEnforcer
}

func (h *Host) SecurityOptions() *securitypolicy.SecurityOptions {
	return h.securityOptions
}

func (h *Host) Transport() transport.Transport {
	return h.vsock
}

func (h *Host) RemoveContainer(id string) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	c, ok := h.containers[id]
	if !ok {
		return
	}

	criType, isCRI := c.spec.Annotations[annotations.KubernetesContainerType]

	// Do NOT call RemoveNetworkNamespace for virtual pod sandbox containers.
	// The host-driven teardown path (TearDownNetworking → RemoveNetNS → removeNIC)
	// removes adapters first and then the namespace. Calling it here would fail
	// with "contains adapters" because the host hasn't removed them yet.
	virtualPodID := c.spec.Annotations[annotations.VirtualPodID]
	isVirtualPodSandbox := virtualPodID != "" && id == virtualPodID
	if !isVirtualPodSandbox && (!isCRI || criType == "sandbox") {
		if err := RemoveNetworkNamespace(context.Background(), id); err != nil {
			logrus.WithError(err).WithField(logfields.ContainerID, id).Warn("failed to remove network namespace")
		}
	}

	delete(h.containers, id)

	// Extract pod cgroup manager under lock, delete cgroup outside lock to
	// avoid holding podsMutex during filesystem I/O.
	var cgToDelete cgroup.Manager
	h.podsMutex.Lock()
	if c.sandboxID != "" {
		if pod, exists := h.pods[c.sandboxID]; exists {
			delete(pod.containers, id)
			if id == c.sandboxID {
				cgToDelete = pod.cgroupControl
				delete(h.pods, c.sandboxID)
			}
		}
	}
	h.podsMutex.Unlock()

	if cgToDelete != nil {
		if err := cgToDelete.Delete(); err != nil {
			logrus.WithFields(logrus.Fields{
				"sandboxID": c.sandboxID,
			}).WithError(err).Warn("failed to delete pod cgroup")
		}
	}

	// Clean up the sandbox root mapping for sandbox containers.
	if c.isSandbox {
		h.unregisterSandboxRoot(id)
	}
}

func (h *Host) GetCreatedContainer(id string) (*Container, error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	c, ok := h.containers[id]
	if !ok {
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	}
	if c.getStatus() != containerCreated {
		return nil, fmt.Errorf("container is not in state \"created\": %w",
			gcserr.NewHresultError(gcserr.HrVmcomputeInvalidState))
	}
	return c, nil
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

// setupSandboxMountsPath creates the sandboxMounts directory from a resolved root.
func setupSandboxMountsPath(sandboxRoot string) (err error) {
	mountPath := specGuest.SandboxMountsDirFromRoot(sandboxRoot)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create sandboxMounts dir at %v", mountPath)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(mountPath)
		}
	}()

	return storage.MountRShared(mountPath)
}

// setupSandboxTmpfsMountsPath creates the sandbox tmpfs mounts directory from a resolved root.
func setupSandboxTmpfsMountsPath(sandboxRoot string) (err error) {
	tmpfsDir := specGuest.SandboxTmpfsMountsDirFromRoot(sandboxRoot)
	if err := os.MkdirAll(tmpfsDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create sandbox tmpfs mounts dir at %v", tmpfsDir)
	}

	defer func() {
		if err != nil {
			_ = os.RemoveAll(tmpfsDir)
		}
	}()

	// mount a tmpfs at the tmpfsDir
	// this ensures that the tmpfsDir is a mount point and not just a directory
	// we don't care if it is already mounted, so ignore EBUSY
	if err := unix.Mount("tmpfs", tmpfsDir, "tmpfs", 0, ""); err != nil && !errors.Is(err, unix.EBUSY) {
		return errors.Wrapf(err, "failed to mount tmpfs at %s", tmpfsDir)
	}

	//TODO: should tmpfs be mounted as noexec?

	return storage.MountRShared(tmpfsDir)
}

// setupSandboxHugePageMountsPath creates the hugepages mounts directory from a resolved root.
func setupSandboxHugePageMountsPath(sandboxRoot string) error {
	mountPath := specGuest.SandboxHugePagesMountsDirFromRoot(sandboxRoot)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create hugepage mounts dir at %v", mountPath)
	}

	return storage.MountRShared(mountPath)
}

// setupSandboxLogDir creates the directory to house all redirected stdio logs from a resolved root.
func setupSandboxLogDir(sandboxRoot string) error {
	mountPath := specGuest.SandboxLogsDirFromRoot(sandboxRoot)
	if err := mkdirAllModePerm(mountPath); err != nil {
		return errors.Wrapf(err, "failed to create sandbox logs dir at %v", mountPath)
	}
	return nil
}

// TODO: unify workload and standalone logic for non-sandbox features (e.g., block devices, huge pages, uVM mounts)
// TODO(go1.24): use [os.Root] instead of `!strings.HasPrefix(<path>, <root>)`

// Returns whether this host has a security policy set, i.e. if it's running
// confidential containers.
func (h *Host) HasSecurityPolicy() bool {
	return len(h.securityOptions.PolicyEnforcer.EncodedSecurityPolicy()) > 0
}

// For confidential containers, make sure that the host can't use unexpected
// bundle paths / scratch dir / rootfs
func checkContainerSettings(sandboxID, containerID string, settings *prot.VMHostedContainerSettingsV2) error {
	if settings.OCISpecification == nil {
		return errors.Errorf("OCISpecification is nil")
	}
	if settings.OCISpecification.Root == nil {
		return errors.Errorf("OCISpecification.Root is nil")
	}

	// matches with CreateContainer / createLinuxContainerDocument in internal/hcsoci
	containerRootInUVM := path.Join(guestpath.LCOWRootPrefixInUVM, containerID)
	if settings.OCIBundlePath != containerRootInUVM {
		return errors.Errorf("OCIBundlePath %q must equal expected %q",
			settings.OCIBundlePath, containerRootInUVM)
	}
	expectedContainerRootfs := path.Join(containerRootInUVM, guestpath.RootfsPath)
	if settings.OCISpecification.Root.Path != expectedContainerRootfs {
		return errors.Errorf("OCISpecification.Root.Path %q must equal expected %q",
			settings.OCISpecification.Root.Path, expectedContainerRootfs)
	}

	// matches with MountLCOWLayers
	scratchDirPath := settings.ScratchDirPath
	expectedScratchDirPathNonShared := path.Join(containerRootInUVM, guestpath.ScratchDir, containerID)
	expectedScratchDirPathShared := path.Join(guestpath.LCOWRootPrefixInUVM, sandboxID, guestpath.ScratchDir, containerID)
	if scratchDirPath != expectedScratchDirPathNonShared &&
		scratchDirPath != expectedScratchDirPathShared {
		return errors.Errorf("ScratchDirPath %q must be either %q or %q",
			scratchDirPath, expectedScratchDirPathNonShared, expectedScratchDirPathShared)
	}

	if settings.OCISpecification.Hooks != nil {
		return errors.Errorf("OCISpecification.Hooks must be nil.")
	}

	return nil
}

func (h *Host) CreateContainer(ctx context.Context, id string, settings *prot.VMHostedContainerSettingsV2) (_ *Container, err error) {
	criType, isCRI := settings.OCISpecification.Annotations[annotations.KubernetesContainerType]

	// Check for virtual pod annotation
	virtualPodID := settings.OCISpecification.Annotations[annotations.VirtualPodID]
	isVirtualPod := virtualPodID != ""

	if h.HasSecurityPolicy() {
		if err = checkValidContainerID(id, "container"); err != nil {
			return nil, err
		}
		if virtualPodID != "" {
			if err = checkValidContainerID(virtualPodID, "virtual pod"); err != nil {
				return nil, err
			}
		}
	}

	// Special handling for virtual pod sandbox containers:
	// The first container in a virtual pod (containerID == virtualPodID) should be treated as a sandbox
	// even if the CRI annotation might indicate otherwise due to host-side UVM setup differences
	if isVirtualPod && id == virtualPodID {
		criType = "sandbox"
		isCRI = true
		logrus.WithFields(logrus.Fields{
			logfields.ContainerID:      id,
			logfields.VirtualSandboxID: virtualPodID,
			"originalCriType":          settings.OCISpecification.Annotations[annotations.KubernetesContainerType],
		}).Info("Virtual pod first container detected - treating as sandbox container")
	}

	c := &Container{
		id:             id,
		vsock:          h.vsock,
		spec:           settings.OCISpecification,
		ociBundlePath:  settings.OCIBundlePath,
		isSandbox:      criType == "sandbox",
		exitType:       prot.NtUnexpectedExit,
		processes:      make(map[uint32]*containerProcess),
		scratchDirPath: settings.ScratchDirPath,
	}
	c.setStatus(containerCreating)

	if err := h.AddContainer(id, c); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			h.RemoveContainer(id)
		}
	}()

	// Determine the sandboxID for this container.
	sandboxID := id
	if criType == "container" {
		sid := settings.OCISpecification.Annotations[annotations.KubernetesSandboxID]
		if sid == "" {
			return nil, errors.Errorf("workload container missing sandbox ID annotation")
		}
		sandboxID = sid
	} else if virtualPodID != "" {
		sandboxID = virtualPodID
	}
	c.sandboxID = sandboxID

	// Normally we would be doing policy checking here at the start of our
	// "policy gated function". However, we can't for create container as we
	// need a properly correct sandboxID which might be changed by the code
	// below that determines the sandboxID. This is a bit of future proofing
	// as currently for our single use case, the sandboxID is the same as the
	// container id

	var namespaceID string
	if isCRI {
		switch criType {
		case "sandbox":
			// Capture namespaceID if any because setupSandboxContainerSpec clears the Windows section.
			namespaceID = specGuest.GetNetworkNamespaceID(settings.OCISpecification)

			// Resolve the sandbox root from OCIBundlePath.
			sandboxRoot, err := h.registerSandboxRoot(id, settings.OCIBundlePath, virtualPodID)
			if err != nil {
				return nil, err
			}
			c.sandboxRoot = sandboxRoot

			err = setupSandboxContainerSpec(ctx, id, sandboxRoot, settings.OCISpecification)
			if err != nil {
				return nil, err
			}
			defer func() {
				if err != nil {
					_ = os.RemoveAll(settings.OCIBundlePath)
				}
			}()

			if err = setupSandboxMountsPath(sandboxRoot); err != nil {
				return nil, err
			}
			if err = setupSandboxTmpfsMountsPath(sandboxRoot); err != nil {
				return nil, err
			}
			if err = setupSandboxHugePageMountsPath(sandboxRoot); err != nil {
				return nil, err
			}
			if err = setupSandboxLogDir(sandboxRoot); err != nil {
				return nil, err
			}

			if err := securitypolicy.ExtendPolicyWithNetworkingMounts(id, h.securityOptions.PolicyEnforcer, settings.OCISpecification); err != nil {
				return nil, err
			}

			if err := h.createPodInUVM(sandboxID, settings.OCISpecification, namespaceID); err != nil {
				return nil, err
			}
		case "container":
			sid, ok := settings.OCISpecification.Annotations[annotations.KubernetesSandboxID]
			if h.HasSecurityPolicy() {
				if err = checkValidContainerID(sid, "sandbox"); err != nil {
					return nil, err
				}
			}
			if !ok || sid == "" {
				return nil, errors.Errorf("unsupported 'io.kubernetes.cri.sandbox-id': '%s'", sid)
			}
			sandboxRoot := h.resolveSandboxRoot(sid)
			c.sandboxRoot = sandboxRoot
			if err = setupWorkloadContainerSpec(ctx, sid, id, sandboxRoot, settings.OCISpecification, settings.OCIBundlePath); err != nil {
				return nil, err
			}

			// Add SEV device when security policy is not empty, except when privileged annotation is
			// set to "true", in which case all UVMs devices are added.
			if h.HasSecurityPolicy() && !oci.ParseAnnotationsBool(ctx,
				settings.OCISpecification.Annotations, annotations.LCOWPrivileged, false) {
				if err := specGuest.AddDevSev(ctx, settings.OCISpecification); err != nil {
					log.G(ctx).WithError(err).Debug("failed to add SEV device")
				}
			}

			defer func() {
				if err != nil {
					_ = os.RemoveAll(settings.OCIBundlePath)
				}
			}()
			if err := securitypolicy.ExtendPolicyWithNetworkingMounts(sandboxID, h.securityOptions.PolicyEnforcer, settings.OCISpecification); err != nil {
				return nil, err
			}
		default:
			return nil, errors.Errorf("unsupported 'io.kubernetes.cri.container-type': '%s'", criType)
		}
	} else {
		// Capture namespaceID if any because setupStandaloneContainerSpec clears the Windows section.
		namespaceID = specGuest.GetNetworkNamespaceID(settings.OCISpecification)
		// Standalone uses OCIBundlePath directly as its root.
		c.sandboxRoot = settings.OCIBundlePath
		if err := setupStandaloneContainerSpec(ctx, id, settings.OCIBundlePath, settings.OCISpecification); err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				_ = os.RemoveAll(settings.OCIBundlePath)
			}
		}()
		if err := securitypolicy.ExtendPolicyWithNetworkingMounts(id, h.securityOptions.PolicyEnforcer,
			settings.OCISpecification); err != nil {
			return nil, err
		}
	}

	// Register container with its pod for CRI containers.
	if isCRI {
		h.addContainerToPod(sandboxID, id)
	}

	// don't specialize tee logs (both files and mounts) just for workload containers
	// add log directory mount before enforcing (mount) policy
	if logDirMount := settings.OCISpecification.Annotations[annotations.LCOWTeeLogDirMount]; logDirMount != "" {
		settings.OCISpecification.Mounts = append(settings.OCISpecification.Mounts, specs.Mount{
			Destination: logDirMount,
			Type:        "bind",
			Source:      specGuest.SandboxLogsDirFromRoot(c.sandboxRoot),
			Options:     []string{"bind"},
		})
	}

	if h.HasSecurityPolicy() {
		if err = checkContainerSettings(sandboxID, id, settings); err != nil {
			return nil, err
		}
	}

	user, groups, umask, err := h.securityOptions.PolicyEnforcer.GetUserInfo(settings.OCISpecification.Process, settings.OCISpecification.Root.Path)
	if err != nil {
		return nil, err
	}

	seccomp, err := securitypolicy.MeasureSeccompProfile(settings.OCISpecification.Linux.Seccomp)
	if err != nil {
		return nil, err
	}

	envToKeep, capsToKeep, allowStdio, err := h.securityOptions.PolicyEnforcer.EnforceCreateContainerPolicy(
		ctx,
		sandboxID,
		id,
		settings.OCISpecification.Process.Args,
		settings.OCISpecification.Process.Env,
		settings.OCISpecification.Process.Cwd,
		settings.OCISpecification.Mounts,
		isPrivilegedContainerCreationRequest(ctx, settings.OCISpecification),
		settings.OCISpecification.Process.NoNewPrivileges,
		user,
		groups,
		umask,
		settings.OCISpecification.Process.Capabilities,
		seccomp,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "container creation denied due to policy")
	}

	if !allowStdio {
		// stdio access isn't allow for this container. Switch to the /dev/null
		// transport that will eat all input/ouput.
		c.vsock = h.devNullTransport
	}

	// delay creating the directory to house the container's stdio until after we've verified
	// policy on log settings.
	// TODO: is using allowStdio appropriate here, since longs aren't leaving the uVM?
	if logPath := settings.OCISpecification.Annotations[annotations.LCOWTeeLogPath]; logPath != "" {
		if !allowStdio {
			return nil, errors.Errorf("teeing container stdio to log path %q denied due to policy not allowing stdio access", logPath)
		}

		logsDir := specGuest.SandboxLogsDirFromRoot(c.sandboxRoot)
		c.logPath = filepath.Join(logsDir, logPath)
		// verify the logpath is still under the correct directory
		if !strings.HasPrefix(c.logPath, logsDir+"/") {
			return nil, errors.Errorf("log path %v is not within sandbox's log dir", c.logPath)
		}

		dir := filepath.Dir(c.logPath)
		log.G(ctx).WithFields(logrus.Fields{
			logfields.Path:        dir,
			logfields.ContainerID: id,
		}).Debug("creating container log file parent directory in uVM")
		if err := mkdirAllModePerm(dir); err != nil {
			return nil, errors.Wrapf(err, "failed to create log file parent directory: %s", dir)
		}
	}

	if envToKeep != nil {
		settings.OCISpecification.Process.Env = []string(envToKeep)
	}

	if capsToKeep != nil {
		settings.OCISpecification.Process.Capabilities = capsToKeep
	}

	if oci.ParseAnnotationsBool(ctx, settings.OCISpecification.Annotations, annotations.LCOWSecurityPolicyEnv, true) {
		if err := h.securityOptions.WriteSecurityContextDir(settings.OCISpecification); err != nil {
			return nil, fmt.Errorf("failed to write security context dir: %w", err)
		}
	}

	// Create the BundlePath
	if err := os.MkdirAll(settings.OCIBundlePath, 0700); err != nil {
		return nil, errors.Wrapf(err, "failed to create OCIBundlePath: '%s'", settings.OCIBundlePath)
	}

	if err := writeSpecToFile(ctx, path.Join(settings.OCIBundlePath, "config.json"), settings.OCISpecification); err != nil {
		return nil, err
	}

	con, err := h.rtime.CreateContainer(sandboxID, id, settings.OCIBundlePath, nil)
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
		// skip network activity for sandbox containers marked with skip uvm networking annotation
		if isCRI && err != nil && !strings.EqualFold(settings.OCISpecification.Annotations[annotations.SkipPodNetworking], "true") {
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

func writeSpecToFile(ctx context.Context, configFile string, spec *specs.Spec) error {
	f, err := os.Create(configFile)
	if err != nil {
		return errors.Wrapf(err, "failed to create config.json at: '%s'", configFile)
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	// capture what we write to the config file in a byte buffer so we can log it later
	var w io.Writer = writer
	buf := &bytes.Buffer{}
	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		w = io.MultiWriter(writer, buf)
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // not embedding JSON into HTML, so no need to escape
	if err := enc.Encode(spec); err != nil {
		return errors.Wrapf(err, "failed to write OCISpecification to config.json at: '%s'", configFile)
	}
	if err := writer.Flush(); err != nil {
		return errors.Wrapf(err, "failed to flush writer for config.json at: '%s'", configFile)
	}

	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		entry := log.G(ctx).WithField(logfields.Path, configFile)

		if b, err := log.ScrubOCISpec(buf.Bytes()); err != nil {
			entry.WithError(err).Warning("could not scrub OCI spec written to config.json")
		} else {
			log.G(ctx).WithField(
				"config", string(bytes.TrimSpace(b)),
			).Trace("wrote OCI spec to config.json")
		}
	}

	return nil
}

func (h *Host) modifyHostSettings(ctx context.Context, containerID string, req *guestrequest.ModificationRequest) (retErr error) {
	if h.HasSecurityPolicy() {
		if err := checkValidContainerID(containerID, "container"); err != nil {
			return err
		}
	}

	switch req.ResourceType {
	case guestresource.ResourceTypeSCSIDevice:
		return modifySCSIDevice(ctx, req.RequestType, req.Settings.(*guestresource.SCSIDevice))
	case guestresource.ResourceTypeMappedVirtualDisk:
		mvd := req.Settings.(*guestresource.LCOWMappedVirtualDisk)
		// find the actual controller number on the bus and update the incoming request.
		var cNum uint8
		cNum, err := scsi.ActualControllerNumber(ctx, mvd.Controller)
		if err != nil {
			return err
		}
		mvd.Controller = cNum
		// first we try to update the internal state for read-write attachments.
		if !mvd.ReadOnly {
			localCtx, cancel := context.WithTimeout(ctx, time.Second*5)
			defer cancel()
			source, err := scsi.GetDevicePath(localCtx, mvd.Controller, mvd.Lun, mvd.Partition)
			if err != nil {
				return err
			}
			switch req.RequestType {
			case guestrequest.RequestTypeAdd:
				if err := h.hostMounts.AddRWDevice(mvd.MountPath, source, mvd.Encrypted); err != nil {
					return err
				}
				defer func() {
					if retErr != nil {
						_ = h.hostMounts.RemoveRWDevice(mvd.MountPath, source)
					}
				}()
			case guestrequest.RequestTypeRemove:
				if err := h.hostMounts.RemoveRWDevice(mvd.MountPath, source); err != nil {
					return err
				}
				defer func() {
					if retErr != nil {
						_ = h.hostMounts.AddRWDevice(mvd.MountPath, source, mvd.Encrypted)
					}
				}()
			}
		}
		return modifyMappedVirtualDisk(ctx, req.RequestType, mvd, h.securityOptions.PolicyEnforcer)
	case guestresource.ResourceTypeMappedDirectory:
		return modifyMappedDirectory(ctx, h.vsock, req.RequestType, req.Settings.(*guestresource.LCOWMappedDirectory), h.securityOptions.PolicyEnforcer)
	case guestresource.ResourceTypeVPMemDevice:
		return modifyMappedVPMemDevice(ctx, req.RequestType, req.Settings.(*guestresource.LCOWMappedVPMemDevice), h.securityOptions.PolicyEnforcer)
	case guestresource.ResourceTypeCombinedLayers:
		cl := req.Settings.(*guestresource.LCOWCombinedLayers)
		// when cl.ScratchPath == "", we mount overlay as read-only, in which case
		// we don't really care about scratch encryption, since the host already
		// knows about the layers and the overlayfs.
		encryptedScratch := cl.ScratchPath != "" && h.hostMounts.IsEncrypted(cl.ScratchPath)
		return modifyCombinedLayers(ctx, req.RequestType, req.Settings.(*guestresource.LCOWCombinedLayers), encryptedScratch, h.securityOptions.PolicyEnforcer)
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
		r, ok := req.Settings.(*guestresource.ConfidentialOptions)
		if !ok {
			return errors.New("the request's settings are not of type ConfidentialOptions")
		}
		return h.securityOptions.SetConfidentialOptions(ctx,
			r.EnforcerType,
			r.EncodedSecurityPolicy,
			r.EncodedUVMReference)
	case guestresource.ResourceTypePolicyFragment:
		r, ok := req.Settings.(*guestresource.SecurityPolicyFragment)
		if !ok {
			return errors.New("the request settings are not of type SecurityPolicyFragment")
		}
		return h.securityOptions.InjectFragment(ctx, r)
	default:
		return errors.Errorf("the ResourceType %q is not supported for UVM", req.ResourceType)
	}
}

func (h *Host) modifyContainerSettings(ctx context.Context, containerID string, req *guestrequest.ModificationRequest) error {
	if h.HasSecurityPolicy() {
		if err := checkValidContainerID(containerID, "container"); err != nil {
			return err
		}
	}

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
func (*Host) Shutdown() {
	_ = syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
}

// Called to shutdown a container
func (h *Host) ShutdownContainer(ctx context.Context, containerID string, graceful bool) error {
	c, err := h.GetCreatedContainer(containerID)
	if err != nil {
		return err
	}

	err = h.securityOptions.PolicyEnforcer.EnforceShutdownContainerPolicy(ctx, containerID)
	if err != nil {
		return err
	}

	signal := unix.SIGTERM
	if !graceful {
		signal = unix.SIGKILL
	}

	return c.Kill(ctx, signal)
}

func (h *Host) SignalContainerProcess(ctx context.Context, containerID string, processID uint32, signal syscall.Signal) error {
	c, err := h.GetCreatedContainer(containerID)
	if err != nil {
		return err
	}

	p, err := c.GetProcess(processID)
	if err != nil {
		return err
	}

	signalingInitProcess := processID == c.initProcess.pid

	startupArgList := p.(*containerProcess).spec.Args
	err = h.securityOptions.PolicyEnforcer.EnforceSignalContainerProcessPolicy(ctx, containerID, signal, signalingInitProcess, startupArgList)
	if err != nil {
		return err
	}

	return p.Kill(ctx, signal)
}

func (h *Host) ExecProcess(ctx context.Context, containerID string, params prot.ProcessParameters, conSettings stdio.ConnectionSettings) (_ int, err error) {
	var pid int
	var c *Container

	if params.IsExternal || containerID == UVMContainerID {
		var envToKeep securitypolicy.EnvList
		var allowStdioAccess bool
		envToKeep, allowStdioAccess, err = h.securityOptions.PolicyEnforcer.EnforceExecExternalProcessPolicy(
			ctx,
			params.CommandArgs,
			processParamEnvToOCIEnv(params.Environment),
			params.WorkingDirectory,
		)
		if err != nil {
			return pid, errors.Wrapf(err, "exec is denied due to policy")
		}

		// It makes no sense to allow access if stdio access is denied and the
		// process requires a terminal.
		if params.EmulateConsole && !allowStdioAccess {
			return pid, errors.New("exec of process that requires terminal access denied due to policy not allowing stdio access")
		}

		if envToKeep != nil {
			params.Environment = processOCIEnvToParam(envToKeep)
		}

		var tport = h.vsock
		if !allowStdioAccess {
			tport = h.devNullTransport
		}
		pid, err = h.runExternalProcess(ctx, params, conSettings, tport)
	} else if c, err = h.GetCreatedContainer(containerID); err == nil {
		// We found a V2 container. Treat this as a V2 process.
		if params.OCIProcess == nil {
			// We've already done policy enforcement for creating a container so
			// there's no policy enforcement to do for starting
			pid, err = c.Start(ctx, conSettings)
		} else {
			// Windows uses a different field for command, there's no enforcement
			// around this yet for Windows so this is Linux specific at the moment.

			var envToKeep securitypolicy.EnvList
			var capsToKeep *specs.LinuxCapabilities
			var user securitypolicy.IDName
			var groups []securitypolicy.IDName
			var umask string
			var allowStdioAccess bool

			user, groups, umask, err = h.securityOptions.PolicyEnforcer.GetUserInfo(params.OCIProcess, c.spec.Root.Path)
			if err != nil {
				return 0, err
			}

			envToKeep, capsToKeep, allowStdioAccess, err = h.securityOptions.PolicyEnforcer.EnforceExecInContainerPolicy(
				ctx,
				containerID,
				params.OCIProcess.Args,
				params.OCIProcess.Env,
				params.OCIProcess.Cwd,
				params.OCIProcess.NoNewPrivileges,
				user,
				groups,
				umask,
				params.OCIProcess.Capabilities,
			)
			if err != nil {
				return pid, errors.Wrapf(err, "exec in container denied due to policy")
			}

			// It makes no sense to allow access if stdio access is denied and the
			// process requires a terminal.
			if params.OCIProcess.Terminal && !allowStdioAccess {
				return pid, errors.New("exec in container of process that requires terminal access denied due to policy not allowing stdio access")
			}

			if envToKeep != nil {
				params.OCIProcess.Env = envToKeep
			}

			if capsToKeep != nil {
				params.OCIProcess.Capabilities = capsToKeep
			}

			pid, err = c.ExecProcess(ctx, params.OCIProcess, conSettings)
		}
	}

	return pid, err
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

func (h *Host) GetProperties(ctx context.Context, containerID string, query prot.PropertyQuery) (*prot.PropertiesV2, error) {
	err := h.securityOptions.PolicyEnforcer.EnforceGetPropertiesPolicy(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "get properties denied due to policy")
	}

	c, err := h.GetCreatedContainer(containerID)
	if err != nil {
		return nil, err
	}

	properties := &prot.PropertiesV2{}
	for _, requestedProperty := range query.PropertyTypes {
		switch requestedProperty {
		case prot.PtProcessList:
			pids, err := c.GetAllProcessPids(ctx)
			if err != nil {
				return nil, err
			}
			properties.ProcessList = make([]prot.ProcessDetails, len(pids))
			for i, pid := range pids {
				if specGuest.OutOfUint32Bounds(pid) {
					return nil, errors.Errorf("PID (%d) exceeds uint32 bounds", pid)
				}
				properties.ProcessList[i].ProcessID = uint32(pid)
			}
		case prot.PtStatistics:
			cgroupMetrics, err := c.GetStats(ctx)
			if err != nil {
				return nil, err
			}
			// zero out [Blkio] sections, since:
			//  1. (Az)CRI (currently) only looks at the CPU and memory sections; and
			//  2. it can get very large for containers with many layers
			if cgroupMetrics.GetBlkio() != nil {
				cgroupMetrics.Blkio.Reset()
			}
			// also preemptively zero out [Rdma] and [Network], since they could also grow untenable large
			if cgroupMetrics.GetRdma() != nil {
				cgroupMetrics.Rdma.Reset()
			}
			if len(cgroupMetrics.GetNetwork()) > 0 {
				cgroupMetrics.Network = []*cgroup1stats.NetworkStat{}
			}
			if logrus.IsLevelEnabled(logrus.TraceLevel) {
				log.G(ctx).WithField("stats", log.Format(ctx, cgroupMetrics)).Trace("queried cgroup statistics")
			}
			properties.Metrics = cgroupMetrics
		default:
			log.G(ctx).WithField("propertyType", requestedProperty).Warn("unknown or empty property type")
		}
	}

	return properties, nil
}

func (h *Host) GetStacks(ctx context.Context) (string, error) {
	err := h.securityOptions.PolicyEnforcer.EnforceDumpStacksPolicy(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "dump stacks denied due to policy")
	}

	return debug.DumpStacks(), nil
}

// RunExternalProcess runs a process in the utility VM.
func (h *Host) runExternalProcess(
	ctx context.Context,
	params prot.ProcessParameters,
	conSettings stdio.ConnectionSettings,
	tport transport.Transport,
) (_ int, err error) {
	var stdioSet *stdio.ConnectionSet
	stdioSet, err = stdio.Connect(tport, conSettings)
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

func newInvalidRequestTypeError(rt guestrequest.RequestType) error {
	return errors.Errorf("the RequestType %q is not supported", rt)
}

func modifySCSIDevice(
	ctx context.Context,
	rt guestrequest.RequestType,
	msd *guestresource.SCSIDevice,
) error {
	switch rt {
	case guestrequest.RequestTypeRemove:
		cNum, err := scsi.ActualControllerNumber(ctx, msd.Controller)
		if err != nil {
			return err
		}
		return scsi.UnplugDevice(ctx, cNum, msd.Lun)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyMappedVirtualDisk(
	ctx context.Context,
	rt guestrequest.RequestType,
	mvd *guestresource.LCOWMappedVirtualDisk,
	securityPolicy securitypolicy.SecurityPolicyEnforcer,
) (err error) {
	var verityInfo *guestresource.DeviceVerityInfo
	if mvd.ReadOnly {
		// The only time the policy is empty, and we want it to be empty
		// is when no policy is provided, and we default to open door
		// policy. In any other case, e.g. explicit open door or any
		// other rego policy we would like to mount layers with verity.
		if len(securityPolicy.EncodedSecurityPolicy()) > 0 {
			devPath, err := scsi.GetDevicePath(ctx, mvd.Controller, mvd.Lun, mvd.Partition)
			if err != nil {
				return err
			}
			verityInfo, err = verity.ReadVeritySuperBlock(ctx, devPath)
			if err != nil {
				return err
			}
			if mvd.Filesystem != "" && mvd.Filesystem != "ext4" {
				return errors.Errorf("filesystem must be ext4 for read-only scsi mounts")
			}
		}
	}
	switch rt {
	case guestrequest.RequestTypeAdd:
		mountCtx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if mvd.MountPath != "" {
			if mvd.ReadOnly {
				var deviceHash string
				if verityInfo != nil {
					deviceHash = verityInfo.RootDigest
				}
				err = securityPolicy.EnforceDeviceMountPolicy(ctx, mvd.MountPath, deviceHash)
				if err != nil {
					return errors.Wrapf(err, "mounting scsi device controller %d lun %d onto %s denied by policy", mvd.Controller, mvd.Lun, mvd.MountPath)
				}
			} else {
				err = securityPolicy.EnforceRWDeviceMountPolicy(ctx, mvd.MountPath, mvd.Encrypted, mvd.EnsureFilesystem, mvd.Filesystem)
				if err != nil {
					return errors.Wrapf(err, "mounting scsi device controller %d lun %d onto %s denied by policy", mvd.Controller, mvd.Lun, mvd.MountPath)
				}
			}
			config := &scsi.Config{
				Encrypted:        mvd.Encrypted,
				VerityInfo:       verityInfo,
				EnsureFilesystem: mvd.EnsureFilesystem,
				Filesystem:       mvd.Filesystem,
				BlockDev:         mvd.BlockDev,
			}
			return scsi.Mount(mountCtx, mvd.Controller, mvd.Lun, mvd.Partition, mvd.MountPath,
				mvd.ReadOnly, mvd.Options, config)
		}
		return nil
	case guestrequest.RequestTypeRemove:
		if mvd.MountPath != "" {
			if mvd.ReadOnly {
				if err := securityPolicy.EnforceDeviceUnmountPolicy(ctx, mvd.MountPath); err != nil {
					return fmt.Errorf("unmounting scsi device at %s denied by policy: %w", mvd.MountPath, err)
				}
			} else {
				if err := securityPolicy.EnforceRWDeviceUnmountPolicy(ctx, mvd.MountPath); err != nil {
					return fmt.Errorf("unmounting scsi device at %s denied by policy: %w", mvd.MountPath, err)
				}
			}
			config := &scsi.Config{
				Encrypted:        mvd.Encrypted,
				VerityInfo:       verityInfo,
				EnsureFilesystem: mvd.EnsureFilesystem,
				Filesystem:       mvd.Filesystem,
				BlockDev:         mvd.BlockDev,
			}
			if err := scsi.Unmount(ctx, mvd.Controller, mvd.Lun, mvd.Partition,
				mvd.MountPath, config); err != nil {
				return err
			}
		}
		return nil
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyMappedDirectory(
	ctx context.Context,
	vsock transport.Transport,
	rt guestrequest.RequestType,
	md *guestresource.LCOWMappedDirectory,
	securityPolicy securitypolicy.SecurityPolicyEnforcer,
) (err error) {
	switch rt {
	case guestrequest.RequestTypeAdd:
		err = securityPolicy.EnforcePlan9MountPolicy(ctx, md.MountPath)
		if err != nil {
			return errors.Wrapf(err, "mounting plan9 device at %s denied by policy", md.MountPath)
		}

		return plan9.Mount(ctx, vsock, md.MountPath, md.ShareName, uint32(md.Port), md.ReadOnly)
	case guestrequest.RequestTypeRemove:
		err = securityPolicy.EnforcePlan9UnmountPolicy(ctx, md.MountPath)
		if err != nil {
			return errors.Wrapf(err, "unmounting plan9 device at %s denied by policy", md.MountPath)
		}

		return storage.UnmountPath(ctx, md.MountPath, true)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyMappedVPMemDevice(ctx context.Context,
	rt guestrequest.RequestType,
	vpd *guestresource.LCOWMappedVPMemDevice,
	securityPolicy securitypolicy.SecurityPolicyEnforcer,
) (err error) {
	var verityInfo *guestresource.DeviceVerityInfo
	var deviceHash string
	if len(securityPolicy.EncodedSecurityPolicy()) > 0 {
		if vpd.MappingInfo != nil {
			return fmt.Errorf("multi mapping is not supported with verity")
		}
		verityInfo, err = verity.ReadVeritySuperBlock(ctx, pmem.GetDevicePath(vpd.DeviceNumber))
		if err != nil {
			return err
		}
		deviceHash = verityInfo.RootDigest
	}
	switch rt {
	case guestrequest.RequestTypeAdd:
		err = securityPolicy.EnforceDeviceMountPolicy(ctx, vpd.MountPath, deviceHash)
		if err != nil {
			return errors.Wrapf(err, "mounting pmem device %d onto %s denied by policy", vpd.DeviceNumber, vpd.MountPath)
		}

		return pmem.Mount(ctx, vpd.DeviceNumber, vpd.MountPath, vpd.MappingInfo, verityInfo)
	case guestrequest.RequestTypeRemove:
		if err := securityPolicy.EnforceDeviceUnmountPolicy(ctx, vpd.MountPath); err != nil {
			return errors.Wrapf(err, "unmounting pmem device from %s denied by policy", vpd.MountPath)
		}

		return pmem.Unmount(ctx, vpd.DeviceNumber, vpd.MountPath, vpd.MappingInfo, verityInfo)
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

func modifyCombinedLayers(
	ctx context.Context,
	rt guestrequest.RequestType,
	cl *guestresource.LCOWCombinedLayers,
	scratchEncrypted bool,
	securityPolicy securitypolicy.SecurityPolicyEnforcer,
) (err error) {
	isConfidential := len(securityPolicy.EncodedSecurityPolicy()) > 0
	containerID := cl.ContainerID

	switch rt {
	case guestrequest.RequestTypeAdd:
		if isConfidential {
			if err := checkValidContainerID(containerID, "container"); err != nil {
				return err
			}

			// We check this regardless of what the policy says, as long as we're in
			// confidential mode.  This matches with checkContainerSettings called for
			// container creation request.
			expectedContainerRootfs := path.Join(guestpath.LCOWRootPrefixInUVM, containerID, guestpath.RootfsPath)
			if cl.ContainerRootPath != expectedContainerRootfs {
				return fmt.Errorf("combined layers target %q does not match expected path %q",
					cl.ContainerRootPath, expectedContainerRootfs)
			}

			if cl.ScratchPath != "" {
				// At this point, we do not know what the sandbox ID would be yet, so we
				// have to allow anything reasonable.
				scratchDirRegexStr := fmt.Sprintf(
					"^%s/%s/%s/%s$",
					guestpath.LCOWRootPrefixInUVM,
					validContainerIDRegexRaw,
					guestpath.ScratchDir,
					containerID,
				)
				scratchDirRegex := regexp.MustCompile(scratchDirRegexStr)
				if !scratchDirRegex.MatchString(cl.ScratchPath) {
					return fmt.Errorf("scratch path %q must match regex %q",
						cl.ScratchPath, scratchDirRegexStr)
				}
			}
		}
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

			if err := securityPolicy.EnforceScratchMountPolicy(ctx, cl.ScratchPath, scratchEncrypted); err != nil {
				return fmt.Errorf("scratch mounting denied by policy: %w", err)
			}
		}

		if err := securityPolicy.EnforceOverlayMountPolicy(ctx, containerID, layerPaths, cl.ContainerRootPath); err != nil {
			return fmt.Errorf("overlay creation denied by policy: %w", err)
		}

		return overlay.MountLayer(ctx, layerPaths, upperdirPath, workdirPath, cl.ContainerRootPath, readonly)
	case guestrequest.RequestTypeRemove:
		// cl.ContainerID is not set on remove requests, but rego checks that we can
		// only umount previously mounted targets anyway
		if err := securityPolicy.EnforceOverlayUnmountPolicy(ctx, cl.ContainerRootPath); err != nil {
			return errors.Wrap(err, "overlay removal denied by policy")
		}

		return storage.UnmountPath(ctx, cl.ContainerRootPath, true)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyNetwork(ctx context.Context, rt guestrequest.RequestType, na *guestresource.LCOWNetworkAdapter) (err error) {
	switch rt {
	case guestrequest.RequestTypeAdd:
		ns := GetOrAddNetworkNamespace(na.NamespaceID)
		if err := ns.AddAdapter(ctx, na); err != nil {
			return err
		}
		// This code doesnt know if the namespace was already added to the
		// container or not so it must always call `Sync`.
		return ns.Sync(ctx)
	case guestrequest.RequestTypeRemove:
		ns, err := getNetworkNamespace(na.NamespaceID)
		if err != nil {
			log.G(ctx).WithFields(logrus.Fields{
				logfields.NamespaceID: na.NamespaceID,
				"adapterID":           na.ID,
			}).WithError(err).Warn("namespace not found for adapter removal, skipping")
			return nil
		}
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

// processOCIEnvToParam is the inverse of processParamEnvToOCIEnv
func processOCIEnvToParam(envs []string) map[string]string {
	paramEnv := make(map[string]string, len(envs))
	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		paramEnv[parts[0]] = parts[1]
	}

	return paramEnv
}

// isPrivilegedContainerCreationRequest returns if a given container
// creation request would create a privileged container
func isPrivilegedContainerCreationRequest(ctx context.Context, spec *specs.Spec) bool {
	return oci.ParseAnnotationsBool(ctx, spec.Annotations, annotations.LCOWPrivileged, false)
}

// createPodInUVM allocates a cgroup for a pod and registers it in the host.
func (h *Host) createPodInUVM(sid string, pSpec *specs.Spec, nsID string) error {
	cgroupPath := path.Join("/pods", sid)
	resources := &specs.LinuxResources{}
	if pSpec != nil && pSpec.Linux != nil && pSpec.Linux.Resources != nil {
		resources = pSpec.Linux.Resources
	}
	cgroupControl, err := cgroup.NewManager(cgroupPath, resources)
	if err != nil {
		return fmt.Errorf("failed to create cgroup for pod %s: %w", sid, err)
	}
	h.podsMutex.Lock()
	defer h.podsMutex.Unlock()
	if _, exists := h.pods[sid]; exists {
		_ = cgroupControl.Delete()
		return fmt.Errorf("pod %s already exists", sid)
	}
	h.pods[sid] = &uvmPod{
		sandboxID:        sid,
		networkNamespace: nsID,
		cgroupPath:       cgroupPath,
		cgroupControl:    cgroupControl,
		containers:       make(map[string]bool),
		createdAt:        time.Now(),
	}
	logrus.WithFields(logrus.Fields{
		"sandboxID":  sid,
		"cgroupPath": cgroupPath,
	}).Info("pod created in UVM")
	return nil
}

// addContainerToPod registers a container as belonging to a pod.
func (h *Host) addContainerToPod(sandboxID, containerID string) {
	h.podsMutex.Lock()
	defer h.podsMutex.Unlock()
	if pod, exists := h.pods[sandboxID]; exists {
		pod.containers[containerID] = true
	}
}
