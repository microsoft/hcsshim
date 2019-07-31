// +build linux

package hcsv2

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/Microsoft/opengcs/internal/storage"
	"github.com/Microsoft/opengcs/internal/storage/overlay"
	"github.com/Microsoft/opengcs/internal/storage/plan9"
	"github.com/Microsoft/opengcs/internal/storage/pmem"
	"github.com/Microsoft/opengcs/internal/storage/scsi"
	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/Microsoft/opengcs/service/gcs/transport"
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

	// Rtime is the Runtime interface used by the GCS core.
	rtime runtime.Runtime
	vsock transport.Transport
}

func NewHost(rtime runtime.Runtime, vsock transport.Transport) *Host {
	return &Host{
		containers: make(map[string]*Container),
		rtime:      rtime,
		vsock:      vsock,
	}
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
		case "container":
			sid, ok := settings.OCISpecification.Annotations["io.kubernetes.cri.sandbox-id"]
			if !ok || sid == "" {
				return nil, errors.Errorf("unsupported 'io.kubernetes.cri.sandbox-id': '%s'", sid)
			}
			err = setupWorkloadContainerSpec(ctx, sid, id, settings.OCISpecification)
			defer func() {
				if err != nil {
					defer os.RemoveAll(getWorkloadRootDir(sid, id))
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
		container: con,
		exitType:  prot.NtUnexpectedExit,
		processes: make(map[uint32]*Process),
	}
	// Add the WG count for the init process
	c.processesWg.Add(1)
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

func (h *Host) ModifyHostSettings(ctx context.Context, settings *prot.ModifySettingRequest) error {
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
	default:
		return errors.Errorf("the ResourceType \"%s\" is not supported", settings.ResourceType)
	}
}

// Shutdown terminates this UVM. This is a destructive call and will destroy all
// state that has not been cleaned before calling this function.
func (h *Host) Shutdown() {
	syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
}

func newInvalidRequestTypeError(rt prot.ModifyRequestType) error {
	return errors.Errorf("the RequestType \"%s\" is not supported", rt)
}

func modifyMappedVirtualDisk(ctx context.Context, rt prot.ModifyRequestType, mvd *prot.MappedVirtualDiskV2) (err error) {
	switch rt {
	case prot.MreqtAdd:
		if mvd.MountPath != "" {
			return scsi.Mount(ctx, mvd.Controller, mvd.Lun, mvd.MountPath, mvd.ReadOnly)
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
		return pmem.Mount(ctx, vpd.DeviceNumber, vpd.MountPath)
	case prot.MreqtRemove:
		return storage.UnmountPath(ctx, vpd.MountPath, true)
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
