// +build windows

package hcsoci

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Microsoft/hcsshim/internal/processorinfo"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/uvmfolder"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/sirupsen/logrus"
)

// createWindowsContainerDocument creates documents for passing to HCS or GCS to create
// a container, both hosted and process isolated. It creates both v1 and v2
// container objects, WCOW only. The containers storage should have been mounted already.
func createWindowsContainerDocument(ctx context.Context, coi *createOptionsInternal) (*schema1.ContainerConfig, *hcsschema.Container, error) {
	log.G(ctx).Debug("hcsshim: CreateHCSContainerDocument")
	// TODO: Make this safe if exported so no null pointer dereferences.

	if coi.Spec == nil {
		return nil, nil, fmt.Errorf("cannot create HCS container document - OCI spec is missing")
	}

	if coi.Spec.Windows == nil {
		return nil, nil, fmt.Errorf("cannot create HCS container document - OCI spec Windows section is missing ")
	}

	v1 := &schema1.ContainerConfig{
		SystemType:              "Container",
		Name:                    coi.actualID,
		Owner:                   coi.actualOwner,
		HvPartition:             false,
		IgnoreFlushesDuringBoot: coi.Spec.Windows.IgnoreFlushesDuringBoot,
	}

	// IgnoreFlushesDuringBoot is a property of the SCSI attachment for the scratch. Set when it's hot-added to the utility VM
	// ID is a property on the create call in V2 rather than part of the schema.
	v2Container := &hcsschema.Container{Storage: &hcsschema.Storage{}}

	// TODO: Still want to revisit this.
	if coi.Spec.Windows.LayerFolders == nil || len(coi.Spec.Windows.LayerFolders) < 2 {
		return nil, nil, fmt.Errorf("invalid spec - not enough layer folders supplied")
	}

	if coi.Spec.Hostname != "" {
		v1.HostName = coi.Spec.Hostname
		v2Container.GuestOs = &hcsschema.GuestOs{HostName: coi.Spec.Hostname}
	}

	// CPU Resources
	cpuNumSet := 0
	cpuCount := oci.ParseAnnotationsCPUCount(ctx, coi.Spec, oci.AnnotationContainerProcessorCount, 0)
	if cpuCount > 0 {
		cpuNumSet++
	}

	cpuLimit := oci.ParseAnnotationsCPULimit(ctx, coi.Spec, oci.AnnotationContainerProcessorLimit, 0)
	if cpuLimit > 0 {
		cpuNumSet++
	}

	cpuWeight := oci.ParseAnnotationsCPUWeight(ctx, coi.Spec, oci.AnnotationContainerProcessorWeight, 0)
	if cpuWeight > 0 {
		cpuNumSet++
	}

	if cpuNumSet > 1 {
		return nil, nil, fmt.Errorf("invalid spec - Windows Process Container CPU Count: '%d', Limit: '%d', and Weight: '%d' are mutually exclusive", cpuCount, cpuLimit, cpuWeight)
	} else if cpuNumSet == 1 {
		var hostCPUCount int32
		if coi.HostingSystem != nil {
			// Normalize to UVM size
			hostCPUCount = coi.HostingSystem.ProcessorCount()
		} else {
			// For process isolated case the amount of logical processors reported
			// from the HCS apis may be greater than what is available to the host in a
			// minroot configuration (host doesn't have access to all available LPs)
			// so prefer standard OS level calls here instead.
			hostCPUCount = processorinfo.ProcessorCount()
		}
		if cpuCount > hostCPUCount {
			l := log.G(ctx).WithField(logfields.ContainerID, coi.ID)
			if coi.HostingSystem != nil {
				l.Data[logfields.UVMID] = coi.HostingSystem.ID()
			}
			l.WithFields(logrus.Fields{
				"requested": cpuCount,
				"assigned":  hostCPUCount,
			}).Warn("Changing user requested CPUCount to current number of processors")
			cpuCount = hostCPUCount
		}

		v1.ProcessorCount = uint32(cpuCount)
		v1.ProcessorMaximum = int64(cpuLimit)
		v1.ProcessorWeight = uint64(cpuWeight)

		v2Container.Processor = &hcsschema.Processor{
			Count:   cpuCount,
			Maximum: cpuLimit,
			Weight:  cpuWeight,
		}
	}

	// Memory Resources
	memoryMaxInMB := oci.ParseAnnotationsMemory(ctx, coi.Spec, oci.AnnotationContainerMemorySizeInMB, 0)
	if memoryMaxInMB > 0 {
		v1.MemoryMaximumInMB = int64(memoryMaxInMB)
		v2Container.Memory = &hcsschema.Memory{
			SizeInMB: memoryMaxInMB,
		}
	}

	// Storage Resources
	storageBandwidthMax := oci.ParseAnnotationsStorageBps(ctx, coi.Spec, oci.AnnotationContainerStorageQoSBandwidthMaximum, 0)
	storageIopsMax := oci.ParseAnnotationsStorageIops(ctx, coi.Spec, oci.AnnotationContainerStorageQoSIopsMaximum, 0)
	if storageBandwidthMax > 0 || storageIopsMax > 0 {
		v1.StorageBandwidthMaximum = uint64(storageBandwidthMax)
		v1.StorageIOPSMaximum = uint64(storageIopsMax)
		v2Container.Storage.QoS = &hcsschema.StorageQoS{
			BandwidthMaximum: storageBandwidthMax,
			IopsMaximum:      storageIopsMax,
		}
	}

	// TODO V2 networking. Only partial at the moment. v2.Container.Networking.Namespace specifically
	if coi.Spec.Windows.Network != nil {
		v2Container.Networking = &hcsschema.Networking{}

		v1.EndpointList = coi.Spec.Windows.Network.EndpointList
		v2Container.Networking.Namespace = coi.actualNetworkNamespace

		v1.AllowUnqualifiedDNSQuery = coi.Spec.Windows.Network.AllowUnqualifiedDNSQuery
		v2Container.Networking.AllowUnqualifiedDnsQuery = v1.AllowUnqualifiedDNSQuery

		if coi.Spec.Windows.Network.DNSSearchList != nil {
			v1.DNSSearchList = strings.Join(coi.Spec.Windows.Network.DNSSearchList, ",")
			v2Container.Networking.DnsSearchList = v1.DNSSearchList
		}

		v1.NetworkSharedContainerName = coi.Spec.Windows.Network.NetworkSharedContainerName
		v2Container.Networking.NetworkSharedContainerName = v1.NetworkSharedContainerName
	}

	if cs, ok := coi.Spec.Windows.CredentialSpec.(string); ok {
		v1.Credentials = cs
		// If this is a HCS v2 schema container, we created the CCG instance
		// with the other container resources. Pass the CCG state information
		// as part of the container document.
		if coi.ccgState != nil {
			v2Container.ContainerCredentialGuard = coi.ccgState
		}
	}

	if coi.Spec.Root == nil {
		return nil, nil, fmt.Errorf("spec is invalid - root isn't populated")
	}

	if coi.Spec.Root.Readonly {
		return nil, nil, fmt.Errorf(`invalid container spec - readonly is not supported for Windows containers`)
	}

	// Strip off the top-most RW/scratch layer as that's passed in separately to HCS for v1
	v1.LayerFolderPath = coi.Spec.Windows.LayerFolders[len(coi.Spec.Windows.LayerFolders)-1]

	if coi.isV2Argon() || coi.isV1Argon() {
		// Argon v1 or v2.
		const volumeGUIDRegex = `^\\\\\?\\(Volume)\{{0,1}[0-9a-fA-F]{8}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{12}(\}){0,1}\}(|\\)$`
		if matched, err := regexp.MatchString(volumeGUIDRegex, coi.Spec.Root.Path); !matched || err != nil {
			return nil, nil, fmt.Errorf(`invalid container spec - Root.Path '%s' must be a volume GUID path in the format '\\?\Volume{GUID}\'`, coi.Spec.Root.Path)
		}
		if coi.Spec.Root.Path[len(coi.Spec.Root.Path)-1] != '\\' {
			coi.Spec.Root.Path += `\` // Be nice to clients and make sure well-formed for back-compat
		}
		v1.VolumePath = coi.Spec.Root.Path[:len(coi.Spec.Root.Path)-1] // Strip the trailing backslash. Required for v1.
		v2Container.Storage.Path = coi.Spec.Root.Path
	} else if coi.isV1Xenon() {
		// V1 Xenon
		v1.HvPartition = true
		if coi.Spec == nil || coi.Spec.Windows == nil || coi.Spec.Windows.HyperV == nil { // Be resilient to nil de-reference
			return nil, nil, fmt.Errorf(`invalid container spec - Spec.Windows.HyperV is nil`)
		}
		if coi.Spec.Windows.HyperV.UtilityVMPath != "" {
			// Client-supplied utility VM path
			v1.HvRuntime = &schema1.HvRuntime{ImagePath: coi.Spec.Windows.HyperV.UtilityVMPath}
		} else {
			// Client was lazy. Let's locate it from the layer folders instead.
			uvmImagePath, err := uvmfolder.LocateUVMFolder(ctx, coi.Spec.Windows.LayerFolders)
			if err != nil {
				return nil, nil, err
			}
			v1.HvRuntime = &schema1.HvRuntime{ImagePath: filepath.Join(uvmImagePath, `UtilityVM`)}
		}
	} else if coi.isV2Xenon() {
		// Hosting system was supplied, so is v2 Xenon.
		v2Container.Storage.Path = coi.Spec.Root.Path
		if coi.HostingSystem.OS() == "windows" {
			layers, err := computeV2Layers(ctx, coi.HostingSystem, coi.Spec.Windows.LayerFolders[:len(coi.Spec.Windows.LayerFolders)-1])
			if err != nil {
				return nil, nil, err
			}
			v2Container.Storage.Layers = layers
		}
	}

	if coi.isV2Argon() || coi.isV1Argon() { // Argon v1 or v2
		for _, layerPath := range coi.Spec.Windows.LayerFolders[:len(coi.Spec.Windows.LayerFolders)-1] {
			layerID, err := wclayer.LayerID(ctx, layerPath)
			if err != nil {
				return nil, nil, err
			}
			v1.Layers = append(v1.Layers, schema1.Layer{ID: layerID.String(), Path: layerPath})
			v2Container.Storage.Layers = append(v2Container.Storage.Layers, hcsschema.Layer{Id: layerID.String(), Path: layerPath})
		}
	}

	// Add the mounts as mapped directories or mapped pipes
	// TODO: Mapped pipes to add in v2 schema.
	var (
		mdsv1 []schema1.MappedDir
		mpsv1 []schema1.MappedPipe
		mdsv2 []hcsschema.MappedDirectory
		mpsv2 []hcsschema.MappedPipe
	)
	for _, mount := range coi.Spec.Mounts {
		if mount.Type != "" {
			return nil, nil, fmt.Errorf("invalid container spec - Mount.Type '%s' must not be set", mount.Type)
		}
		if uvm.IsPipe(mount.Source) {
			src, dst := uvm.GetContainerPipeMapping(coi.HostingSystem, mount)
			mpsv1 = append(mpsv1, schema1.MappedPipe{HostPath: src, ContainerPipeName: dst})
			mpsv2 = append(mpsv2, hcsschema.MappedPipe{HostPath: src, ContainerPipeName: dst})
		} else {
			readOnly := false
			for _, o := range mount.Options {
				if strings.ToLower(o) == "ro" {
					readOnly = true
				}
			}
			mdv1 := schema1.MappedDir{HostPath: mount.Source, ContainerPath: mount.Destination, ReadOnly: readOnly}
			mdv2 := hcsschema.MappedDirectory{ContainerPath: mount.Destination, ReadOnly: readOnly}
			if coi.HostingSystem == nil {
				mdv2.HostPath = mount.Source
			} else {
				uvmPath, err := coi.HostingSystem.GetVSMBUvmPath(ctx, mount.Source)
				if err != nil {
					if err == uvm.ErrNotAttached {
						// It could also be a scsi mount.
						uvmPath, err = coi.HostingSystem.GetScsiUvmPath(ctx, mount.Source)
						if err != nil {
							return nil, nil, err
						}
					} else {
						return nil, nil, err
					}
				}
				mdv2.HostPath = uvmPath
			}
			mdsv1 = append(mdsv1, mdv1)
			mdsv2 = append(mdsv2, mdv2)
		}
	}

	v1.MappedDirectories = mdsv1
	v2Container.MappedDirectories = mdsv2
	if len(mpsv1) > 0 && osversion.Get().Build < osversion.RS3 {
		return nil, nil, fmt.Errorf("named pipe mounts are not supported on this version of Windows")
	}
	v1.MappedPipes = mpsv1
	v2Container.MappedPipes = mpsv2

	if specHasAssignedDevices(coi) {
		// add assigned devices to the container definition
		if err := parseAssignedDevices(coi, v2Container); err != nil {
			return nil, nil, err
		}
	}

	return v1, v2Container, nil
}

func specHasAssignedDevices(coi *createOptionsInternal) bool {
	if coi.Spec.Windows != nil && coi.Spec.Windows.Devices != nil &&
		len(coi.Spec.Windows.Devices) > 0 {
		return true
	}
	return false
}

// parseAssignedDevices parses assigned devices for the container definition
// this is currently supported for HCS schema V2 argon only
func parseAssignedDevices(coi *createOptionsInternal, v2 *hcsschema.Container) error {
	if !coi.isV2Argon() {
		return errors.New("device assignment is currently only supported for HCS schema V2 argon")
	}

	v2AssignedDevices := []hcsschema.Device{}
	for _, d := range coi.Spec.Windows.Devices {
		v2Dev := hcsschema.Device{}
		switch d.IDType {
		case uvm.VPCILocationPathIDType:
			v2Dev.LocationPath = d.ID
			v2Dev.Type = hcsschema.DeviceInstance
		case uvm.VPCIClassGUIDTypeLegacy:
			v2Dev.InterfaceClassGuid = d.ID
		case uvm.VPCIClassGUIDType:
			v2Dev.InterfaceClassGuid = d.ID
		default:
			return fmt.Errorf("specified device %s has unsupported type %s", d.ID, d.IDType)
		}
		v2AssignedDevices = append(v2AssignedDevices, v2Dev)
	}
	v2.AssignedDevices = v2AssignedDevices
	return nil
}
