//go:build windows

package hcsoci

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"

	"github.com/Microsoft/hcsshim/internal/devices"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

const deviceUtilExeName = "device-util.exe"

// getSpecKernelDrivers gets any device drivers specified on the spec.
// Drivers are optional, therefore do not return an error if none are on the spec.
func getSpecKernelDrivers(annots map[string]string) ([]string, error) {
	drivers := oci.ParseAnnotationCommaSeparated(annotations.VirtualMachineKernelDrivers, annots)
	for _, driver := range drivers {
		if _, err := os.Stat(driver); err != nil {
			return nil, errors.Wrapf(err, "failed to find path to drivers at %s", driver)
		}
	}
	return drivers, nil
}

// getDeviceExtensionPaths gets any device extensions paths specified on the spec.
// device extensions are optional, therefore if none are on the spec, do not return an error.
func getDeviceExtensionPaths(annots map[string]string) ([]string, error) {
	extensions := oci.ParseAnnotationCommaSeparated(annotations.DeviceExtensions, annots)
	for _, ext := range extensions {
		if _, err := os.Stat(ext); err != nil {
			return nil, errors.Wrapf(err, "failed to find path to driver extensions at %s", ext)
		}
	}
	return extensions, nil
}

// getGPUVHDPath gets the gpu vhd path from the shim options or uses the default if no
// shim option is set. Right now we only support Nvidia gpus, so this will default to
// a gpu vhd with nvidia files
func getGPUVHDPath(annot map[string]string) (string, error) {
	gpuVHDPath, ok := annot[annotations.GPUVHDPath]
	if !ok || gpuVHDPath == "" {
		return "", errors.New("no gpu vhd specified")
	}
	if _, err := os.Stat(gpuVHDPath); err != nil {
		return "", errors.Wrapf(err, "failed to find gpu support vhd %s", gpuVHDPath)
	}
	return gpuVHDPath, nil
}

// getDeviceUtilHostPath is a simple helper function to find the host path of the device-util tool
func getDeviceUtilHostPath() string {
	return filepath.Join(filepath.Dir(os.Args[0]), deviceUtilExeName)
}

func isDeviceExtensionsSupported() bool {
	// device extensions support was added from 20348 onwards.
	return osversion.Build() >= 20348
}

// getDeviceExtensions is a helper function to read the files at `extensionPaths` and unmarshal the contents
// into a `hcsshema.DeviceExtension` to be added to a container's hcs create document.
func getDeviceExtensions(annotations map[string]string) (*hcsschema.ContainerDefinitionDevice, error) {
	extensionPaths, err := getDeviceExtensionPaths(annotations)
	if err != nil {
		return nil, err
	}

	if len(extensionPaths) == 0 {
		return nil, nil
	}

	if !isDeviceExtensionsSupported() {
		return nil, fmt.Errorf("device extensions are not supported on this build (%d)", osversion.Build())
	}

	results := &hcsschema.ContainerDefinitionDevice{
		DeviceExtension: []hcsschema.DeviceExtension{},
	}
	for _, extensionPath := range extensionPaths {
		data, err := os.ReadFile(extensionPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read extension file at %s", extensionPath)
		}
		extension := hcsschema.DeviceExtension{}
		if err := json.Unmarshal(data, &extension); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal extension file at %s", extensionPath)
		}
		results.DeviceExtension = append(results.DeviceExtension, extension)
	}
	return results, nil
}

// handleAssignedDevicesWindows does all of the work to setup the hosting UVM, assign in devices
// specified on the spec, and install any necessary, specified kernel drivers into the UVM.
//
// Drivers must be installed after the target devices are assigned into the UVM.
// This ordering allows us to guarantee that driver installation on a device in the UVM is completed
// before we attempt to create a container.
func handleAssignedDevicesWindows(
	ctx context.Context,
	vm *uvm.UtilityVM,
	annotations map[string]string,
	specDevs []specs.WindowsDevice) (resultDevs []specs.WindowsDevice, closers []resources.ResourceCloser, err error) {
	defer func() {
		if err != nil {
			// best effort clean up allocated resources on failure
			for _, r := range closers {
				if releaseErr := r.Release(ctx); releaseErr != nil {
					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
				}
			}
			closers = nil
			resultDevs = nil
		}
	}()

	// install the device util tool in the UVM
	toolHostPath := getDeviceUtilHostPath()
	options := vm.DefaultVSMBOptions(true)
	toolsShare, err := vm.AddVSMB(ctx, toolHostPath, options)
	if err != nil {
		return nil, closers, fmt.Errorf("failed to add VSMB share to utility VM for path %+v: %s", toolHostPath, err)
	}
	closers = append(closers, toolsShare)
	deviceUtilPath, err := vm.GetVSMBUvmPath(ctx, toolHostPath, true)
	if err != nil {
		return nil, closers, err
	}

	// assign device into UVM and create corresponding spec windows devices
	for _, d := range specDevs {
		pciID, index := getDeviceInfoFromPath(d.ID)
		vpciCloser, locationPaths, err := devices.AddDevice(ctx, vm, d.IDType, pciID, index, deviceUtilPath)
		if err != nil {
			return nil, nil, err
		}
		closers = append(closers, vpciCloser)
		for _, value := range locationPaths {
			specDev := specs.WindowsDevice{
				ID:     value,
				IDType: uvm.VPCILocationPathIDType,
			}
			log.G(ctx).WithField("parsed devices", specDev).Info("added windows device to spec")
			resultDevs = append(resultDevs, specDev)
		}
	}

	return resultDevs, closers, nil
}

// handleAssignedDevicesLCOW does all of the work to setup the hosting UVM, assign in devices
// specified on the spec
//
// For LCOW, drivers must be installed before the target devices are assigned into the UVM so they
// can be linked on arrival.
func handleAssignedDevicesLCOW(
	ctx context.Context,
	vm *uvm.UtilityVM,
	annotations map[string]string,
	specDevs []specs.WindowsDevice) (resultDevs []specs.WindowsDevice, closers []resources.ResourceCloser, err error) {
	defer func() {
		if err != nil {
			// best effort clean up allocated resources on failure
			for _, r := range closers {
				if releaseErr := r.Release(ctx); releaseErr != nil {
					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
				}
			}
			closers = nil
			resultDevs = nil
		}
	}()

	gpuPresent := false

	// assign device into UVM and create corresponding spec windows devices
	for _, d := range specDevs {
		switch d.IDType {
		case uvm.VPCIDeviceIDType, uvm.VPCIDeviceIDTypeLegacy, uvm.GPUDeviceIDType:
			gpuPresent = gpuPresent || d.IDType == uvm.GPUDeviceIDType
			pciID, index := getDeviceInfoFromPath(d.ID)
			vpci, err := vm.AssignDevice(ctx, pciID, index, "")
			if err != nil {
				return resultDevs, closers, errors.Wrapf(err, "failed to assign device %s, function %d to pod %s", pciID, index, vm.ID())
			}
			closers = append(closers, vpci)

			// update device ID on the spec to the assigned device's resulting vmbus guid so gcs knows which devices to
			// map into the container
			d.ID = vpci.VMBusGUID
			resultDevs = append(resultDevs, d)
		default:
			return resultDevs, closers, errors.Errorf("specified device %s has unsupported type %s", d.ID, d.IDType)
		}
	}

	if gpuPresent {
		gpuSupportVhdPath, err := getGPUVHDPath(annotations)
		if err != nil {
			return resultDevs, closers, errors.Wrapf(err, "failed to add gpu vhd to %v", vm.ID())
		}
		// gpuvhd must be granted VM Group access.
		driverCloser, err := devices.InstallDrivers(ctx, vm, gpuSupportVhdPath, true)
		if err != nil {
			return resultDevs, closers, err
		}
		if driverCloser != nil {
			closers = append(closers, driverCloser)
		}
	}

	return resultDevs, closers, nil
}

// addSpecGuestDrivers is a helper function to install kernel drivers specified on a spec into the guest
func addSpecGuestDrivers(ctx context.Context, vm *uvm.UtilityVM, annotations map[string]string) (closers []resources.ResourceCloser, err error) {
	defer func() {
		if err != nil {
			// best effort clean up allocated resources on failure
			for _, r := range closers {
				if releaseErr := r.Release(ctx); releaseErr != nil {
					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
				}
			}
		}
	}()

	// get the spec specified kernel drivers and install them on the UVM
	drivers, err := getSpecKernelDrivers(annotations)
	if err != nil {
		return closers, err
	}
	for _, d := range drivers {
		driverCloser, err := devices.InstallDrivers(ctx, vm, d, false)
		if err != nil {
			return closers, err
		}
		if driverCloser != nil {
			closers = append(closers, driverCloser)
		}
	}
	return closers, err
}

func getDeviceInfoFromPath(rawDevicePath string) (string, uint16) {
	indexString := filepath.Base(rawDevicePath)
	index, err := strconv.ParseUint(indexString, 10, 16)
	if err == nil {
		// we have a vf index
		return filepath.Dir(rawDevicePath), uint16(index)
	}
	// otherwise, just use default index and full device ID given
	return rawDevicePath, 0
}
