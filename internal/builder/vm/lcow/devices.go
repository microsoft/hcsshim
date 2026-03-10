//go:build windows

package lcow

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/controller/device/vpci"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	"github.com/Microsoft/hcsshim/osversion"
	shimannotations "github.com/Microsoft/hcsshim/pkg/annotations"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

// parseDeviceOptions parses device options from annotations and assigned devices.
// isConfidential indicates if this is a confidential scenario, which affects VPMem and PCI device configuration.
// numaConfig is used to determine if NUMA affinity propagation should be enabled for vPCI devices.
func parseDeviceOptions(
	ctx context.Context,
	annotations map[string]string,
	devices []specs.WindowsDevice,
	rootFsFullPath string,
	isNumaEnabled bool,
	isFullyPhysicallyBacked bool,
	isConfidential bool,
) (*hcsschema.VirtualPMemController, map[string]hcsschema.Scsi, map[string]hcsschema.VirtualPciDevice, error) {

	log.G(ctx).WithFields(logrus.Fields{
		"deviceCount":    len(devices),
		"isConfidential": isConfidential,
		"rootFsPath":     rootFsFullPath,
	}).Debug("parseDeviceOptions: starting device options parsing")

	// ===============================Parse VPMem configuration===============================
	vpmemCount := oci.ParseAnnotationsUint32(ctx, annotations, shimannotations.VPMemCount, vmutils.DefaultVPMEMCount)
	vpmemSize := oci.ParseAnnotationsUint64(ctx, annotations, shimannotations.VPMemSize, vmutils.DefaultVPMemSizeBytes)

	// VPMem is not supported by the enlightened kernel for SNP (confidential VMs).
	if isFullyPhysicallyBacked || isConfidential {
		vpmemCount = 0
	}

	if vpmemCount > vmutils.MaxVPMEMCount {
		return nil, nil, nil, fmt.Errorf("vp_mem_device_count cannot be greater than %d", vmutils.MaxVPMEMCount)
	}

	if vpmemCount > 0 && vpmemSize%4096 != 0 {
		return nil, nil, nil, fmt.Errorf("vp_mem_size_bytes must be a multiple of 4096")
	}

	log.G(ctx).WithFields(logrus.Fields{
		"vpmemCount":     vpmemCount,
		"vpmemSizeBytes": vpmemSize,
	}).Debug("parsed VPMem configuration")

	// Extract the rootfs file name.
	rootFsFile := filepath.Base(rootFsFullPath)

	// Create VPMem controller configuration
	var vpMemController *hcsschema.VirtualPMemController
	if vpmemCount > 0 && rootFsFile == vmutils.VhdFile {
		// If booting from VHD via VPMem, configure the VPMem device for rootfs
		vpMemController = &hcsschema.VirtualPMemController{
			MaximumCount:     vpmemCount,
			MaximumSizeBytes: vpmemSize,
			Devices:          make(map[string]hcsschema.VirtualPMemDevice),
		}

		// Determine image format based on file extension.
		// filepath.Ext returns the extension with the leading dot (e.g. ".vhdx").
		imageFormat := "Vhd1"
		if strings.HasSuffix(strings.ToLower(filepath.Ext(rootFsFile)), "vhdx") {
			imageFormat = "Vhdx"
		}

		// Add rootfs VHD as VPMem device 0
		vpMemController.Devices["0"] = hcsschema.VirtualPMemDevice{
			HostPath:    rootFsFullPath,
			ReadOnly:    true,
			ImageFormat: imageFormat,
		}

		log.G(ctx).WithFields(logrus.Fields{
			"device":      "0",
			"path":        rootFsFullPath,
			"imageFormat": imageFormat,
		}).Debug("configured VPMem device for VHD rootfs boot")
	}

	// ===============================Parse SCSI configuration===============================
	scsiControllerCount := uint32(1)
	// If vpmemMaxCount has been set to 0, it means we are going to need multiple SCSI controllers
	// to support lots of layers.
	if osversion.Build() >= osversion.RS5 && vpmemCount == 0 {
		scsiControllerCount = uint32(len(guestrequest.ScsiControllerGuids))
	}

	log.G(ctx).WithField("scsiControllerCount", scsiControllerCount).Debug("configuring SCSI controllers")

	// Initialize SCSI controllers map with empty controllers
	scsiControllers := map[string]hcsschema.Scsi{}
	for i := uint32(0); i < scsiControllerCount; i++ {
		controllerGUID := guestrequest.ScsiControllerGuids[i]
		scsiControllers[controllerGUID] = hcsschema.Scsi{
			Attachments: make(map[string]hcsschema.Attachment),
		}
	}

	// If booting from VHD via SCSI (no VPMem), attach the rootfs VHD to SCSI controller 0, LUN 0
	// For confidential Containers, rootFSFile will be DmVerityRootfsPath.
	if vpmemCount == 0 && rootFsFile == vmutils.VhdFile {
		scsiControllers[guestrequest.ScsiControllerGuids[0]].Attachments["0"] = hcsschema.Attachment{
			Type_:    "VirtualDisk",
			Path:     rootFsFullPath,
			ReadOnly: true,
		}
		log.G(ctx).WithFields(logrus.Fields{
			"controller": guestrequest.ScsiControllerGuids[0],
			"lun":        "0",
			"path":       rootFsFullPath,
		}).Debug("configured SCSI attachment for VHD rootfs boot")
	}

	// ===============================Parse VPCI Devices configuration===============================
	// For confidential VMs, vPCI assigned devices are not supported
	var vpciDevices map[string]hcsschema.VirtualPciDevice

	if !isConfidential && len(devices) > 0 {
		vpciDevices = make(map[string]hcsschema.VirtualPciDevice)

		log.G(ctx).Debug("parsing vPCI device assignments")
		// deviceKey is used to uniquely identify a device for duplicate detection.
		type deviceKey struct {
			instanceID    string
			functionIndex uint16
		}

		// Use a map to track seen devices and avoid duplicates.
		seen := make(map[deviceKey]struct{})

		// Determine if NUMA affinity propagation should be enabled.
		// Only applicable on builds >= V25H1Server with NUMA-enabled VMs.
		var propagateAffinity *bool
		if osversion.Get().Build >= osversion.V25H1Server {
			if isNumaEnabled {
				t := true
				propagateAffinity = &t
				log.G(ctx).Debug("NUMA affinity propagation enabled for vPCI devices")
			}
		}

		for _, dev := range devices {
			if d := getVPCIDevice(ctx, dev); d != nil {
				key := deviceKey{instanceID: d.DeviceInstancePath, functionIndex: d.VirtualFunction}
				if _, exists := seen[key]; exists {
					return nil, nil, nil, fmt.Errorf("device %s with index %d is specified multiple times", d.DeviceInstancePath, d.VirtualFunction)
				}
				seen[key] = struct{}{}

				// Generate a unique VMBus GUID for each vPCI device.
				vmbusGUID, err := guid.NewV4()
				if err != nil {
					return nil, nil, nil, fmt.Errorf("failed to generate vmbus GUID for device %s: %w", d.DeviceInstancePath, err)
				}

				vpciDevices[vmbusGUID.String()] = hcsschema.VirtualPciDevice{
					Functions: []hcsschema.VirtualPciFunction{
						*d,
					},
					PropagateNumaAffinity: propagateAffinity,
				}

				log.G(ctx).WithFields(logrus.Fields{
					"deviceInstancePath": d.DeviceInstancePath,
					"virtualFunction":    d.VirtualFunction,
					"vmbusGUID":          vmbusGUID.String(),
				}).Debug("configured vPCI device")
			}
		}
	}

	log.G(ctx).Debug("parseDeviceOptions completed successfully")
	return vpMemController, scsiControllers, vpciDevices, nil
}

// getVPCIDevice maps a WindowsDevice into the sandbox vPCIDevice format when supported.
func getVPCIDevice(ctx context.Context, dev specs.WindowsDevice) *hcsschema.VirtualPciFunction {
	pciID, index := vpci.GetDeviceInfoFromPath(dev.ID)
	if vpci.IsValidDeviceType(dev.IDType) {
		log.G(ctx).WithFields(logrus.Fields{
			"deviceInstancePath": pciID,
			"virtualFunction":    index,
			"deviceType":         dev.IDType,
		}).Debug("getVPCIDevice: resolved valid vPCI device")
		return &hcsschema.VirtualPciFunction{
			DeviceInstancePath: pciID,
			VirtualFunction:    index,
		}
	}

	log.G(ctx).WithFields(logrus.Fields{
		"device": dev,
	}).Warnf("device type %s invalid, skipping", dev.IDType)

	return nil
}
