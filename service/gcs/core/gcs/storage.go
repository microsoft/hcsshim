package gcs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/sirupsen/logrus"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

const (
	// baseFilesPath is the path in the utility VM containing all the files
	// that will be used as the base layer for containers.
	baseFilesPath = "/tmp/base/"

	// deviceLookupTimeout is the amount of time before deviceIDToName will
	// give up trying to look up the device name from its ID.
	deviceLookupTimeout = time.Second * 2

	// mappedDiskMountTimeout is the amount of time before
	// mountMappedVirtualDisks will give up trying to mount a device.
	mappedDiskMountTimeout = time.Second * 2
)

type mountSpec struct {
	Source     string
	FileSystem string
	Flags      uintptr
	Options    []string
}

const (
	// From mount(8): Don't load the journal on mounting.  Note that if the
	// filesystem was not unmounted cleanly, skipping the journal replay will
	// lead to the filesystem containing inconsistencies that can lead to any
	// number of problems.
	mountOptionNoLoad = "noload"
	// Enable DAX mode. This turns off the local cache for the file system and
	// accesses the storage directly from host memory, reducing memory use
	// and increasing sharing across VMs. Only supported on vPMEM devices.
	mountOptionDax = "dax"

	// For now the file system is hard-coded
	defaultFileSystem = "ext4"
)

// Mount mounts the file system to the specified target.
func (ms *mountSpec) Mount(osl oslayer.OS, target string) error {
	options := strings.Join(ms.Options, ",")
	err := osl.Mount(ms.Source, target, ms.FileSystem, ms.Flags, options)
	if err != nil {
		return errors.Wrapf(err, "mount %s %s %s 0x%x %s", ms.Source, target, ms.FileSystem, ms.Flags, options)
	}
	return nil
}

// getLayerMounts computes the mount specs for the scratch and layers.
func (c *gcsCore) getLayerMounts(scratch string, layers []prot.Layer) (scratchMount *mountSpec, layerMounts []*mountSpec, err error) {
	layerMounts = make([]*mountSpec, len(layers))
	for i, layer := range layers {
		deviceName, pmem, err := deviceIDToName(c.OS, layer.Path)
		if err != nil {
			return nil, nil, err
		}
		options := []string{mountOptionNoLoad}
		if pmem {
			// PMEM devices support DAX and should use it
			options = append(options, mountOptionDax)
		}
		layerMounts[i] = &mountSpec{
			Source:     deviceName,
			FileSystem: defaultFileSystem,
			Flags:      syscall.MS_RDONLY,
			Options:    options,
		}
	}
	// An empty scratch value indicates no scratch space is to be attached.
	if scratch != "" {
		scratchDevice, _, err := deviceIDToName(c.OS, scratch)
		if err != nil {
			return nil, nil, err
		}
		scratchMount = &mountSpec{
			Source:     scratchDevice,
			FileSystem: defaultFileSystem,
		}
	}

	return scratchMount, layerMounts, nil
}

// getMappedVirtualDiskMounts uses the Lun values in the given disks to
// retrieve their associated mount spec.
func (c *gcsCore) getMappedVirtualDiskMounts(disks []prot.MappedVirtualDisk) ([]*mountSpec, error) {
	devices := make([]*mountSpec, len(disks))
	for i, disk := range disks {
		device, err := scsiLunToName(c.OS, disk.Lun)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get device name for mapped virtual disk %s, lun %d", disk.ContainerPath, disk.Lun)
		}
		flags := uintptr(0)
		var options []string
		if disk.ReadOnly {
			flags |= syscall.MS_RDONLY
			options = append(options, mountOptionNoLoad)
		}
		devices[i] = &mountSpec{
			Source:     device,
			FileSystem: defaultFileSystem,
			Flags:      flags,
			Options:    options,
		}
	}
	return devices, nil
}

// scsiLunToName finds the SCSI device with the given LUN. This assumes
// only one SCSI controller.
func scsiLunToName(osl oslayer.OS, lun uint8) (string, error) {
	scsiID := fmt.Sprintf("0:0:0:%d", lun)

	// Query for the device name up until the timeout.
	var deviceNames []os.FileInfo
	startTime := time.Now()
	for {
		// Devices matching the given SCSI code should each have a subdirectory
		// under /sys/bus/scsi/devices/<scsiID>/block.
		var err error
		deviceNames, err = osl.ReadDir(filepath.Join("/sys/bus/scsi/devices", scsiID, "block"))
		if err != nil {
			currentTime := time.Now()
			elapsedTime := currentTime.Sub(startTime)
			if elapsedTime > deviceLookupTimeout {
				return "", errors.Wrap(err, "failed to retrieve SCSI device names from filesystem")
			}
		} else {
			break
		}
		time.Sleep(time.Millisecond * 10)
	}

	if len(deviceNames) == 0 {
		return "", errors.Errorf("no matching device names found for SCSI ID \"%s\"", scsiID)
	}
	if len(deviceNames) > 1 {
		return "", errors.Errorf("more than one block device could match SCSI ID \"%s\"", scsiID)
	}
	return filepath.Join("/dev", deviceNames[0].Name()), nil
}

// deviceIDToName converts a device ID (scsi:<lun> or pmem:<device#> to a
// device name (/dev/sd? or /dev/pmem?).
// For temporary compatibility, this also accepts just <lun> for SCSI devices.
func deviceIDToName(osl oslayer.OS, id string) (device string, pmem bool, err error) {
	const (
		pmemPrefix = "pmem:"
		scsiPrefix = "scsi:"
	)

	if strings.HasPrefix(id, pmemPrefix) {
		return "/dev/pmem" + id[len(pmemPrefix):], true, nil
	}

	lunStr := id
	if strings.HasPrefix(id, scsiPrefix) {
		lunStr = id[len(scsiPrefix):]
	}

	if lun, err := strconv.ParseInt(lunStr, 10, 8); err == nil {
		name, err := scsiLunToName(osl, uint8(lun))
		return name, false, err
	}

	return "", false, errors.Errorf("unknown device ID %s", id)
}

// mountMappedVirtualDisks mounts the given disks to the given directories,
// with the given options. The device names of each disk are given in a
// parallel slice.
func (c *gcsCore) mountMappedVirtualDisks(disks []prot.MappedVirtualDisk, mounts []*mountSpec) error {
	if len(disks) != len(mounts) {
		return errors.Errorf("disk and device slices were of different sizes. disks: %d, mounts: %d", len(disks), len(mounts))
	}
	for i, disk := range disks {
		// Don't mount the disk if AttachOnly is specified.
		if !disk.AttachOnly {
			if !disk.CreateInUtilityVM {
				return errors.New("we do not currently support mapping virtual disks inside the container namespace")
			}
			mount := mounts[i]
			if err := c.OS.MkdirAll(disk.ContainerPath, 0700); err != nil {
				return errors.Wrapf(err, "failed to create directory for mapped virtual disk %s", disk.ContainerPath)
			}

			// Attempt mounting multiple times up until the given timout. This is
			// necessary because there is a span of time between when the device
			// name becomes available under /sys/bus/scsi and when it appears under
			// /dev. Once it appears under /dev, there is still a span of time
			// before it becomes mountable. Retrying mount should succeed in
			// mounting the device as long as it becomes mountable under /dev
			// before the timeout.
			startTime := time.Now()
			for {
				err := mount.Mount(c.OS, disk.ContainerPath)
				if err != nil {
					currentTime := time.Now()
					elapsedTime := currentTime.Sub(startTime)
					if elapsedTime > mappedDiskMountTimeout {
						return errors.Wrapf(err, "failed to mount directory %s for mapped virtual disk device %s", disk.ContainerPath, mount.Source)
					}
				} else {
					break
				}
				time.Sleep(time.Millisecond * 10)
			}
		}
	}
	return nil
}

// unmountMappedVirtualDisks unmounts the given container's mapped virtual disk
// directories.
func (c *gcsCore) unmountMappedVirtualDisks(disks []prot.MappedVirtualDisk) error {
	for _, disk := range disks {
		// If the disk was specified AttachOnly, it shouldn't have been mounted
		// in the first place.
		if !disk.AttachOnly {
			exists, err := c.OS.PathExists(disk.ContainerPath)
			if err != nil {
				return errors.Wrapf(err, "failed to determine if mapped virtual disk path exists %s", disk.ContainerPath)
			}
			mounted, err := c.OS.PathIsMounted(disk.ContainerPath)
			if err != nil {
				return errors.Wrapf(err, "failed to determine if mapped virtual disk path is mounted %s", disk.ContainerPath)
			}
			if exists && mounted {
				if err := c.OS.Unmount(disk.ContainerPath, 0); err != nil {
					return errors.Wrapf(err, "failed to unmount mapped virtual disk path %s", disk.ContainerPath)
				}
			}
		}
	}
	return nil
}

// mountMappedDirectories mounts the given mapped directories using a Plan9
// filesystem with the given options.
func (c *gcsCore) mountMappedDirectories(dirs []prot.MappedDirectory) error {
	for _, dir := range dirs {
		if !dir.CreateInUtilityVM {
			return errors.New("we do not currently support mapping directories inside the container namespace")
		}
		if err := c.OS.MkdirAll(dir.ContainerPath, 0700); err != nil {
			return errors.Wrapf(err, "failed to create directory for mapped directory %s", dir.ContainerPath)
		}
		var mountOptions uintptr = 0
		var data string = fmt.Sprintf("trans=vsock,port=%d", dir.Port)
		if dir.ReadOnly {
			mountOptions |= syscall.MS_RDONLY
			data += ",noload"
		}
		if err := c.OS.Mount(dir.ContainerPath, dir.ContainerPath, "9p", mountOptions, data); err != nil {
			return errors.Wrapf(err, "failed to mount directory for mapped directory %s", dir.ContainerPath)
		}
	}
	return nil
}

// unmountMappedDirectories unmounts the given container's mapped directories.
func (c *gcsCore) unmountMappedDirectories(dirs []prot.MappedDirectory) error {
	for _, dir := range dirs {
		exists, err := c.OS.PathExists(dir.ContainerPath)
		if err != nil {
			return errors.Wrapf(err, "failed to determine if mapped directory path exists %s", dir.ContainerPath)
		}
		mounted, err := c.OS.PathIsMounted(dir.ContainerPath)
		if err != nil {
			return errors.Wrapf(err, "failed to determine if mapped directory path is mounted %s", dir.ContainerPath)
		}
		if exists && mounted {
			if err := c.OS.Unmount(dir.ContainerPath, 0); err != nil {
				return errors.Wrapf(err, "failed to unmount mapped directory path %s", dir.ContainerPath)
			}
		}
	}
	return nil
}

// mountLayers mounts each device into a mountpoint, and then layers them into a
// union filesystem in the given order.
// These mountpoints are all stored under a directory reserved for the container
// with the given ID.
func (c *gcsCore) mountLayers(id string, scratchMount *mountSpec, layers []*mountSpec) error {
	layerPrefix, scratchPath, workdirPath, rootfsPath := c.getUnioningPaths(id)

	logrus.Infof("layerPrefix=%s\n", layerPrefix)
	logrus.Infof("scratchPath:%s\n", scratchPath)
	logrus.Infof("workdirPath=%s\n", workdirPath)
	logrus.Infof("rootfsPath=%s\n", rootfsPath)

	// Mount the layer devices.
	layerPaths := make([]string, len(layers)+1)
	for i, layer := range layers {
		layerPath := fmt.Sprintf("%s%d", layerPrefix, i)
		logrus.Infof("layerPath: %s\n", layerPath)
		if err := c.OS.MkdirAll(layerPath, 0700); err != nil {
			return errors.Wrapf(err, "failed to create directory for layer %s", layerPath)
		}
		if err := layer.Mount(c.OS, layerPath); err != nil {
			return errors.Wrapf(err, "failed to mount layer directory %s", layerPath)
		}
		layerPaths[i] = layerPath
	}
	// TODO: The base path code may be temporary until a more permanent DNS
	// solution is reached.
	// NOTE: This should probably still always be kept, because otherwise
	// mounting will fail when no layer devices are attached. There should
	// always be at least one layer, even if it's empty, to prevent this
	// from happening.
	layerPaths[len(layerPaths)-1] = baseFilesPath

	// Mount the layers into a union filesystem.
	var mountOptions uintptr
	if err := c.OS.MkdirAll(baseFilesPath, 0700); err != nil {
		return errors.Wrapf(err, "failed to create directory for base files %s", baseFilesPath)
	}
	if err := c.OS.MkdirAll(scratchPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory for scratch space %s", scratchPath)
	}
	if scratchMount != nil {
		if err := scratchMount.Mount(c.OS, scratchPath); err != nil {
			return errors.Wrapf(err, "failed to mount scratch directory %s", scratchPath)
		}
	} else {
		// If no scratch device is attached, the overlay filesystem should be
		// readonly.
		mountOptions |= syscall.O_RDONLY
	}
	upperDir := filepath.Join(scratchPath, "upper")
	if err := c.OS.MkdirAll(upperDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create upper directory in scratch space")
	}
	if err := c.OS.MkdirAll(workdirPath, 0755); err != nil {
		return errors.Wrap(err, "failed to create workdir in scratch space")
	}
	if err := c.OS.MkdirAll(rootfsPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory for container root filesystem %s", rootfsPath)
	}
	lowerdir := strings.Join(layerPaths, ":")
	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerdir, upperDir, workdirPath)
	if err := c.OS.Mount("overlay", rootfsPath, "overlay", mountOptions, options); err != nil {
		return errors.Wrapf(err, "failed to mount container root filesystem using overlayfs %s", rootfsPath)
	}

	return nil
}

// unmountLayers unmounts the union filesystem for the container with the given
// ID, as well as any devices whose mountpoints were layers in that filesystem.
func (c *gcsCore) unmountLayers(id string) error {
	layerPrefix, scratchPath, _, rootfsPath := c.getUnioningPaths(id)

	// clean up rootfsPath operations
	exists, err := c.OS.PathExists(rootfsPath)
	if err != nil {
		return errors.Wrapf(err, "failed to determine if container root filesystem path exists %s", rootfsPath)
	}
	mounted, err := c.OS.PathIsMounted(rootfsPath)
	if err != nil {
		return errors.Wrapf(err, "failed to determine if container root filesystem path is mounted %s", rootfsPath)
	}
	if exists && mounted {
		if err := c.OS.Unmount(rootfsPath, 0); err != nil {
			return errors.Wrapf(err, "failed to unmount container root filesystem %s", rootfsPath)
		}
	}

	// clean up scratchPath operations
	exists, err = c.OS.PathExists(scratchPath)
	if err != nil {
		return errors.Wrapf(err, "failed to determine if scratch path exists %s", scratchPath)
	}
	mounted, err = c.OS.PathIsMounted(scratchPath)
	if err != nil {
		return errors.Wrapf(err, "failed to determine if scratch path is mounted %s", scratchPath)
	}
	if exists && mounted {
		if err := c.OS.Unmount(scratchPath, 0); err != nil {
			return errors.Wrapf(err, "failed to unmount scratch path %s", scratchPath)
		}
	}

	// Clean up layer path operations
	layerPaths, err := filepath.Glob(layerPrefix + "*")
	if err != nil {
		return errors.Wrap(err, "failed to get layer paths using Glob")
	}
	for _, layerPath := range layerPaths {
		exists, err = c.OS.PathExists(layerPath)
		if err != nil {
			return errors.Wrapf(err, "failed to determine if layer path exists %s", layerPath)
		}
		mounted, err = c.OS.PathIsMounted(layerPath)
		if err != nil {
			return errors.Wrapf(err, "failed to determine if layer path is mounted %s", layerPath)
		}
		if exists && mounted {
			if err := c.OS.Unmount(layerPath, 0); err != nil {
				return errors.Wrapf(err, "failed to unmount layer path %s", layerPath)
			}
		}
	}

	return nil
}

// destroyContainerStorage removes any files the GCS stores on disk for the
// container with the given ID.
// These files include directories used for mountpoints in the union filesystem
// and config files.
func (c *gcsCore) destroyContainerStorage(id string) error {
	if err := c.OS.RemoveAll(c.getContainerStoragePath(id)); err != nil {
		return errors.Wrapf(err, "failed to remove container storage path for container %s", id)
	}
	return nil
}

// writeConfigFile writes the given oci.Spec to disk so that it can be consumed
// by an OCI runtime.
func (c *gcsCore) writeConfigFile(id string, config oci.Spec) error {
	configPath := c.getConfigPath(id)
	if err := c.OS.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return errors.Wrapf(err, "failed to create config file directory for container %s", id)
	}
	configFile, err := c.OS.Create(configPath)
	if err != nil {
		return errors.Wrapf(err, "failed to create config file for container %s", id)
	}
	defer configFile.Close()
	writer := bufio.NewWriter(configFile)
	if err := json.NewEncoder(writer).Encode(config); err != nil {
		return errors.Wrapf(err, "failed to write contents of config file for container %s", id)
	}
	if err := writer.Flush(); err != nil {
		return errors.Wrapf(err, "failed to flush to config file for container %s", id)
	}
	return nil
}

func (c *gcsCore) getStorageRootPath() string {
	return "/tmp/gcs"
}

// getContainerStoragePath returns the path where the GCS stores files on disk
// for the container with the given ID.
func (c *gcsCore) getContainerStoragePath(id string) string {
	return filepath.Join(c.getStorageRootPath(), id)
}

// getUnioningPaths returns paths that will be used in the union filesystem for
// the container with the given ID.
func (c *gcsCore) getUnioningPaths(id string) (layerPrefix string, scratchPath string, workdirPath string, rootfsPath string) {
	mountPath := c.getContainerStoragePath(id)
	layerPrefix = filepath.Join(mountPath, "layer")
	scratchPath = filepath.Join(mountPath, "scratch")
	workdirPath = filepath.Join(mountPath, "scratch", "work")
	rootfsPath = filepath.Join(mountPath, "rootfs")
	return
}

// getConfigPath returns the path to the container's config file.
func (c *gcsCore) getConfigPath(id string) string {
	return filepath.Join(c.getContainerStoragePath(id), "config.json")
}
