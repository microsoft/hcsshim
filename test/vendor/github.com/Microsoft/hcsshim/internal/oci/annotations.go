package oci

const (
	// AnnotationContainerMemorySizeInMB overrides the container memory size set
	// via the OCI spec.
	//
	// Note: This annotation is in MB. OCI is in Bytes. When using this override
	// the caller MUST use MB or sizing will be wrong.
	//
	// Note: This is only present because CRI does not (currently) have a
	// `WindowsPodSandboxConfig` for setting this correctly. It should not be
	// used via OCI runtimes and rather use
	// `spec.Windows.Resources.Memory.Limit`.
	AnnotationContainerMemorySizeInMB = "io.microsoft.container.memory.sizeinmb"
	// AnnotationContainerProcessorCount overrides the container processor count
	// set via the OCI spec.
	//
	// Note: For Windows Process Containers CPU Count/Limit/Weight are mutually
	// exclusive and the caller MUST only set one of the values.
	//
	// Note: This is only present because CRI does not (currently) have a
	// `WindowsPodSandboxConfig` for setting this correctly. It should not be
	// used via OCI runtimes and rather use `spec.Windows.Resources.CPU.Count`.
	AnnotationContainerProcessorCount = "io.microsoft.container.processor.count"
	// AnnotationContainerProcessorLimit overrides the container processor limit
	// set via the OCI spec.
	//
	// Limit allows values 1 - 10,000 where 10,000 means 100% CPU. (And is the
	// default if omitted)
	//
	// Note: For Windows Process Containers CPU Count/Limit/Weight are mutually
	// exclusive and the caller MUST only set one of the values.
	//
	// Note: This is only present because CRI does not (currently) have a
	// `WindowsPodSandboxConfig` for setting this correctly. It should not be
	// used via OCI runtimes and rather use
	// `spec.Windows.Resources.CPU.Maximum`.
	AnnotationContainerProcessorLimit = "io.microsoft.container.processor.limit"
	// AnnotationContainerProcessorWeight overrides the container processor
	// weight set via the OCI spec.
	//
	// Weight allows values 0 - 10,000. (100 is the default)
	//
	// Note: For Windows Process Containers CPU Count/Limit/Weight are mutually
	// exclusive and the caller MUST only set one of the values.
	//
	// Note: This is only present because CRI does not (currently) have a
	// `WindowsPodSandboxConfig` for setting this correctly. It should not be
	// used via OCI runtimes and rather use `spec.Windows.Resources.CPU.Shares`.
	AnnotationContainerProcessorWeight = "io.microsoft.container.processor.weight"
	// AnnotationContainerStorageQoSBandwidthMaximum overrides the container
	// storage bandwidth per second set via the OCI spec.
	//
	// Note: This is only present because CRI does not (currently) have a
	// `WindowsPodSandboxConfig` for setting this correctly. It should not be
	// used via OCI runtimes and rather use
	// `spec.Windows.Resources.Storage.Bps`.
	AnnotationContainerStorageQoSBandwidthMaximum = "io.microsoft.container.storage.qos.bandwidthmaximum"
	// AnnotationContainerStorageQoSIopsMaximum overrides the container storage
	// maximum iops set via the OCI spec.
	//
	// Note: This is only present because CRI does not (currently) have a
	// `WindowsPodSandboxConfig` for setting this correctly. It should not be
	// used via OCI runtimes and rather use
	// `spec.Windows.Resources.Storage.Iops`.
	AnnotationContainerStorageQoSIopsMaximum = "io.microsoft.container.storage.qos.iopsmaximum"
	// AnnotationGPUVHDPath overrides the default path to search for the gpu vhd
	AnnotationGPUVHDPath = "io.microsoft.lcow.gpuvhdpath"
	// AnnotationAssignedDeviceKernelDrivers indicates what drivers to install in the pod during device
	// assignment. This value should contain a list of comma separated directories containing all
	// files and information needed to install given driver(s). This may include .sys,
	// .inf, .cer, and/or other files used during standard installation with pnputil.
	AnnotationAssignedDeviceKernelDrivers = "io.microsoft.assigneddevice.kerneldrivers"
	// AnnotationHostProcessInheritUser indicates whether to ignore the username passed in to run a host process
	// container as and instead inherit the user token from the executable that is launching the container process.
	AnnotationHostProcessInheritUser = "microsoft.com/hostprocess-inherit-user"
	// AnnotationHostProcessContainer indicates to launch a host process container (job container in this repository).
	AnnotationHostProcessContainer = "microsoft.com/hostprocess-container"

	AnnotationAllowOvercommit       = "io.microsoft.virtualmachine.computetopology.memory.allowovercommit"
	AnnotationEnableDeferredCommit  = "io.microsoft.virtualmachine.computetopology.memory.enabledeferredcommit"
	AnnotationEnableColdDiscardHint = "io.microsoft.virtualmachine.computetopology.memory.enablecolddiscardhint"
	// annotationMemorySizeInMB overrides the container memory size set via the
	// OCI spec.
	//
	// Note: This annotation is in MB. OCI is in Bytes. When using this override
	// the caller MUST use MB or sizing will be wrong.
	AnnotationMemorySizeInMB         = "io.microsoft.virtualmachine.computetopology.memory.sizeinmb"
	AnnotationMemoryLowMMIOGapInMB   = "io.microsoft.virtualmachine.computetopology.memory.lowmmiogapinmb"
	AnnotationMemoryHighMMIOBaseInMB = "io.microsoft.virtualmachine.computetopology.memory.highmmiobaseinmb"
	AnnotationMemoryHighMMIOGapInMB  = "io.microsoft.virtualmachine.computetopology.memory.highmmiogapinmb"
	// annotationProcessorCount overrides the hypervisor isolated vCPU count set
	// via the OCI spec.
	//
	// Note: Unlike Windows process isolated container QoS Count/Limt/Weight on
	// the UVM are not mutually exclusive and can be set together.
	AnnotationProcessorCount = "io.microsoft.virtualmachine.computetopology.processor.count"
	// annotationProcessorLimit overrides the hypervisor isolated vCPU limit set
	// via the OCI spec.
	//
	// Limit allows values 1 - 100,000 where 100,000 means 100% CPU. (And is the
	// default if omitted)
	//
	// Note: Unlike Windows process isolated container QoS Count/Limt/Weight on
	// the UVM are not mutually exclusive and can be set together.
	AnnotationProcessorLimit = "io.microsoft.virtualmachine.computetopology.processor.limit"
	// annotationProcessorWeight overrides the hypervisor isolated vCPU weight set
	// via the OCI spec.
	//
	// Weight allows values 0 - 10,000. (100 is the default if omitted)
	//
	// Note: Unlike Windows process isolated container QoS Count/Limt/Weight on
	// the UVM are not mutually exclusive and can be set together.
	AnnotationProcessorWeight             = "io.microsoft.virtualmachine.computetopology.processor.weight"
	AnnotationVPMemCount                  = "io.microsoft.virtualmachine.devices.virtualpmem.maximumcount"
	AnnotationVPMemSize                   = "io.microsoft.virtualmachine.devices.virtualpmem.maximumsizebytes"
	AnnotationPreferredRootFSType         = "io.microsoft.virtualmachine.lcow.preferredrootfstype"
	AnnotationBootFilesRootPath           = "io.microsoft.virtualmachine.lcow.bootfilesrootpath"
	AnnotationKernelDirectBoot            = "io.microsoft.virtualmachine.lcow.kerneldirectboot"
	AnnotationVPCIEnabled                 = "io.microsoft.virtualmachine.lcow.vpcienabled"
	AnnotationVPMemNoMultiMapping         = "io.microsoft.virtualmachine.lcow.vpmem.nomultimapping"
	AnnotationStorageQoSBandwidthMaximum  = "io.microsoft.virtualmachine.storageqos.bandwidthmaximum"
	AnnotationStorageQoSIopsMaximum       = "io.microsoft.virtualmachine.storageqos.iopsmaximum"
	AnnotationFullyPhysicallyBacked       = "io.microsoft.virtualmachine.fullyphysicallybacked"
	AnnotationDisableCompartmentNamespace = "io.microsoft.virtualmachine.disablecompartmentnamespace"
	AnnotationVSMBNoDirectMap             = "io.microsoft.virtualmachine.wcow.virtualSMB.nodirectmap"

	// annotation used to specify the cpugroup ID that a UVM should be assigned to
	AnnotationCPUGroupID = "io.microsoft.virtualmachine.cpugroup.id"

	// SaveAsTemplate annotation must be used with a pod & container creation request.
	// If this annotation is present in the request then it will save the UVM (pod)
	// and the container(s) inside it as a template. However, this also means that this
	// pod and the containers inside this pod will permananetly stay in the
	// paused/templated state and can not be resumed again.
	AnnotationSaveAsTemplate = "io.microsoft.virtualmachine.saveastemplate"

	// This annotation should be used when creating a pod or a container from a template.
	// When creating a pod from a template use the ID of the templated pod as the
	// TemplateID and when creating a container use the ID of the templated container as
	// the TemplateID. It is the client's responsibility to make sure that the sandbox
	// within which a cloned container needs to be created must also be created from the
	// same template.
	AnnotationTemplateID         = "io.microsoft.virtualmachine.templateid"
	AnnotationNetworkConfigProxy = "io.microsoft.network.ncproxy"
	AnnotationNcproxyContainerID = "io.microsoft.network.ncproxy.containerid"
)
