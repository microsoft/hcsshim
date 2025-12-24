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
	"sync/atomic"
	"syscall"
	"time"

	cgroups "github.com/containerd/cgroups/v3/cgroup1"
	cgroup1stats "github.com/containerd/cgroups/v3/cgroup1/stats"
	"github.com/mattn/go-shellwords"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	"github.com/Microsoft/hcsshim/internal/debug"
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
	"github.com/Microsoft/hcsshim/internal/oc"
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
	if id == UVMContainerID {
		return nil
	}

	if !validContainerIDRegex.MatchString(id) {
		return errors.Errorf("invalid %s id: %s (must match %s)", idType, id, validContainerIDRegex.String())
	}

	return nil
}

// VirtualPod represents a virtual pod that shares a UVM/Sandbox with other pods
type VirtualPod struct {
	VirtualSandboxID string
	MasterSandboxID  string
	NetworkNamespace string
	CgroupPath       string
	CgroupControl    cgroups.Cgroup
	Containers       map[string]bool // containerID -> exists
	CreatedAt        time.Time
}

// Host is the structure tracking all UVM host state including all containers
// and processes.
type Host struct {
	containersMutex sync.Mutex
	containers      map[string]*Container

	externalProcessesMutex sync.Mutex
	externalProcesses      map[int]*externalProcess

	// Virtual pod support for multi-pod scenarios
	virtualPodsMutex        sync.Mutex
	virtualPods             map[string]*VirtualPod // virtualSandboxID -> VirtualPod
	containerToVirtualPod   map[string]string      // containerID -> virtualSandboxID
	virtualPodsCgroupParent cgroups.Cgroup         // Parent cgroup for all virtual pods

	rtime            runtime.Runtime
	vsock            transport.Transport
	devNullTransport transport.Transport

	// state required for the security policy enforcement
	securityOptions *securitypolicy.SecurityOptions

	// hostMounts keeps the state of currently mounted devices and file systems,
	// which is used for GCS hardening.  It is only used for confidential
	// containers, and is initialized in SetConfidentialUVMOptions.  If this is
	// nil, we do not do add any special restrictions on mounts / unmounts.
	hostMounts *hostMounts
	// A permanent flag to indicate that further mounts, unmounts and container
	// creation should not be allowed.  This is set when, because of a failure
	// during an unmount operation, we end up in a state where the policy
	// enforcer's state is out of sync with what we have actually done, but we
	// cannot safely revert its state.
	//
	// Not used in non-confidential mode.
	mountsBroken atomic.Bool
	// A user-friendly error message for why mountsBroken was set.
	mountsBrokenCausedBy string
}

func NewHost(rtime runtime.Runtime, vsock transport.Transport, initialEnforcer securitypolicy.SecurityPolicyEnforcer, logWriter io.Writer) *Host {
	securityPolicyOptions := securitypolicy.NewSecurityOptions(
		initialEnforcer,
		false,
		"",
		logWriter,
	)
	return &Host{
		containers:            make(map[string]*Container),
		externalProcesses:     make(map[int]*externalProcess),
		virtualPods:           make(map[string]*VirtualPod),
		containerToVirtualPod: make(map[string]string),
		rtime:                 rtime,
		vsock:                 vsock,
		devNullTransport:      &transport.DevNullTransport{},
		hostMounts:            nil,
		securityOptions:       securityPolicyOptions,
		mountsBroken:          atomic.Bool{},
	}
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

	// Check if this container is part of a virtual pod
	virtualPodID, isVirtualPod := c.spec.Annotations[annotations.VirtualPodID]
	if isVirtualPod {
		// Remove from virtual pod tracking
		h.RemoveContainerFromVirtualPod(id)
		// Network namespace cleanup is handled in virtual pod cleanup when last container is removed.
		logrus.WithFields(logrus.Fields{
			"containerID":  id,
			"virtualPodID": virtualPodID,
		}).Info("Container removed from virtual pod")
	} else {
		// delete the network namespace for standalone and sandbox containers
		criType, isCRI := c.spec.Annotations[annotations.KubernetesContainerType]
		if !isCRI || criType == "sandbox" {
			_ = RemoveNetworkNamespace(context.Background(), id)
		}
	}

	delete(h.containers, id)
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

func setupSandboxMountsPath(id string) (err error) {
	mountPath := specGuest.SandboxMountsDir(id)
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

func setupSandboxTmpfsMountsPath(id string) (err error) {
	tmpfsDir := specGuest.SandboxTmpfsMountsDir(id)
	if err := os.MkdirAll(tmpfsDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create sandbox tmpfs mounts dir in sandbox %v", id)
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

func setupSandboxHugePageMountsPath(id string) error {
	mountPath := specGuest.HugePagesMountsDir(id)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create hugepage Mounts dir in sandbox %v", id)
	}

	return storage.MountRShared(mountPath)
}

// setupSandboxLogDir creates the directory to house all redirected stdio logs from containers.
//
// Virtual pod aware.
func setupSandboxLogDir(sandboxID, virtualSandboxID string) error {
	mountPath := specGuest.SandboxLogsDir(sandboxID, virtualSandboxID)
	if err := mkdirAllModePerm(mountPath); err != nil {
		id := sandboxID
		if virtualSandboxID != "" {
			id = virtualSandboxID
		}
		return errors.Wrapf(err, "failed to create sandbox logs dir in sandbox %v", id)
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

// Returns an error if h.mountsBroken is set (and we're in a confidential
// container host)
func (h *Host) checkMountsNotBroken() error {
	if h.HasSecurityPolicy() && h.mountsBroken.Load() {
		return errors.Errorf(
			"Mount, unmount, container creation and deletion has been disabled in this UVM due to a previous error (%q)",
			h.mountsBrokenCausedBy,
		)
	}
	return nil
}

func (h *Host) setMountsBrokenIfConfidential(cause string) {
	if !h.HasSecurityPolicy() {
		return
	}
	h.mountsBroken.Store(true)
	h.mountsBrokenCausedBy = cause
	log.G(context.Background()).WithFields(logrus.Fields{
		"cause": cause,
	}).Error("Host::mountsBroken set to true. All further mounts/unmounts, container creation and deletion will fail.")
}

func checkExists(path string) (error, bool) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, false
		}
		return errors.Wrapf(err, "failed to determine if path '%s' exists", path), false
	}
	return nil, true
}

func (h *Host) CreateContainer(ctx context.Context, id string, settings *prot.VMHostedContainerSettingsV2) (_ *Container, err error) {
	if err = h.checkMountsNotBroken(); err != nil {
		return nil, err
	}

	criType, isCRI := settings.OCISpecification.Annotations[annotations.KubernetesContainerType]

	// Check for virtual pod annotation
	virtualPodID, isVirtualPod := settings.OCISpecification.Annotations[annotations.VirtualPodID]

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
			"containerID":     id,
			"virtualPodID":    virtualPodID,
			"originalCriType": settings.OCISpecification.Annotations[annotations.KubernetesContainerType],
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
		terminated:     atomic.Bool{},
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

	// Handle virtual pod logic
	if isVirtualPod && isCRI {
		logrus.WithFields(logrus.Fields{
			"containerID":  id,
			"virtualPodID": virtualPodID,
			"criType":      criType,
		}).Info("Processing container for virtual pod")

		if criType == "sandbox" {
			// This is a virtual pod sandbox - create the virtual pod if it doesn't exist
			if _, exists := h.GetVirtualPod(virtualPodID); !exists {
				// Use the network namespace ID from the current container spec
				// Virtual pods share the same network namespace
				networkNamespace := specGuest.GetNetworkNamespaceID(settings.OCISpecification)
				if networkNamespace == "" {
					networkNamespace = fmt.Sprintf("virtual-pod-%s", virtualPodID)
				}

				// Extract memory limit from sandbox container spec
				var memoryLimit *int64
				if settings.OCISpecification.Linux != nil &&
					settings.OCISpecification.Linux.Resources != nil &&
					settings.OCISpecification.Linux.Resources.Memory != nil &&
					settings.OCISpecification.Linux.Resources.Memory.Limit != nil {
					memoryLimit = settings.OCISpecification.Linux.Resources.Memory.Limit
					logrus.WithFields(logrus.Fields{
						"containerID":  id,
						"virtualPodID": virtualPodID,
						"memoryLimit":  *memoryLimit,
					}).Info("Extracted memory limit from sandbox container spec")
				} else {
					logrus.WithFields(logrus.Fields{
						"containerID":  id,
						"virtualPodID": virtualPodID,
					}).Info("No memory limit found in sandbox container spec")
				}

				if err := h.CreateVirtualPod(ctx, virtualPodID, virtualPodID, networkNamespace, memoryLimit); err != nil {
					return nil, errors.Wrapf(err, "failed to create virtual pod %s", virtualPodID)
				}
			}
		}

		// Add this container to the virtual pod
		if err := h.AddContainerToVirtualPod(id, virtualPodID); err != nil {
			return nil, errors.Wrapf(err, "failed to add container %s to virtual pod %s", id, virtualPodID)
		}
	}

	// Normally we would be doing policy checking here at the start of our
	// "policy gated function". However, we can't for create container as we
	// need a properly correct sandboxID which might be changed by the code
	// below that determines the sandboxID. This is a bit of future proofing
	// as currently for our single use case, the sandboxID is the same as the
	// container id

	var namespaceID string
	// for sandbox container sandboxID is same as container id
	sandboxID := id
	if isCRI {
		switch criType {
		case "sandbox":
			// Capture namespaceID if any because setupSandboxContainerSpec clears the Windows section.
			namespaceID = specGuest.GetNetworkNamespaceID(settings.OCISpecification)

			err = setupSandboxContainerSpec(ctx, id, settings.OCISpecification)
			if err != nil {
				return nil, err
			}
			defer func() {
				if err != nil {
					_ = os.RemoveAll(settings.OCIBundlePath)
				}
			}()

			if isVirtualPod {
				// For virtual pods, create virtual pod specific paths
				if err = setupVirtualPodMountsPath(virtualPodID); err != nil {
					return nil, err
				}
				if err = setupVirtualPodTmpfsMountsPath(virtualPodID); err != nil {
					return nil, err
				}
				if err = setupVirtualPodHugePageMountsPath(virtualPodID); err != nil {
					return nil, err
				}
			} else {
				// Traditional sandbox setup
				if err = setupSandboxMountsPath(id); err != nil {
					return nil, err
				}
				if err = setupSandboxTmpfsMountsPath(id); err != nil {
					return nil, err
				}
				if err = setupSandboxHugePageMountsPath(id); err != nil {
					return nil, err
				}
			}
			if err = setupSandboxLogDir(id, virtualPodID); err != nil {
				return nil, err
			}

			if err := securitypolicy.ExtendPolicyWithNetworkingMounts(id, h.securityOptions.PolicyEnforcer, settings.OCISpecification); err != nil {
				return nil, err
			}
		case "container":
			sid, ok := settings.OCISpecification.Annotations[annotations.KubernetesSandboxID]
			sandboxID = sid
			if h.HasSecurityPolicy() {
				if err = checkValidContainerID(sid, "sandbox"); err != nil {
					return nil, err
				}
			}
			if !ok || sid == "" {
				return nil, errors.Errorf("unsupported 'io.kubernetes.cri.sandbox-id': '%s'", sid)
			}
			if err = setupWorkloadContainerSpec(ctx, sid, id, settings.OCISpecification, settings.OCIBundlePath); err != nil {
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
		if err := setupStandaloneContainerSpec(ctx, id, settings.OCISpecification); err != nil {
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

	// don't specialize tee logs (both files and mounts) just for workload containers
	// add log directory mount before enforcing (mount) policy
	if logDirMount := settings.OCISpecification.Annotations[annotations.LCOWTeeLogDirMount]; logDirMount != "" {
		settings.OCISpecification.Mounts = append(settings.OCISpecification.Mounts, specs.Mount{
			Destination: logDirMount,
			Type:        "bind",
			Source:      specGuest.SandboxLogsDir(sandboxID, virtualPodID),
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

		c.logPath = specGuest.SandboxLogPath(sandboxID, virtualPodID, logPath)
		// verify the logpath is still under the correct directory
		if !strings.HasPrefix(c.logPath, specGuest.SandboxLogsDir(sandboxID, virtualPodID)) {
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

// Returns whether there is a running container that is currently using the
// given overlay (as its rootfs).
func (h *Host) IsOverlayInUse(overlayPath string) bool {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	for _, c := range h.containers {
		if c.terminated.Load() {
			continue
		}

		if c.spec.Root.Path == overlayPath {
			return true
		}
	}

	return false
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
		if err := h.checkMountsNotBroken(); err != nil {
			return err
		}

		mvd := req.Settings.(*guestresource.LCOWMappedVirtualDisk)
		// find the actual controller number on the bus and update the incoming request.
		var cNum uint8
		cNum, err := scsi.ActualControllerNumber(ctx, mvd.Controller)
		if err != nil {
			return err
		}
		mvd.Controller = cNum
		return h.modifyMappedVirtualDisk(ctx, req.RequestType, mvd)
	case guestresource.ResourceTypeMappedDirectory:
		if err := h.checkMountsNotBroken(); err != nil {
			return err
		}

		return h.modifyMappedDirectory(ctx, h.vsock, req.RequestType, req.Settings.(*guestresource.LCOWMappedDirectory))
	case guestresource.ResourceTypeVPMemDevice:
		if err := h.checkMountsNotBroken(); err != nil {
			return err
		}

		return h.modifyMappedVPMemDevice(ctx, req.RequestType, req.Settings.(*guestresource.LCOWMappedVPMemDevice))
	case guestresource.ResourceTypeCombinedLayers:
		if err := h.checkMountsNotBroken(); err != nil {
			return err
		}

		return h.modifyCombinedLayers(ctx, req.RequestType, req.Settings.(*guestresource.LCOWCombinedLayers))
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
		err := h.securityOptions.SetConfidentialOptions(ctx,
			r.EnforcerType,
			r.EncodedSecurityPolicy,
			r.EncodedUVMReference)
		if err != nil {
			return err
		}

		// Start tracking mounts and restricting unmounts on confidential containers.
		// As long as we started off with the ClosedDoorSecurityPolicyEnforcer, no
		// mounts should have been allowed until this point.
		if h.HasSecurityPolicy() {
			log.G(ctx).Debug("hostMounts initialized")
			h.hostMounts = newHostMounts()
		}
		return nil
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

func (h *Host) modifyMappedVirtualDisk(
	ctx context.Context,
	rt guestrequest.RequestType,
	mvd *guestresource.LCOWMappedVirtualDisk,
) (err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Host::modifyMappedVirtualDisk")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("requestType", string(rt)),
		trace.BoolAttribute("hasHostMounts", h.hostMounts != nil),
		trace.Int64Attribute("controller", int64(mvd.Controller)),
		trace.Int64Attribute("lun", int64(mvd.Lun)),
		trace.Int64Attribute("partition", int64(mvd.Partition)),
		trace.BoolAttribute("readOnly", mvd.ReadOnly),
		trace.StringAttribute("mountPath", mvd.MountPath),
	)

	var verityInfo *guestresource.DeviceVerityInfo
	securityPolicy := h.securityOptions.PolicyEnforcer
	devPath, err := scsi.GetDevicePath(ctx, mvd.Controller, mvd.Lun, mvd.Partition)
	if err != nil {
		return err
	}
	span.AddAttributes(trace.StringAttribute("devicePath", devPath))

	if mvd.ReadOnly {
		// The only time the policy is empty, and we want it to be empty
		// is when no policy is provided, and we default to open door
		// policy. In any other case, e.g. explicit open door or any
		// other rego policy we would like to mount layers with verity.
		if h.HasSecurityPolicy() {
			verityInfo, err = verity.ReadVeritySuperBlock(ctx, devPath)
			if err != nil {
				return err
			}
			if mvd.Filesystem != "" && mvd.Filesystem != "ext4" {
				return errors.Errorf("filesystem must be ext4 for read-only scsi mounts")
			}
		}
	}

	// For confidential containers, we revert the policy metadata on both mount
	// and unmount errors, but if we've actually called Unmount and it fails we
	// permanently block further device operations.
	var rev securitypolicy.RevertableSectionHandle
	rev, err = securityPolicy.StartRevertableSection()
	if err != nil {
		return errors.Wrapf(err, "failed to start revertable section on security policy enforcer")
	}
	defer h.commitOrRollbackPolicyRevSection(ctx, rev, &err)

	switch rt {
	case guestrequest.RequestTypeAdd:
		mountCtx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if mvd.MountPath != "" {
			if h.HasSecurityPolicy() {
				// The only option we allow if there is policy enforcement is
				// "ro", and it must match the readonly field in the request.
				mountOptionHasRo := false
				for _, opt := range mvd.Options {
					if opt == "ro" {
						mountOptionHasRo = true
						continue
					}
					return errors.Errorf("mounting scsi device controller %d lun %d onto %s: mount option %q denied by policy", mvd.Controller, mvd.Lun, mvd.MountPath, opt)
				}
				if mvd.ReadOnly != mountOptionHasRo {
					return errors.Errorf(
						"mounting scsi device controller %d lun %d onto %s with mount option %q failed due to mount option mismatch: mvd.ReadOnly=%t but mountOptionHasRo=%t",
						mvd.Controller, mvd.Lun, mvd.MountPath, strings.Join(mvd.Options, ","), mvd.ReadOnly, mountOptionHasRo,
					)
				}
			}
			if mvd.ReadOnly {
				var deviceHash string
				if verityInfo != nil {
					deviceHash = verityInfo.RootDigest
				}
				err = securityPolicy.EnforceDeviceMountPolicy(ctx, mvd.MountPath, deviceHash)
				if err != nil {
					return errors.Wrapf(err, "mounting scsi device controller %d lun %d onto %s denied by policy", mvd.Controller, mvd.Lun, mvd.MountPath)
				}
				if h.hostMounts != nil {
					h.hostMounts.Lock()
					defer h.hostMounts.Unlock()

					err = h.hostMounts.AddRODevice(mvd.MountPath, devPath)
					if err != nil {
						return err
					}
					// Note: "When a function returns, its deferred calls are
					// executed in last-in-first-out order." - so we are safe to
					// call RemoveRODevice in this defer.
					defer func() {
						if err != nil {
							_ = h.hostMounts.RemoveRODevice(mvd.MountPath, devPath)
						}
					}()
				}
			} else {
				err = securityPolicy.EnforceRWDeviceMountPolicy(ctx, mvd.MountPath, mvd.Encrypted, mvd.EnsureFilesystem, mvd.Filesystem)
				if err != nil {
					return errors.Wrapf(err, "mounting scsi device controller %d lun %d onto %s denied by policy", mvd.Controller, mvd.Lun, mvd.MountPath)
				}
				if h.hostMounts != nil {
					h.hostMounts.Lock()
					defer h.hostMounts.Unlock()

					err = h.hostMounts.AddRWDevice(mvd.MountPath, devPath, mvd.Encrypted)
					if err != nil {
						return err
					}
					defer func() {
						if err != nil {
							_ = h.hostMounts.RemoveRWDevice(mvd.MountPath, devPath, mvd.Encrypted)
						}
					}()
				}
			}
			config := &scsi.Config{
				Encrypted:        mvd.Encrypted,
				VerityInfo:       verityInfo,
				EnsureFilesystem: mvd.EnsureFilesystem,
				Filesystem:       mvd.Filesystem,
				BlockDev:         mvd.BlockDev,
			}
			// Since we're rolling back the policy metadata (via the revertable
			// section) on failure, we need to ensure that we have reverted all
			// the side effects from this failed mount attempt, otherwise the
			// Rego metadata is technically still inconsistent with reality.
			// Mount cleans up the created directory and dm devices on failure,
			// so we're good.
			return scsi.Mount(mountCtx, mvd.Controller, mvd.Lun, mvd.Partition, mvd.MountPath,
				mvd.ReadOnly, mvd.Options, config)
		}
		return nil
	case guestrequest.RequestTypeRemove:
		if mvd.MountPath != "" {
			if mvd.ReadOnly {
				if err = securityPolicy.EnforceDeviceUnmountPolicy(ctx, mvd.MountPath); err != nil {
					return fmt.Errorf("unmounting scsi device at %s denied by policy: %w", mvd.MountPath, err)
				}
				if h.hostMounts != nil {
					h.hostMounts.Lock()
					defer h.hostMounts.Unlock()

					if err = h.hostMounts.RemoveRODevice(mvd.MountPath, devPath); err != nil {
						return err
					}
					defer func() {
						if err != nil {
							_ = h.hostMounts.AddRODevice(mvd.MountPath, devPath)
						}
					}()
				}
			} else {
				if err = securityPolicy.EnforceRWDeviceUnmountPolicy(ctx, mvd.MountPath); err != nil {
					return fmt.Errorf("unmounting scsi device at %s denied by policy: %w", mvd.MountPath, err)
				}
				if h.hostMounts != nil {
					h.hostMounts.Lock()
					defer h.hostMounts.Unlock()

					if err = h.hostMounts.RemoveRWDevice(mvd.MountPath, devPath, mvd.Encrypted); err != nil {
						return err
					}
					defer func() {
						if err != nil {
							_ = h.hostMounts.AddRWDevice(mvd.MountPath, devPath, mvd.Encrypted)
						}
					}()
				}
			}
			// Check that the directory actually exists first, and if it does
			// not then we just refuse to do anything, without closing the dm
			// device or setting the mountsBroken flag.  Policy metadata is
			// still reverted to reflect the fact that we have not done
			// anything.
			//
			// Note: we should not do this check before calling the policy
			// enforcer, as otherwise we might inadvertently allow the host to
			// find out whether an arbitrary path (which may point to sensitive
			// data within a container rootfs) exists or not
			if h.HasSecurityPolicy() {
				err, exists := checkExists(mvd.MountPath)
				if err != nil {
					return err
				}
				if !exists {
					return errors.Errorf("unmounting scsi device at %s failed: directory does not exist", mvd.MountPath)
				}
			}
			config := &scsi.Config{
				Encrypted:        mvd.Encrypted,
				VerityInfo:       verityInfo,
				EnsureFilesystem: mvd.EnsureFilesystem,
				Filesystem:       mvd.Filesystem,
				BlockDev:         mvd.BlockDev,
			}
			err = scsi.Unmount(ctx, mvd.Controller, mvd.Lun, mvd.Partition, mvd.MountPath, config)
			if err != nil {
				h.setMountsBrokenIfConfidential(
					fmt.Sprintf("unmounting scsi device at %s failed: %v", mvd.MountPath, err),
				)
				return err
			}
		}
		return nil
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func (h *Host) modifyMappedDirectory(
	ctx context.Context,
	vsock transport.Transport,
	rt guestrequest.RequestType,
	md *guestresource.LCOWMappedDirectory,
) (err error) {
	securityPolicy := h.securityOptions.PolicyEnforcer
	// For confidential containers, we revert the policy metadata on both mount
	// and unmount errors, but if we've actually called Unmount and it fails we
	// permanently block further device operations.
	var rev securitypolicy.RevertableSectionHandle
	rev, err = securityPolicy.StartRevertableSection()
	if err != nil {
		return errors.Wrapf(err, "failed to start revertable section on security policy enforcer")
	}
	defer h.commitOrRollbackPolicyRevSection(ctx, rev, &err)

	switch rt {
	case guestrequest.RequestTypeAdd:
		err = securityPolicy.EnforcePlan9MountPolicy(ctx, md.MountPath)
		if err != nil {
			return errors.Wrapf(err, "mounting plan9 device at %s denied by policy", md.MountPath)
		}

		if h.HasSecurityPolicy() {
			if err = plan9.ValidateShareName(md.ShareName); err != nil {
				return err
			}
		}

		// Similar to the reasoning in modifyMappedVirtualDisk, since we're
		// rolling back the policy metadata, plan9.Mount here must clean up
		// everything if it fails, which it does do.
		return plan9.Mount(ctx, vsock, md.MountPath, md.ShareName, uint32(md.Port), md.ReadOnly)
	case guestrequest.RequestTypeRemove:
		err = securityPolicy.EnforcePlan9UnmountPolicy(ctx, md.MountPath)
		if err != nil {
			return errors.Wrapf(err, "unmounting plan9 device at %s denied by policy", md.MountPath)
		}

		// Note: storage.UnmountPath is nop if path does not exist.
		err = storage.UnmountPath(ctx, md.MountPath, true)
		if err != nil {
			h.setMountsBrokenIfConfidential(
				fmt.Sprintf("unmounting plan9 device at %s failed: %v", md.MountPath, err),
			)
			return err
		}
		return nil
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func (h *Host) modifyMappedVPMemDevice(ctx context.Context,
	rt guestrequest.RequestType,
	vpd *guestresource.LCOWMappedVPMemDevice,
) (err error) {
	var verityInfo *guestresource.DeviceVerityInfo
	securityPolicy := h.securityOptions.PolicyEnforcer
	var deviceHash string
	if h.HasSecurityPolicy() {
		if vpd.MappingInfo != nil {
			return fmt.Errorf("multi mapping is not supported with verity")
		}
		verityInfo, err = verity.ReadVeritySuperBlock(ctx, pmem.GetDevicePath(vpd.DeviceNumber))
		if err != nil {
			return err
		}
		deviceHash = verityInfo.RootDigest
	}

	// For confidential containers, we revert the policy metadata on both mount
	// and unmount errors, but if we've actually called Unmount and it fails we
	// permanently block further device operations.
	var rev securitypolicy.RevertableSectionHandle
	rev, err = securityPolicy.StartRevertableSection()
	if err != nil {
		return errors.Wrapf(err, "failed to start revertable section on security policy enforcer")
	}
	defer h.commitOrRollbackPolicyRevSection(ctx, rev, &err)

	switch rt {
	case guestrequest.RequestTypeAdd:
		err = securityPolicy.EnforceDeviceMountPolicy(ctx, vpd.MountPath, deviceHash)
		if err != nil {
			return errors.Wrapf(err, "mounting pmem device %d onto %s denied by policy", vpd.DeviceNumber, vpd.MountPath)
		}

		// Similar to the reasoning in modifyMappedVirtualDisk, since we're
		// rolling back the policy metadata, pmem.Mount here must clean up
		// everything if it fails, which it does do.
		return pmem.Mount(ctx, vpd.DeviceNumber, vpd.MountPath, vpd.MappingInfo, verityInfo)
	case guestrequest.RequestTypeRemove:
		if err = securityPolicy.EnforceDeviceUnmountPolicy(ctx, vpd.MountPath); err != nil {
			return errors.Wrapf(err, "unmounting pmem device from %s denied by policy", vpd.MountPath)
		}

		// Check that the directory actually exists first, and if it does not
		// then we just refuse to do anything, without closing the dm-linear or
		// dm-verity device or setting the mountsBroken flag.
		//
		// Similar to the reasoning in modifyMappedVirtualDisk, we should not do
		// this check before calling the policy enforcer.
		if h.HasSecurityPolicy() {
			err, exists := checkExists(vpd.MountPath)
			if err != nil {
				return err
			}
			if !exists {
				return errors.Errorf("unmounting pmem device at %s failed: directory does not exist", vpd.MountPath)
			}
		}

		err = pmem.Unmount(ctx, vpd.DeviceNumber, vpd.MountPath, vpd.MappingInfo, verityInfo)
		if err != nil {
			h.setMountsBrokenIfConfidential(
				fmt.Sprintf("unmounting pmem device at %s failed: %v", vpd.MountPath, err),
			)
			return err
		}
		return nil
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

func (h *Host) modifyCombinedLayers(
	ctx context.Context,
	rt guestrequest.RequestType,
	cl *guestresource.LCOWCombinedLayers,
) (err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Host::modifyCombinedLayers")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("requestType", string(rt)),
		trace.BoolAttribute("hasHostMounts", h.hostMounts != nil),
		trace.StringAttribute("containerRootPath", cl.ContainerRootPath),
		trace.StringAttribute("scratchPath", cl.ScratchPath),
	)

	securityPolicy := h.securityOptions.PolicyEnforcer
	containerID := cl.ContainerID

	// For confidential containers, we revert the policy metadata on both mount
	// and unmount errors, but if we've actually called Unmount and it fails we
	// permanently block further device operations.
	var rev securitypolicy.RevertableSectionHandle
	rev, err = securityPolicy.StartRevertableSection()
	if err != nil {
		return errors.Wrapf(err, "failed to start revertable section on security policy enforcer")
	}
	defer h.commitOrRollbackPolicyRevSection(ctx, rev, &err)

	if h.hostMounts != nil {
		// We will need this in multiple places, let's take the lock once here.
		h.hostMounts.Lock()
		defer h.hostMounts.Unlock()
	}

	switch rt {
	case guestrequest.RequestTypeAdd:
		if h.HasSecurityPolicy() {
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
			scratchEncrypted := false
			if h.hostMounts != nil {
				scratchEncrypted = h.hostMounts.IsEncrypted(cl.ScratchPath)
			}

			if err := securityPolicy.EnforceScratchMountPolicy(ctx, cl.ScratchPath, scratchEncrypted); err != nil {
				return fmt.Errorf("scratch mounting denied by policy: %w", err)
			}
		}

		if err = securityPolicy.EnforceOverlayMountPolicy(ctx, containerID, layerPaths, cl.ContainerRootPath); err != nil {
			return fmt.Errorf("overlay creation denied by policy: %w", err)
		}
		if h.hostMounts != nil {
			if err = h.hostMounts.AddOverlay(cl.ContainerRootPath, layerPaths, cl.ScratchPath); err != nil {
				return err
			}
			defer func() {
				if err != nil {
					_, _ = h.hostMounts.RemoveOverlay(cl.ContainerRootPath)
				}
			}()
		}

		// Correctness for policy revertable section:
		// MountLayer does two things - mkdir, then mount. On mount failure, the
		// target directory is cleaned up.  Therefore we're clean in terms of
		// side effects.
		return overlay.MountLayer(ctx, layerPaths, upperdirPath, workdirPath, cl.ContainerRootPath, readonly)
	case guestrequest.RequestTypeRemove:
		// cl.ContainerID is not set on remove requests, but rego checks that we can
		// only umount previously mounted targets anyway
		if err = securityPolicy.EnforceOverlayUnmountPolicy(ctx, cl.ContainerRootPath); err != nil {
			return errors.Wrap(err, "overlay removal denied by policy")
		}

		// Check that no running container is using this overlay as its rootfs.
		if h.HasSecurityPolicy() && h.IsOverlayInUse(cl.ContainerRootPath) {
			return fmt.Errorf("overlay %q is in use by a running container", cl.ContainerRootPath)
		}

		if h.hostMounts != nil {
			var undoRemoveOverlay func()
			if undoRemoveOverlay, err = h.hostMounts.RemoveOverlay(cl.ContainerRootPath); err != nil {
				return err
			}
			defer func() {
				if err != nil && undoRemoveOverlay != nil {
					undoRemoveOverlay()
				}
			}()
		}

		// Note: storage.UnmountPath is a no-op if the path does not exist.
		err = storage.UnmountPath(ctx, cl.ContainerRootPath, true)
		if err != nil {
			h.setMountsBrokenIfConfidential(
				fmt.Sprintf("unmounting overlay at %s failed: %v", cl.ContainerRootPath, err),
			)
			return err
		}
		return nil
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
		ns := GetOrAddNetworkNamespace(na.ID)
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

// Virtual Pod Management Methods

// InitializeVirtualPodSupport sets up the parent cgroup for virtual pods
func (h *Host) InitializeVirtualPodSupport(virtualPodsCgroup cgroups.Cgroup) {
	h.virtualPodsMutex.Lock()
	defer h.virtualPodsMutex.Unlock()

	h.virtualPodsCgroupParent = virtualPodsCgroup
	logrus.Info("Virtual pod support initialized")
}

// CreateVirtualPod creates a new virtual pod with its own cgroup and network namespace
func (h *Host) CreateVirtualPod(ctx context.Context, virtualSandboxID, masterSandboxID, networkNamespace string, memoryLimit *int64) error {
	h.virtualPodsMutex.Lock()
	defer h.virtualPodsMutex.Unlock()

	// Check if virtual pod already exists
	if _, exists := h.virtualPods[virtualSandboxID]; exists {
		return fmt.Errorf("virtual pod %s already exists", virtualSandboxID)
	}

	// Create cgroup path for this virtual pod under the parent cgroup
	parentPath := ""
	if h.virtualPodsCgroupParent != nil {
		if pather, ok := h.virtualPodsCgroupParent.(interface{ Path() string }); ok {
			parentPath = pather.Path()
		} else {
			parentPath = "/containers/virtual-pods" // fallback for default behavior
		}
	} else {
		parentPath = "/containers/virtual-pods" // fallback for default behavior
	}
	cgroupPath := path.Join(parentPath, virtualSandboxID)

	// Create the cgroup for this virtual pod with memory limit if provided
	resources := &specs.LinuxResources{}
	if memoryLimit != nil {
		resources.Memory = &specs.LinuxMemory{
			Limit: memoryLimit,
		}
		logrus.WithFields(logrus.Fields{
			"virtualSandboxID": virtualSandboxID,
			"memoryLimit":      *memoryLimit,
		}).Info("Creating virtual pod with memory limit")
	} else {
		logrus.WithField("virtualSandboxID", virtualSandboxID).Info("Creating virtual pod without memory limit")
	}

	cgroupControl, err := cgroups.New(cgroups.StaticPath(cgroupPath), resources)
	if err != nil {
		return errors.Wrapf(err, "failed to create cgroup for virtual pod %s", virtualSandboxID)
	}

	// Create virtual pod structure
	virtualPod := &VirtualPod{
		VirtualSandboxID: virtualSandboxID,
		MasterSandboxID:  masterSandboxID,
		NetworkNamespace: networkNamespace,
		CgroupPath:       cgroupPath,
		CgroupControl:    cgroupControl,
		Containers:       make(map[string]bool),
		CreatedAt:        time.Now(),
	}

	h.virtualPods[virtualSandboxID] = virtualPod

	logrus.WithFields(logrus.Fields{
		"virtualSandboxID": virtualSandboxID,
		"masterSandboxID":  masterSandboxID,
		"cgroupPath":       cgroupPath,
		"networkNamespace": networkNamespace,
	}).Info("Virtual pod created successfully")

	return nil
}

// CreateVirtualPodWithoutMemoryLimit creates a virtual pod without memory limits (backward compatibility)
func (h *Host) CreateVirtualPodWithoutMemoryLimit(ctx context.Context, virtualSandboxID, masterSandboxID, networkNamespace string) error {
	return h.CreateVirtualPod(ctx, virtualSandboxID, masterSandboxID, networkNamespace, nil)
}

// GetVirtualPod retrieves a virtual pod by its virtualSandboxID
func (h *Host) GetVirtualPod(virtualSandboxID string) (*VirtualPod, bool) {
	h.virtualPodsMutex.Lock()
	defer h.virtualPodsMutex.Unlock()

	vp, exists := h.virtualPods[virtualSandboxID]
	return vp, exists
}

// AddContainerToVirtualPod associates a container with a virtual pod
func (h *Host) AddContainerToVirtualPod(containerID, virtualSandboxID string) error {
	h.virtualPodsMutex.Lock()
	defer h.virtualPodsMutex.Unlock()

	// Check if virtual pod exists
	vp, exists := h.virtualPods[virtualSandboxID]
	if !exists {
		return fmt.Errorf("virtual pod %s does not exist", virtualSandboxID)
	}

	// Add container to virtual pod
	vp.Containers[containerID] = true
	h.containerToVirtualPod[containerID] = virtualSandboxID

	logrus.WithFields(logrus.Fields{
		"containerID":      containerID,
		"virtualSandboxID": virtualSandboxID,
	}).Info("Container added to virtual pod")

	return nil
}

// RemoveContainerFromVirtualPod removes a container from a virtual pod
func (h *Host) RemoveContainerFromVirtualPod(containerID string) {
	h.virtualPodsMutex.Lock()
	defer h.virtualPodsMutex.Unlock()

	virtualSandboxID, exists := h.containerToVirtualPod[containerID]
	if !exists {
		return // Container not in any virtual pod
	}

	// Remove from virtual pod
	if vp, vpExists := h.virtualPods[virtualSandboxID]; vpExists {
		delete(vp.Containers, containerID)

		// If this is the sandbox container, delete the network namespace
		if containerID == virtualSandboxID && vp.NetworkNamespace != "" {
			if err := RemoveNetworkNamespace(context.Background(), vp.NetworkNamespace); err != nil {
				logrus.WithError(err).WithField("virtualSandboxID", virtualSandboxID).
					Warn("Failed to remove virtual pod network namespace (sandbox container removal)")
			}
		}

		// If this was the last container, cleanup the virtual pod
		if len(vp.Containers) == 0 {
			h.cleanupVirtualPod(virtualSandboxID)
		}
	}

	delete(h.containerToVirtualPod, containerID)

	logrus.WithFields(logrus.Fields{
		"containerID":      containerID,
		"virtualSandboxID": virtualSandboxID,
	}).Info("Container removed from virtual pod")
}

// cleanupVirtualPod removes a virtual pod and its cgroup (should be called with mutex held)
func (h *Host) cleanupVirtualPod(virtualSandboxID string) {
	if vp, exists := h.virtualPods[virtualSandboxID]; exists {
		// Delete the cgroup
		if err := vp.CgroupControl.Delete(); err != nil {
			logrus.WithError(err).WithField("virtualSandboxID", virtualSandboxID).
				Warn("Failed to delete virtual pod cgroup")
		}

		// Clean up network namespace if this is the last virtual pod using it
		// Only remove if this virtual pod was managing the network namespace
		if vp.NetworkNamespace != "" {
			// For virtual pods, the network namespace is shared, so we only clean it up
			// when the virtual pod itself is being destroyed
			if err := RemoveNetworkNamespace(context.Background(), vp.NetworkNamespace); err != nil {
				logrus.WithError(err).WithField("virtualSandboxID", virtualSandboxID).
					Warn("Failed to remove virtual pod network namespace")
			}
		}

		delete(h.virtualPods, virtualSandboxID)

		logrus.WithField("virtualSandboxID", virtualSandboxID).Info("Virtual pod cleaned up")
	}
}

// setupVirtualPodMountsPath creates mount directories for virtual pods
func setupVirtualPodMountsPath(virtualSandboxID string) (err error) {
	// Create virtual pod specific mount path using the new path generation functions
	mountPath := specGuest.VirtualPodMountsDir(virtualSandboxID)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create virtual pod mounts dir in sandbox %v", virtualSandboxID)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(mountPath)
		}
	}()

	return storage.MountRShared(mountPath)
}

func setupVirtualPodTmpfsMountsPath(virtualSandboxID string) (err error) {
	tmpfsDir := specGuest.VirtualPodTmpfsMountsDir(virtualSandboxID)
	if err := os.MkdirAll(tmpfsDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create virtual pod tmpfs mounts dir in sandbox %v", virtualSandboxID)
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

	return storage.MountRShared(tmpfsDir)
}

func setupVirtualPodHugePageMountsPath(virtualSandboxID string) error {
	mountPath := specGuest.VirtualPodHugePagesMountsDir(virtualSandboxID)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create virtual pod hugepage mounts dir %v", virtualSandboxID)
	}

	return storage.MountRShared(mountPath)
}

// If *err is not nil, the section is rolled back, otherwise it is committed.
func (h *Host) commitOrRollbackPolicyRevSection(
	ctx context.Context,
	rev securitypolicy.RevertableSectionHandle,
	err *error,
) {
	if !h.HasSecurityPolicy() {
		// Don't produce bogus log entries if we aren't in confidential mode,
		// even though rev.Rollback would have been no-op.
		return
	}
	if *err != nil {
		rev.Rollback()
		logrus.WithContext(ctx).WithError(*err).Warn("rolling back security policy revertable section due to error")
	} else {
		rev.Commit()
	}
}

func (h *Host) DeleteContainerState(ctx context.Context, containerID string) error {
	if h.HasSecurityPolicy() {
		if err := checkValidContainerID(containerID, "container"); err != nil {
			return err
		}
	}

	if err := h.checkMountsNotBroken(); err != nil {
		return err
	}

	c, err := h.GetCreatedContainer(containerID)
	if err != nil {
		return err
	}
	if h.HasSecurityPolicy() {
		if !c.terminated.Load() {
			return errors.Errorf("Denied deleting state of a running container %q", containerID)
		}
		overlay := c.spec.Root.Path
		h.hostMounts.Lock()
		defer h.hostMounts.Unlock()
		if h.hostMounts.HasOverlayMountedAt(overlay) {
			return errors.Errorf("Denied deleting state of a container with a overlay mount still active")
		}
	}

	// remove container state regardless of delete's success
	defer h.RemoveContainer(containerID)

	if err = c.Delete(ctx); err != nil {
		return err
	}

	return nil
}
