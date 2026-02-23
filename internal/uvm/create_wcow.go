//go:build windows

package uvm

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/go-winio/vhd"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/processorinfo"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/security"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

var (
	// A predefined GUID for UtilityVMs to identify a scratch VHD that is completely empty/unformatted.
	// This GUID is set in the metadata of the VHD and thus can be reliably used to identify the disk.
	// a7b3c5d1-4e2f-4a8b-9c6d-1e3f5a7b9c2d
	unformattedScratchIdentifier = &guid.GUID{
		Data1: 0xa7b3c5d1,
		Data2: 0x4e2f,
		Data3: 0x4a8b,
		Data4: [8]byte{0x9c, 0x6d, 0x1e, 0x3f, 0x5a, 0x7b, 0x9c, 0x2d},
	}
)

type ConfidentialWCOWOptions struct {
	*ConfidentialCommonOptions
	/* Below options are only included for testing/debugging purposes - shouldn't be used in regular scenarios */
	IsolationType      string
	DisableSecureBoot  bool
	FirmwareParameters string
	WritableEFI        bool
}

// OptionsWCOW are the set of options passed to CreateWCOW() to create a utility vm.
type OptionsWCOW struct {
	*Options
	*ConfidentialWCOWOptions

	BootFiles *WCOWBootFiles

	// NoDirectMap specifies that no direct mapping should be used for any VSMBs added to the UVM
	NoDirectMap bool

	// NoInheritHostTimezone specifies whether to not inherit the hosts timezone for the UVM. UTC will be set as the default for the VM instead.
	NoInheritHostTimezone bool

	// AdditionalRegistryKeys are Registry keys and their values to additionally add to the uVM.
	AdditionalRegistryKeys []hcsschema.RegistryValue

	OutputHandlerCreator OutputHandlerCreator // Creates an [OutputHandler] that controls how output received over HVSocket from the UVM is handled. Defaults to parsing output as ETW Log events
	LogSources           string               // ETW providers to be set for the logging service
	ForwardLogs          bool                 // Whether to forward logs to the host or not
}

func defaultConfidentialWCOWOSBootFilesPath() string {
	return filepath.Join(filepath.Dir(os.Args[0]), "WindowsBootFiles", "confidential")
}

func GetDefaultConfidentialVMGSPath() string {
	return filepath.Join(defaultConfidentialWCOWOSBootFilesPath(), "cwcow.snp.vmgs")
}

func GetDefaultConfidentialBootCIMPath() string {
	return filepath.Join(defaultConfidentialWCOWOSBootFilesPath(), "rootfs.vhd")
}

func GetDefaultConfidentialEFIPath() string {
	return filepath.Join(defaultConfidentialWCOWOSBootFilesPath(), "boot.vhd")
}

func GetDefaultReferenceInfoFilePath() string {
	return filepath.Join(defaultConfidentialWCOWOSBootFilesPath(), "reference_info.cose")
}

// NewDefaultOptionsWCOW creates the default options for a bootable version of
// WCOW. The caller `MUST` set the `BootFiles` on the returned value.
//
// `id` the ID of the compute system. If not passed will generate a new GUID.
//
// `owner` the owner of the compute system. If not passed will use the
// executable files name.
func NewDefaultOptionsWCOW(id, owner string) *OptionsWCOW {
	return &OptionsWCOW{
		Options:                newDefaultOptions(id, owner),
		AdditionalRegistryKeys: []hcsschema.RegistryValue{},
		ConfidentialWCOWOptions: &ConfidentialWCOWOptions{
			ConfidentialCommonOptions: &ConfidentialCommonOptions{
				SecurityPolicyEnabled: false,
			},
		},
		OutputHandlerCreator: parseLogrus,
		ForwardLogs:          true, // Default to true for WCOW, and set to false for CWCOW in internal/oci/uvm.go SpecToUVMCreateOpts
		LogSources:           "",
	}
}

// startExternalGcsListener connects to the GCS service running inside the
// UVM. gcsServiceID can either be the service ID of the default GCS that is present in
// all UtilityVMs or it can be the service ID of the sidecar GCS that is used mostly in
// confidential mode.
func (uvm *UtilityVM) startExternalGcsListener(ctx context.Context, gcsServiceID guid.GUID) error {
	log.G(ctx).WithFields(logrus.Fields{
		"vmID":         uvm.runtimeID,
		"gcsServiceID": gcsServiceID.String(),
	}).Debug("Using external GCS bridge")

	l, err := winio.ListenHvsock(&winio.HvsockAddr{
		VMID:      uvm.runtimeID,
		ServiceID: gcsServiceID,
	})

	if err != nil {
		return err
	}
	uvm.gcListener = l
	return nil
}

func prepareCommonConfigDoc(ctx context.Context, uvm *UtilityVM, opts *OptionsWCOW) (*hcsschema.ComputeSystem, error) {
	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host processor information: %w", err)
	}

	// To maintain compatibility with Docker we need to automatically downgrade
	// a user CPU count if the setting is not possible.
	uvm.processorCount = vmutils.NormalizeProcessorCount(ctx, uvm.id, opts.ProcessorCount, processorTopology)

	// Align the requested memory size.
	memorySizeInMB := vmutils.NormalizeMemorySize(ctx, uvm.id, opts.MemorySizeInMB)

	var registryChanges hcsschema.RegistryChanges
	// We're getting asked to setup local dump collection for WCOW. We need to:
	//
	// 1. Turn off WER reporting, so we don't both upload the dump and save a local copy.
	// 2. Set WerSvc to start when the UVM starts to work around a bug when generating dumps for certain exceptions.
	// https://github.com/microsoft/Windows-Containers/issues/60#issuecomment-834633192
	// This supposedly should be fixed soon but for now keep this until we know which container images
	// (1809, 1903/9, 2004 etc.) this went out too.
	if opts.ProcessDumpLocation != "" {
		uvm.processDumpLocation = opts.ProcessDumpLocation
		registryChanges.AddValues = append(registryChanges.AddValues,
			hcsschema.RegistryValue{
				Key: &hcsschema.RegistryKey{
					Hive: hcsschema.RegistryHive_SYSTEM,
					Name: "ControlSet001\\Services\\WerSvc",
				},
				Name:       "Start",
				DWordValue: 2,
				Type_:      hcsschema.RegistryValueType_D_WORD,
			},
			hcsschema.RegistryValue{
				Key: &hcsschema.RegistryKey{
					Hive: hcsschema.RegistryHive_SOFTWARE,
					Name: "Microsoft\\Windows\\Windows Error Reporting",
				},
				Name:       "Disabled",
				DWordValue: 1,
				Type_:      hcsschema.RegistryValueType_D_WORD,
			},
		)
	}

	// Here for a temporary workaround until the need for setting this regkey is no more. To protect
	// against any undesired behavior (such as some general networking scenarios ceasing to function)
	// with a recent change to fix SMB share access in the UVM, this registry key will be checked to
	// enable the change in question inside GNS.dll.
	if !opts.DisableCompartmentNamespace {
		registryChanges.AddValues = append(registryChanges.AddValues,
			hcsschema.RegistryValue{
				Key: &hcsschema.RegistryKey{
					Hive: hcsschema.RegistryHive_SYSTEM,
					Name: "CurrentControlSet\\Services\\gns",
				},
				Name:       "EnableCompartmentNamespace",
				DWordValue: 1,
				Type_:      hcsschema.RegistryValueType_D_WORD,
			},
		)
	}

	registryChanges.AddValues = append(registryChanges.AddValues, opts.AdditionalRegistryKeys...)

	processor := &hcsschema.VirtualMachineProcessor{
		Count:  uint32(uvm.processorCount),
		Limit:  uint64(opts.ProcessorLimit),
		Weight: uint64(opts.ProcessorWeight),
	}

	numa, numaProcessors, err := vmutils.PrepareVNumaTopology(ctx, &vmutils.NumaConfig{
		MaxProcessorsPerNumaNode:   opts.MaxProcessorsPerNumaNode,
		MaxMemorySizePerNumaNode:   opts.MaxMemorySizePerNumaNode,
		PreferredPhysicalNumaNodes: opts.PreferredPhysicalNumaNodes,
		NumaMappedPhysicalNodes:    opts.NumaMappedPhysicalNodes,
		NumaProcessorCounts:        opts.NumaProcessorCounts,
		NumaMemoryBlocksCounts:     opts.NumaMemoryBlocksCounts,
	})
	if err != nil {
		return nil, err
	}

	if numa != nil {
		if opts.AllowOvercommit {
			return nil, fmt.Errorf("vNUMA supports only Physical memory backing type")
		}
		if err := vmutils.ValidateNumaForVM(numa, processor.Count, memorySizeInMB); err != nil {
			return nil, fmt.Errorf("failed to validate vNUMA settings: %w", err)
		}
	}

	if numaProcessors != nil {
		processor.NumaProcessorsSettings = numaProcessors
	}

	// We can set a cpu group for the VM at creation time in recent builds.
	if opts.CPUGroupID != "" {
		if osversion.Build() < osversion.V21H1 {
			return nil, errCPUGroupCreateNotSupported
		}
		processor.CpuGroup = &hcsschema.CpuGroup{Id: opts.CPUGroupID}
	}

	doc := &hcsschema.ComputeSystem{
		Owner:                             uvm.owner,
		SchemaVersion:                     schemaversion.SchemaV21(),
		ShouldTerminateOnLastHandleClosed: true,
		VirtualMachine: &hcsschema.VirtualMachine{
			StopOnReset:     true,
			RegistryChanges: &registryChanges,
			ComputeTopology: &hcsschema.Topology{
				Memory: &hcsschema.VirtualMachineMemory{
					SizeInMB:             memorySizeInMB,
					AllowOvercommit:      opts.AllowOvercommit,
					EnableHotHint:        opts.AllowOvercommit,
					EnableDeferredCommit: opts.EnableDeferredCommit,
					LowMMIOGapInMB:       opts.LowMMIOGapInMB,
					HighMMIOBaseInMB:     opts.HighMMIOBaseInMB,
					HighMMIOGapInMB:      opts.HighMMIOGapInMB,
				},
				Processor: processor,
				Numa:      numa,
			},
			Devices: &hcsschema.Devices{
				HvSocket: &hcsschema.HvSocket2{
					HvSocketConfig: &hcsschema.HvSocketSystemConfig{
						// Allow administrators and SYSTEM to bind to hyper-v sockets
						// so that we can communicate to the GCS.
						DefaultBindSecurityDescriptor: "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
						ServiceTable:                  make(map[string]hcsschema.HvSocketServiceConfig),
					},
				},
				VirtualSmb: &hcsschema.VirtualSmb{
					DirectFileMappingInMB: 1024, // Sensible default, but could be a tuning parameter somewhere
				},
			},
		},
	}

	// Expose ACPI information into UVM
	if numa != nil || numaProcessors != nil {
		firmwareFallbackMeasured := hcsschema.VirtualSlitType_FIRMWARE_FALLBACK_MEASURED
		doc.VirtualMachine.ComputeTopology.Memory.SlitType = &firmwareFallbackMeasured
	}

	if opts.ResourcePartitionID != nil {
		// TODO (maksiman): assign pod to resource partition and potentially do an OS version check before that
		log.G(ctx).WithField("resource-partition-id", opts.ResourcePartitionID.String()).Debug("setting resource partition ID")
	}

	maps.Copy(doc.VirtualMachine.Devices.HvSocket.HvSocketConfig.ServiceTable, opts.AdditionalHyperVConfig)
	if opts.ForwardLogs {
		key := prot.WindowsLoggingHvsockServiceID.String()
		doc.VirtualMachine.Devices.HvSocket.HvSocketConfig.ServiceTable[key] = hcsschema.HvSocketServiceConfig{
			AllowWildcardBinds:        true,
			BindSecurityDescriptor:    "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
			ConnectSecurityDescriptor: "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
		}
	}

	// Handle StorageQoS if set
	if opts.StorageQoSBandwidthMaximum > 0 || opts.StorageQoSIopsMaximum > 0 {
		doc.VirtualMachine.StorageQoS = &hcsschema.StorageQoS{
			IopsMaximum:      opts.StorageQoSIopsMaximum,
			BandwidthMaximum: opts.StorageQoSBandwidthMaximum,
		}
	}

	if opts.ConsolePipe != "" {
		doc.VirtualMachine.Devices.ComPorts = map[string]hcsschema.ComPort{
			"0": {
				NamedPipe: opts.ConsolePipe,
			},
		}
	}

	if opts.EnableGraphicsConsole {
		doc.VirtualMachine.Devices.Keyboard = &hcsschema.Keyboard{}
		doc.VirtualMachine.Devices.EnhancedModeVideo = &hcsschema.EnhancedModeVideo{}
		doc.VirtualMachine.Devices.VideoMonitor = &hcsschema.VideoMonitor{}
	}

	// Set crash dump options
	if opts.DumpDirectoryPath != "" {
		if info, err := os.Stat(opts.DumpDirectoryPath); err != nil {
			return nil, fmt.Errorf("failed to stat dump directory %s: %w", opts.DumpDirectoryPath, err)
		} else if !info.IsDir() {
			return nil, fmt.Errorf("dump directory path %s isn't a directory", opts.DumpDirectoryPath)
		}
		if err := security.GrantVmGroupAccessWithMask(opts.DumpDirectoryPath, security.AccessMaskAll); err != nil {
			return nil, fmt.Errorf("failed to add SDL to dump directory: %w", err)
		}
		doc.VirtualMachine.DebugOptions = &hcsschema.DebugOptions{
			BugcheckSavedStateFileName:            filepath.Join(opts.DumpDirectoryPath, fmt.Sprintf("%s-savedstate.vmrs", uvm.id)),
			BugcheckNoCrashdumpSavedStateFileName: filepath.Join(opts.DumpDirectoryPath, fmt.Sprintf("%s-savedstate_nocrashdump.vmrs", uvm.id)),
		}

		doc.VirtualMachine.Devices.GuestCrashReporting = &hcsschema.GuestCrashReporting{
			WindowsCrashSettings: &hcsschema.WindowsCrashReporting{
				DumpFileName: filepath.Join(opts.DumpDirectoryPath, fmt.Sprintf("%s-windows-crash", uvm.id)),
				DumpType:     "Full",
			},
		}
	}

	doc.VirtualMachine.Devices.Scsi = map[string]hcsschema.Scsi{}
	for i := 0; i < int(uvm.scsiControllerCount); i++ {
		doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[i]] = hcsschema.Scsi{
			Attachments: make(map[string]hcsschema.Attachment),
		}
	}

	return doc, nil
}

func prepareSecurityConfigDoc(ctx context.Context, uvm *UtilityVM, opts *OptionsWCOW) (*hcsschema.ComputeSystem, error) {
	if opts.BootFiles.BootType != BlockCIMBoot {
		return nil, fmt.Errorf("expected BlockCIM boot type, found: %d", opts.BootFiles.BootType)
	}

	doc, err := prepareCommonConfigDoc(ctx, uvm, opts)
	if err != nil {
		return nil, err
	}

	if opts.DisableSecureBoot {
		doc.VirtualMachine.Chipset = &hcsschema.Chipset{
			Uefi: &hcsschema.Uefi{
				BootThis:                nil,
				ApplySecureBootTemplate: "Skip",
			},
		}
	} else {
		doc.VirtualMachine.Chipset = &hcsschema.Chipset{
			Uefi: &hcsschema.Uefi{
				BootThis:                nil,
				ApplySecureBootTemplate: "Apply",
				SecureBootTemplateId:    "1734c6e8-3154-4dda-ba5f-a874cc483422", // aka MicrosoftWindowsSecureBootTemplateGUID equivalent to "Microsoft Windows" template from Get-VMHost | select SecureBootTemplates,
			},
		}
	}

	policyDigest, err := securitypolicy.NewSecurityPolicyDigest(opts.SecurityPolicy)
	if err != nil {
		return nil, err
	}

	// HCS API expect a base64 encoded string as LaunchData. Internally it
	// decodes it to bytes. SEV later returns the decoded byte blob as HostData
	// field of the report.
	hostData := base64.StdEncoding.EncodeToString(policyDigest)

	enableHCL := true
	doc.VirtualMachine.SecuritySettings = &hcsschema.SecuritySettings{
		EnableTpm: false, // TPM MUST always remain false in confidential mode as per the design
		Isolation: &hcsschema.IsolationSettings{
			IsolationType: "SecureNestedPaging",
			HclEnabled:    &enableHCL,
			LaunchData:    hostData,
		},
	}

	if opts.IsolationType != "" {
		doc.VirtualMachine.SecuritySettings.Isolation.IsolationType = opts.IsolationType
	}

	if err := wclayer.GrantVmAccess(ctx, uvm.id, opts.GuestStateFilePath); err != nil {
		return nil, errors.Wrap(err, "failed to grant vm access to guest state file")
	}

	doc.VirtualMachine.GuestState = &hcsschema.GuestState{
		GuestStateFilePath: opts.GuestStateFilePath,
		GuestStateFileType: "BlockStorage",
	}

	if opts.FirmwareParameters != "" {
		doc.VirtualMachine.Chipset.FirmwareFile = &hcsschema.FirmwareFile{
			Parameters: []byte(opts.FirmwareParameters),
		}
	}

	doc.SchemaVersion = schemaversion.SchemaV25()
	// VM Version 12 is the min version that supports the various SNP features.
	doc.VirtualMachine.Version = &hcsschema.Version{
		Major: 12,
		Minor: 0,
	}

	// TODO(ambarve): only scratch VHD is unique per VM, EFI & Boot CIM VHDs are
	// shared across UVMs, so we don't need to assign VM group access to them every
	// time. It should have been done once while deploying the package.
	if err := wclayer.GrantVmAccess(ctx, uvm.id, opts.BootFiles.BlockCIMFiles.EFIVHDPath); err != nil {
		return nil, errors.Wrap(err, "failed to grant vm access to EFI VHD")
	}

	if err := wclayer.GrantVmAccess(ctx, uvm.id, opts.BootFiles.BlockCIMFiles.BootCIMVHDPath); err != nil {
		return nil, errors.Wrap(err, "failed to grant vm access to Boot CIM VHD")
	}

	if err := wclayer.GrantVmAccess(ctx, uvm.id, opts.GuestStateFilePath); err != nil {
		return nil, errors.Wrap(err, "failed to grant vm access to guest state file")
	}

	if err := wclayer.GrantVmAccess(ctx, uvm.id, opts.BootFiles.BlockCIMFiles.ScratchVHDPath); err != nil {
		return nil, errors.Wrap(err, "failed to grant vm access to scratch VHD")
	}

	if err = vhd.SetVirtualDiskIdentifier(opts.BootFiles.BlockCIMFiles.ScratchVHDPath, *unformattedScratchIdentifier); err != nil {
		return nil, fmt.Errorf("set scratch VHD identifier: %w", err)
	}

	// boot depends on scratch being attached at LUN 0, it MUST ALWAYS remain at LUN 0
	doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[0]].Attachments["0"] = hcsschema.Attachment{
		Path:  opts.BootFiles.BlockCIMFiles.ScratchVHDPath,
		Type_: "VirtualDisk",
	}

	doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[0]].Attachments["1"] = hcsschema.Attachment{
		Path:     opts.BootFiles.BlockCIMFiles.EFIVHDPath,
		Type_:    "VirtualDisk",
		ReadOnly: !opts.WritableEFI,
	}

	doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[0]].Attachments["2"] = hcsschema.Attachment{
		Path:     opts.BootFiles.BlockCIMFiles.BootCIMVHDPath,
		Type_:    "VirtualDisk",
		ReadOnly: true,
	}

	uvm.reservedSCSISlots = append(uvm.reservedSCSISlots,
		scsi.Slot{Controller: 0, LUN: 0},
		scsi.Slot{Controller: 0, LUN: 1},
		scsi.Slot{Controller: 0, LUN: 2})

	vsmbOpts := &hcsschema.VirtualSmbShareOptions{
		ReadOnly:  true,
		ShareRead: true,
		NoOplocks: true,
	}

	// Construct a per-VM share directory relative to the bundle. The directory EmptyDoNotModify should be left empty.
	sharePath := filepath.Join(opts.BundleDirectory, "EmptyDoNotModify")

	// Ensure the directory exists.
	if err := os.MkdirAll(sharePath, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create VSMB default empty share directory %q: %w", sharePath, err)
	}

	if err := wclayer.GrantVmAccess(ctx, uvm.id, sharePath); err != nil {
		return nil, errors.Wrap(err, "failed to grant vm access to VSMB default empty share directory")
	}

	doc.VirtualMachine.Devices.VirtualSmb.Shares = []hcsschema.VirtualSmbShare{{
		Name:    "defaultEmptyShare",
		Path:    sharePath,
		Options: vsmbOpts,
	}}

	return doc, nil
}

func prepareConfigDoc(ctx context.Context, uvm *UtilityVM, opts *OptionsWCOW) (*hcsschema.ComputeSystem, error) {
	if opts.BootFiles.BootType != VmbFSBoot {
		return nil, fmt.Errorf("expected VmbFS boot type, found: %d", opts.BootFiles.BootType)
	}

	doc, err := prepareCommonConfigDoc(ctx, uvm, opts)
	if err != nil {
		return nil, err
	}

	vsmbOpts := uvm.DefaultVSMBOptions(true)
	vsmbOpts.TakeBackupPrivilege = true
	doc.VirtualMachine.Devices.VirtualSmb.Shares = []hcsschema.VirtualSmbShare{{
		Name:    "os",
		Path:    opts.BootFiles.VmbFSFiles.OSFilesPath,
		Options: vsmbOpts,
	}}

	doc.VirtualMachine.Chipset = &hcsschema.Chipset{
		Uefi: &hcsschema.Uefi{
			BootThis: &hcsschema.UefiBootEntry{
				DevicePath: filepath.Join(opts.BootFiles.VmbFSFiles.OSRelativeBootDirPath, "bootmgfw.efi"),
				DeviceType: "VmbFs",
			},
		},
	}

	if err := wclayer.GrantVmAccess(ctx, uvm.id, opts.BootFiles.VmbFSFiles.ScratchVHDPath); err != nil {
		return nil, errors.Wrap(err, "failed to grant vm access to scratch")
	}

	doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[0]].Attachments["0"] = hcsschema.Attachment{
		Path:  opts.BootFiles.VmbFSFiles.ScratchVHDPath,
		Type_: "VirtualDisk",
	}
	uvm.reservedSCSISlots = append(uvm.reservedSCSISlots, scsi.Slot{Controller: 0, LUN: 0})

	return doc, nil
}

// CreateWCOW creates an HCS compute system representing a utility VM.
//
// WCOW Notes:
//   - The scratch is always attached to SCSI 0:0
func CreateWCOW(ctx context.Context, opts *OptionsWCOW) (_ *UtilityVM, err error) {
	ctx, span := oc.StartSpan(ctx, "uvm::CreateWCOW")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	if opts.ID == "" {
		g, err := guid.NewV4()
		if err != nil {
			return nil, err
		}
		opts.ID = g.String()
	}

	span.AddAttributes(trace.StringAttribute(logfields.UVMID, opts.ID))
	log.G(ctx).WithField("options", log.Format(ctx, opts)).Debug("uvm::CreateWCOW options")

	uvm := &UtilityVM{
		id:                      opts.ID,
		owner:                   opts.Owner,
		operatingSystem:         "windows",
		scsiControllerCount:     opts.SCSIControllerCount,
		vsmbDirShares:           make(map[string]*VSMBShare),
		vsmbFileShares:          make(map[string]*VSMBShare),
		vpciDevices:             make(map[VPCIDeviceID]*VPCIDevice),
		noInheritHostTimezone:   opts.NoInheritHostTimezone,
		physicallyBacked:        !opts.AllowOvercommit,
		devicesPhysicallyBacked: opts.FullyPhysicallyBacked,
		vsmbNoDirectMap:         opts.NoDirectMap,
		noWritableFileShares:    opts.NoWritableFileShares,
		createOpts:              opts,
		blockCIMMounts:          make(map[string]*UVMMountedBlockCIMs),
		logSources:              opts.LogSources,
		forwardLogs:             opts.ForwardLogs,
	}

	defer func() {
		if err != nil {
			uvm.Close()
		}
	}()

	if err := verifyOptions(ctx, opts); err != nil {
		return nil, errors.Wrap(err, errBadUVMOpts.Error())
	}

	var doc *hcsschema.ComputeSystem
	if opts.SecurityPolicyEnabled {
		doc, err = prepareSecurityConfigDoc(ctx, uvm, opts)
		if logrus.IsLevelEnabled(logrus.TraceLevel) {
			log.G(ctx).WithFields(logrus.Fields{
				"doc":           log.Format(ctx, doc),
				logrus.ErrorKey: err,
			}).Trace("CreateWCOW prepareSecurityConfigDoc")
		}
	} else {
		doc, err = prepareConfigDoc(ctx, uvm, opts)
		if logrus.IsLevelEnabled(logrus.TraceLevel) {
			log.G(ctx).WithFields(logrus.Fields{
				"doc":           log.Format(ctx, doc),
				logrus.ErrorKey: err,
			}).Trace("CreateWCOW prepareConfigDoc")
		}
	}
	if err != nil {
		return nil, fmt.Errorf("error in preparing config doc: %w", err)
	}

	err = uvm.create(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("error while creating the compute system: %w", err)
	}

	if opts.ForwardLogs {
		// Create a socket that the executed program can send to. This is usually
		// used by Log Forward Service to send log data.
		uvm.outputHandler = opts.OutputHandlerCreator(opts.Options)
		uvm.outputProcessingDone = make(chan struct{})
		uvm.outputListener, err = winio.ListenHvsock(&winio.HvsockAddr{
			VMID:      uvm.RuntimeID(),
			ServiceID: prot.WindowsLoggingHvsockServiceID,
		})
	}

	gcsServiceID := prot.WindowsGcsHvsockServiceID
	if opts.SecurityPolicyEnabled {
		gcsServiceID = prot.WindowsSidecarGcsHvsockServiceID
	}

	if err = uvm.startExternalGcsListener(ctx, gcsServiceID); err != nil {
		return nil, err
	}

	uvm.ncProxyClientAddress = opts.NetworkConfigProxy

	return uvm, nil
}
