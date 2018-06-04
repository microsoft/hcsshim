// +build windows

package hcsoci

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guid"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/osversion"
	version "github.com/Microsoft/hcsshim/internal/osversion"
	"github.com/Microsoft/hcsshim/internal/schema1"
	hcsschemav2 "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/uvmfolder"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

// CreateOptions are the set of fields used to call CreateContainer().
// Note: In the spec, the LayerFolders must be arranged in the same way in which
// moby configures them: layern, layern-1,...,layer2,layer1,scratch
// where layer1 is the base read-only layer, layern is the top-most read-only
// layer, and scratch is the RW layer. This is for historical reasons only.
type CreateOptions struct {

	// Common parameters
	ID               string                       // Identifier for the container
	Owner            string                       // Specifies the owner. Defaults to executable name.
	Spec             *specs.Spec                  // Definition of the container or utility VM being created
	SchemaVersion    *schemaversion.SchemaVersion // Requested Schema Version. Defaults to v2 for RS5, v1 for RS1..RS4
	HostingSystem    *uvm.UtilityVM               // Utility or service VM in which the container is to be created.
	NetworkNamespace string                       // Host network namespace to use (overrides anything in the spec)

	// This is an advanced debugging parameter. It allows for diagnosibility by leaving a containers
	// resources allocated in case of a failure. Thus you would be able to use tools such as hcsdiag
	// to look at the state of a utility VM to see what resources were allocated. Obviously the caller
	// must a) not tear down the utility VM on failure (or pause in some way) and b) is responsible for
	// performing the ReleaseResources() call themselves.
	DoNotReleaseResourcesOnFailure bool

	// These are v1 LCOW backwards-compatibility only.
	KirdPath          string // Folder in which kernel and initrd reside. Defaults to \Program Files\Linux Containers
	KernelFile        string // Filename under KirdPath for the kernel. Defaults to bootx64.efi
	InitrdFile        string // Filename under KirdPath for the initrd image. Defaults to initrd.img
	KernelBootOptions string // Additional boot options for the kernel
}

// createOptionsInternal is the set of user-supplied create options, but includes internal
// fields for processing the request once user-supplied stuff has been validated.
type createOptionsInternal struct {
	*CreateOptions

	actualSchemaVersion    *schemaversion.SchemaVersion // Calculated based on Windows build and optional caller-supplied override
	actualID               string                       // Identifier for the container
	actualOwner            string                       // Owner for the container
	actualNetworkNamespace string

	// These are v1 LCOW backwards-compatibility only
	actualKirdPath   string // LCOW kernel/initrd path
	actualKernelFile string // LCOW kernel file
	actualInitrdFile string // LCOW initrd file

}

// CreateContainer creates a container. It can cope with a  wide variety of
// scenarios, including v1 HCS schema calls, as well as more complex v2 HCS schema
// calls. Note we always return the resources that have been allocated, even in the
// case of an error. This provides support for the debugging option not to
// release the resources on failure, so that the client can make the necessary
// call to release resources that have been allocated as part of calling this function.
func CreateContainer(createOptions *CreateOptions) (_ *hcs.System, _ *Resources, err error) {
	logrus.Debugf("hcsshim::CreateContainer options: %+v", createOptions)

	coi := &createOptionsInternal{
		CreateOptions:    createOptions,
		actualID:         createOptions.ID,
		actualOwner:      createOptions.Owner,
		actualKirdPath:   createOptions.KirdPath,
		actualKernelFile: createOptions.KernelFile,
		actualInitrdFile: createOptions.InitrdFile,
	}

	// Defaults if omitted by caller.
	if coi.actualID == "" {
		coi.actualID = guid.New().String()
	}
	if coi.actualOwner == "" {
		coi.actualOwner = filepath.Base(os.Args[0])
	}
	if coi.actualKirdPath == "" {
		coi.actualKirdPath = filepath.Join(os.Getenv("ProgramFiles"), "Linux Containers")
	}
	if coi.actualKernelFile == "" {
		coi.actualKernelFile = "bootx64.efi"
	}
	if coi.actualInitrdFile == "" {
		coi.actualInitrdFile = "initrd.img"
	}

	if coi.Spec == nil {
		return nil, nil, fmt.Errorf("Spec must be supplied")
	}

	if coi.HostingSystem != nil {
		// By definition, a hosting system can only be supplied for a v2 Xenon.
		coi.actualSchemaVersion = schemaversion.SchemaV20()
	} else {
		coi.actualSchemaVersion = schemaversion.DetermineSchemaVersion(coi.SchemaVersion)
		logrus.Debugf("hcsshim::CreateContainer using schema %s", coi.actualSchemaVersion.String())
	}

	resources := &Resources{}
	defer func() {
		if err != nil {
			if !coi.DoNotReleaseResourcesOnFailure {
				ReleaseResources(resources, coi.HostingSystem, true)
			}
		}
	}()

	if coi.HostingSystem != nil {
		n := coi.HostingSystem.ContainerCounter()
		if coi.Spec.Linux != nil {
			resources.GuestRoot = "/run/gcs/c/" + strconv.FormatUint(n, 16)
		} else {
			resources.GuestRoot = `C:\c\` + strconv.FormatUint(n, 16)
		}
	}

	// Create a network namespace if necessary.
	if coi.Spec.Windows != nil &&
		coi.Spec.Windows.Network != nil &&
		coi.actualSchemaVersion.IsV20() {

		if coi.NetworkNamespace != "" {
			resources.NetNS = coi.NetworkNamespace
		} else {
			err := createNetworkNamespace(coi, resources)
			if err != nil {
				return nil, resources, err
			}
		}
		coi.actualNetworkNamespace = resources.NetNS
		if coi.HostingSystem != nil {
			endpoints, err := getNamespaceEndpoints(coi.actualNetworkNamespace)
			if err != nil {
				return nil, resources, err
			}
			err = coi.HostingSystem.AddNetNS(coi.actualNetworkNamespace, endpoints)
			if err != nil {
				return nil, resources, err
			}
			resources.AddedNetNSToVM = true
		}
	}

	var hcsDocument interface{}
	logrus.Debugf("hcsshim::CreateContainer allocating resources")
	if coi.Spec.Linux != nil {
		if coi.actualSchemaVersion.IsV10() {
			logrus.Debugf("hcsshim::CreateContainer createLCOWv1")
			if coi.Spec.Windows == nil {
				return nil, resources, fmt.Errorf("containerSpec 'Windows' field must container layer folders for a Linux container")
			}
			//return createLCOWv1(coi)
			return nil, resources, errors.New("LCOW v1 not supported")
		}

		logrus.Debugf("hcsshim::CreateContainer allocateLinuxResources")
		err = allocateLinuxResources(coi, resources)
		if err != nil {
			logrus.Debugf("failed to allocateLinuxResources %s", err)
			return nil, resources, err
		}
		hcsDocument, err = createLinuxContainerDocument(coi, resources.GuestRoot)
		if err != nil {
			logrus.Debugf("failed createHCSContainerDocument %s", err)
			return nil, resources, err
		}
	} else {
		err = allocateWindowsResources(coi, resources)
		if err != nil {
			logrus.Debugf("failed to allocateWindowsResources %s", err)
			return nil, resources, err
		}
		logrus.Debugf("hcsshim::CreateContainer creating container document")
		hcsDocument, err = createWindowsContainerDocument(coi)
		if err != nil {
			logrus.Debugf("failed createHCSContainerDocument %s", err)
			return nil, resources, err
		}
	}

	logrus.Debugf("hcsshim::CreateContainer creating compute system")
	system, err := hcs.CreateComputeSystem(coi.actualID, hcsDocument)
	if err != nil {
		logrus.Debugf("failed to CreateComputeSystem %s", err)
		return nil, resources, err
	}
	return system, resources, err
}

func createLCOWSpec(coi *createOptionsInternal) (*specs.Spec, error) {
	// Remarshal the spec to perform a deep copy.
	j, err := json.Marshal(coi.Spec)
	if err != nil {
		return nil, err
	}
	spec := &specs.Spec{}
	err = json.Unmarshal(j, spec)
	if err != nil {
		return nil, err
	}

	// Translate the mounts. The root has already been translated in
	// allocateLinuxResources.
	/*
		for i := range spec.Mounts {
			spec.Mounts[i].Source = "???"
			spec.Mounts[i].Destination = "???"
		}
	*/

	// Linux containers don't care about Windows aspects of the spec
	spec.Windows = nil

	// Hooks are not supported (they should be run in the host)
	spec.Hooks = nil

	// Clear unsupported features
	if spec.Linux.Resources != nil {
		spec.Linux.Resources.Devices = nil
		spec.Linux.Resources.Memory = nil
		spec.Linux.Resources.Pids = nil
		spec.Linux.Resources.BlockIO = nil
		spec.Linux.Resources.HugepageLimits = nil
		spec.Linux.Resources.Network = nil
	}
	spec.Linux.Seccomp = nil

	// Clear any specified namespaces
	var namespaces []specs.LinuxNamespace
	for _, ns := range spec.Linux.Namespaces {
		switch ns.Type {
		case specs.NetworkNamespace:
		default:
			ns.Path = ""
			namespaces = append(namespaces, ns)
		}
	}
	spec.Linux.Namespaces = namespaces

	return spec, nil
}

type linuxHostedSystem struct {
	SchemaVersion    *schemaversion.SchemaVersion
	OciBundlePath    string
	OciSpecification *specs.Spec
}

func createLinuxContainerDocument(coi *createOptionsInternal, guestRoot string) (interface{}, error) {
	spec, err := createLCOWSpec(coi)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("hcsshim::createLinuxContainerDoc: guestRoot:%s", guestRoot)
	v2 := &hcsschemav2.ComputeSystemV2{
		Owner:                             coi.actualOwner,
		SchemaVersion:                     schemaversion.SchemaV20(),
		ShouldTerminateOnLastHandleClosed: true,
		HostingSystemId:                   coi.HostingSystem.ID(),
		HostedSystem: &linuxHostedSystem{
			SchemaVersion:    schemaversion.SchemaV20(),
			OciBundlePath:    guestRoot,
			OciSpecification: spec,
		},
	}

	return v2, nil
}

// createWindowsContainerDocument creates a document suitable for calling HCS to create
// a container, both hosted and process isolated. It can create both v1 and v2
// schema, WCOW only. The containers storage should have been mounted already.
func createWindowsContainerDocument(coi *createOptionsInternal) (interface{}, error) {
	logrus.Debugf("hcsshim: CreateHCSContainerDocument")
	// TODO: Make this safe if exported so no null pointer dereferences.

	if coi.Spec == nil {
		return nil, fmt.Errorf("cannot create HCS container document - OCI spec is missing")
	}

	if coi.Spec.Windows == nil {
		return nil, fmt.Errorf("cannot create HCS container document - OCI spec Windows section is missing ")
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
	v2 := &hcsschemav2.ComputeSystemV2{
		Owner:                             coi.actualOwner,
		SchemaVersion:                     schemaversion.SchemaV20(),
		ShouldTerminateOnLastHandleClosed: true,
	}
	v2Container := &hcsschemav2.ContainerV2{Storage: &hcsschemav2.ContainersResourcesStorageV2{}}

	// TODO: Still want to revisit this.
	if coi.Spec.Windows.LayerFolders == nil || len(coi.Spec.Windows.LayerFolders) < 2 {
		return nil, fmt.Errorf("invalid spec - not enough layer folders supplied")
	}

	if coi.Spec.Hostname != "" {
		v1.HostName = coi.Spec.Hostname
		v2Container.GuestOS = &hcsschemav2.GuestOsV2{HostName: coi.Spec.Hostname}
	}

	if coi.Spec.Windows.Resources != nil {
		if coi.Spec.Windows.Resources.CPU != nil {
			if coi.Spec.Windows.Resources.CPU.Count != nil ||
				coi.Spec.Windows.Resources.CPU.Shares != nil ||
				coi.Spec.Windows.Resources.CPU.Maximum != nil {
				v2Container.Processor = &hcsschemav2.ContainersResourcesProcessorV2{}
			}
			if coi.Spec.Windows.Resources.CPU.Count != nil {
				cpuCount := *coi.Spec.Windows.Resources.CPU.Count
				hostCPUCount := uint64(runtime.NumCPU())
				if cpuCount > hostCPUCount {
					logrus.Warnf("Changing requested CPUCount of %d to current number of processors, %d", cpuCount, hostCPUCount)
					cpuCount = hostCPUCount
				}
				v1.ProcessorCount = uint32(cpuCount)
				v2Container.Processor.Count = v1.ProcessorCount
			}
			if coi.Spec.Windows.Resources.CPU.Shares != nil {
				v1.ProcessorWeight = uint64(*coi.Spec.Windows.Resources.CPU.Shares)
				v2Container.Processor.Weight = v1.ProcessorWeight
			}
			if coi.Spec.Windows.Resources.CPU.Maximum != nil {
				v1.ProcessorMaximum = int64(*coi.Spec.Windows.Resources.CPU.Maximum)
				v2Container.Processor.Maximum = uint64(v1.ProcessorMaximum)
			}
		}
		if coi.Spec.Windows.Resources.Memory != nil {
			if coi.Spec.Windows.Resources.Memory.Limit != nil {
				v1.MemoryMaximumInMB = int64(*coi.Spec.Windows.Resources.Memory.Limit) / 1024 / 1024
				v2Container.Memory = &hcsschemav2.ContainersResourcesMemoryV2{Maximum: uint64(v1.MemoryMaximumInMB)}

			}
		}
		if coi.Spec.Windows.Resources.Storage != nil {
			if coi.Spec.Windows.Resources.Storage.Bps != nil || coi.Spec.Windows.Resources.Storage.Iops != nil {
				v2Container.Storage.StorageQoS = &hcsschemav2.ContainersResourcesStorageQoSV2{}
			}
			if coi.Spec.Windows.Resources.Storage.Bps != nil {
				v1.StorageBandwidthMaximum = *coi.Spec.Windows.Resources.Storage.Bps
				v2Container.Storage.StorageQoS.BandwidthMaximum = *coi.Spec.Windows.Resources.Storage.Bps
			}
			if coi.Spec.Windows.Resources.Storage.Iops != nil {
				v1.StorageIOPSMaximum = *coi.Spec.Windows.Resources.Storage.Iops
				v2Container.Storage.StorageQoS.IOPSMaximum = *coi.Spec.Windows.Resources.Storage.Iops
			}
		}
	}

	// TODO V2 networking. Only partial at the moment. v2.Container.Networking.Namespace specifically
	if coi.Spec.Windows.Network != nil {
		v2Container.Networking = &hcsschemav2.ContainersResourcesNetworkingV2{}

		v1.EndpointList = coi.Spec.Windows.Network.EndpointList
		v2Container.Networking.Namespace = coi.actualNetworkNamespace

		v1.AllowUnqualifiedDNSQuery = coi.Spec.Windows.Network.AllowUnqualifiedDNSQuery
		v2Container.Networking.AllowUnqualifiedDnsQuery = v1.AllowUnqualifiedDNSQuery

		if coi.Spec.Windows.Network.DNSSearchList != nil {
			v1.DNSSearchList = strings.Join(coi.Spec.Windows.Network.DNSSearchList, ",")
			v2Container.Networking.DNSSearchList = v1.DNSSearchList
		}

		v1.NetworkSharedContainerName = coi.Spec.Windows.Network.NetworkSharedContainerName
		v2Container.Networking.NetworkSharedContainerName = v1.NetworkSharedContainerName
	}

	//	// TODO V2 Credentials not in the schema yet.
	if cs, ok := coi.Spec.Windows.CredentialSpec.(string); ok {
		v1.Credentials = cs
	}

	if coi.Spec.Root == nil {
		return nil, fmt.Errorf("spec is invalid - root isn't populated")
	}

	if coi.Spec.Root.Readonly {
		return nil, fmt.Errorf(`invalid container spec - readonly is not supported for Windows containers`)
	}

	// Strip off the top-most RW/scratch layer as that's passed in separately to HCS for v1
	v1.LayerFolderPath = coi.Spec.Windows.LayerFolders[len(coi.Spec.Windows.LayerFolders)-1]

	if (coi.actualSchemaVersion.IsV20() && coi.HostingSystem == nil) ||
		(coi.actualSchemaVersion.IsV10() && coi.Spec.Windows.HyperV == nil) {
		// Argon v1 or v2.
		const volumeGUIDRegex = `^\\\\\?\\(Volume)\{{0,1}[0-9a-fA-F]{8}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{4}\-[0-9a-fA-F]{12}(\}){0,1}\}\\$`
		if _, err := regexp.MatchString(volumeGUIDRegex, coi.Spec.Root.Path); err != nil {
			return nil, fmt.Errorf(`invalid container spec - Root.Path '%s' must be a volume GUID path in the format '\\?\Volume{GUID}\'`, coi.Spec.Root.Path)
		}
		if coi.Spec.Root.Path[len(coi.Spec.Root.Path)-1] != '\\' {
			coi.Spec.Root.Path += `\` // Be nice to clients and make sure well-formed for back-compat
		}
		v1.VolumePath = coi.Spec.Root.Path[:len(coi.Spec.Root.Path)-1] // Strip the trailing backslash. Required for v1.
		v2Container.Storage.Path = coi.Spec.Root.Path
	} else {
		// A hosting system was supplied, implying v2 Xenon; OR a v1 Xenon.
		if coi.actualSchemaVersion.IsV10() {
			// V1 Xenon
			v1.HvPartition = true
			if coi.Spec == nil || coi.Spec.Windows == nil || coi.Spec.Windows.HyperV == nil { // Be resilient to nil de-reference
				return nil, fmt.Errorf(`invalid container spec - Spec.Windows.HyperV is nil`)
			}
			if coi.Spec.Windows.HyperV.UtilityVMPath != "" {
				// Client-supplied utility VM path
				v1.HvRuntime = &schema1.HvRuntime{ImagePath: coi.Spec.Windows.HyperV.UtilityVMPath}
			} else {
				// Client was lazy. Let's locate it from the layer folders instead.
				uvmImagePath, err := uvmfolder.LocateUVMFolder(coi.Spec.Windows.LayerFolders)
				if err != nil {
					return nil, err
				}
				v1.HvRuntime = &schema1.HvRuntime{ImagePath: filepath.Join(uvmImagePath, `UtilityVM`)}
			}
		} else {
			// Hosting system was supplied, so is v2 Xenon.
			v2Container.Storage.Path = coi.Spec.Root.Path
			if coi.HostingSystem.OS() == "windows" {
				layers, err := computeV2Layers(coi.HostingSystem, coi.Spec.Windows.LayerFolders[:len(coi.Spec.Windows.LayerFolders)-1])
				if err != nil {
					return nil, err
				}
				v2Container.Storage.Layers = layers
			}
		}
	}

	if coi.HostingSystem == nil { // Argon v1 or v2
		for _, layerPath := range coi.Spec.Windows.LayerFolders[:len(coi.Spec.Windows.LayerFolders)-1] {
			layerID, err := wclayer.LayerID(layerPath)
			if err != nil {
				return nil, err
			}
			v1.Layers = append(v1.Layers, schema1.Layer{ID: layerID.String(), Path: layerPath})
			v2Container.Storage.Layers = append(v2Container.Storage.Layers, hcsschemav2.ContainersResourcesLayerV2{Id: layerID.String(), Path: layerPath})
		}
	}

	// Add the mounts as mapped directories or mapped pipes
	// TODO: Mapped pipes to add in v2 schema.
	var (
		mdsv1 []schema1.MappedDir
		mpsv1 []schema1.MappedPipe
		mdsv2 []hcsschemav2.ContainersResourcesMappedDirectoryV2
		mpsv2 []hcsschemav2.ContainersResourcesMappedPipeV2
	)
	for _, mount := range coi.Spec.Mounts {
		const pipePrefix = `\\.\pipe\`
		if mount.Type != "" {
			return nil, fmt.Errorf("invalid container spec - Mount.Type '%s' must not be set", mount.Type)
		}
		if strings.HasPrefix(mount.Destination, pipePrefix) {
			mpsv1 = append(mpsv1, schema1.MappedPipe{HostPath: mount.Source, ContainerPipeName: mount.Destination[len(pipePrefix):]})
			mpsv2 = append(mpsv2, hcsschemav2.ContainersResourcesMappedPipeV2{HostPath: mount.Source, ContainerPipeName: mount.Destination[len(pipePrefix):]})
		} else {
			mdv1 := schema1.MappedDir{HostPath: mount.Source, ContainerPath: mount.Destination, ReadOnly: false}
			var mdv2 hcsschemav2.ContainersResourcesMappedDirectoryV2
			if coi.HostingSystem == nil {
				mdv2 = hcsschemav2.ContainersResourcesMappedDirectoryV2{HostPath: mount.Source, ContainerPath: mount.Destination, ReadOnly: false}
			} else {
				guestPath, err := coi.HostingSystem.GetVSMBGuestPath(mount.Source)
				if err != nil {
					return nil, err
				}
				mdv2 = hcsschemav2.ContainersResourcesMappedDirectoryV2{
					HostPath:      guestPath,
					ContainerPath: mount.Destination,
					ReadOnly:      false,
				}
			}
			for _, o := range mount.Options {
				if strings.ToLower(o) == "ro" {
					mdv1.ReadOnly = true
					mdv2.ReadOnly = true
				}
			}
			mdsv1 = append(mdsv1, mdv1)
			mdsv2 = append(mdsv2, mdv2)
		}
	}

	v1.MappedDirectories = mdsv1
	v2Container.MappedDirectories = mdsv2
	if len(mpsv1) > 0 && version.GetOSVersion().Build < osversion.RS3 {
		return nil, fmt.Errorf("named pipe mounts are not supported on this version of Windows")
	}
	v1.MappedPipes = mpsv1
	v2Container.MappedPipes = mpsv2

	// Put the v2Container object as a HostedSystem for a Xenon, or directly in the schema for an Argon.
	if coi.HostingSystem == nil {
		v2.Container = v2Container
	} else {
		v2.HostingSystemId = coi.HostingSystem.ID()
		v2.HostedSystem = &hcsschemav2.HostedSystemV2{
			SchemaVersion: schemaversion.SchemaV20(),
			Container:     v2Container,
		}
	}

	if coi.actualSchemaVersion.IsV10() {
		return v1, nil
	}

	return v2, nil
}
