//go:build windows

package uvm

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/gcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/processorinfo"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/security"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/Microsoft/hcsshim/osversion"
)

// OptionsWCOW are the set of options passed to CreateWCOW() to create a utility vm.
type OptionsWCOW struct {
	*Options
	*guestresource.WCOWConfidentialOptions

	BootFiles *WCOWBootFiles

	// NoDirectMap specifies that no direct mapping should be used for any VSMBs added to the UVM
	NoDirectMap bool

	// NoInheritHostTimezone specifies whether to not inherit the hosts timezone for the UVM. UTC will be set as the default for the VM instead.
	NoInheritHostTimezone bool

	// AdditionalRegistryKeys are Registry keys and their values to additionally add to the uVM.
	AdditionalRegistryKeys []hcsschema.RegistryValue
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
		WCOWConfidentialOptions: &guestresource.WCOWConfidentialOptions{
			WCOWSecurityPolicyEnabled: false,
		},
	}
}

func (uvm *UtilityVM) startExternalGcsListener(ctx context.Context) error {
	log.G(ctx).WithField("vmID", uvm.runtimeID).Debug("Using external GCS bridge")

	l, err := winio.ListenHvsock(&winio.HvsockAddr{
		// 1. TODO:
		// Following line is only temporary for POC and ease of developement.
		// "VMID: gcs.HV_GUID_LOOPBACK" means that we are trying to start sidecar
		// outside of the UVM, that is in the host itself. This is only for
		// easy developement.
		VMID: gcs.HV_GUID_LOOPBACK,
		// ORIGINAL: uvm.runtimeID,
		ServiceID: gcs.WindowsSidecarGcsHvsockServiceID,
		// 2. TODO:
		// Following line can be uncommented after POC to ensure that
		// hcsshim connects to gcs-sidecar.exe GUID and NOT to the windows GCS
		// directly and this change should ONLY be for C-WCOW cases.
		// We can base the decision of which GUID the external GCS listener should
		// connect to based on annotations.WindowsSecurityPolicy annotation in pod.json.
		// gcs.WindowsGcsHvsockServiceID,
	})
	if err != nil {
		return err
	}
	uvm.gcListener = l
	return nil
}

func prepareConfigDoc(ctx context.Context, uvm *UtilityVM, opts *OptionsWCOW) (*hcsschema.ComputeSystem, error) {
	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host processor information: %w", err)
	}

	// To maintain compatibility with Docker we need to automatically downgrade
	// a user CPU count if the setting is not possible.
	uvm.processorCount = uvm.normalizeProcessorCount(ctx, opts.ProcessorCount, processorTopology)

	// Align the requested memory size.
	memorySizeInMB := uvm.normalizeMemorySize(ctx, opts.MemorySizeInMB)

	// UVM rootfs share is readonly.
	vsmbOpts := uvm.DefaultVSMBOptions(true)
	vsmbOpts.TakeBackupPrivilege = true
	virtualSMB := &hcsschema.VirtualSmb{
		DirectFileMappingInMB: 1024, // Sensible default, but could be a tuning parameter somewhere
		Shares: []hcsschema.VirtualSmbShare{
			{
				Name:    "os",
				Path:    opts.BootFiles.OSFilesPath,
				Options: vsmbOpts,
			},
		},
	}

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

	// Temporary hack to start up windows sidecar gcs in the uvm
	isCWCOW := true
	/* TODO: temp only for POC/demo. Can be removed once we have pipeline work to
	// consume gcs-sidecar.exe and bring it up as a service during boot time.
	// Till such time, this start gcs-sidecar.exe as a service for every createPod()
	// request.
	if opts.WcowSecurityPolicy != "" {
		isCWCOW = true
	}
	*/
	if isCWCOW {
		registryChanges.AddValues = append(registryChanges.AddValues,
			hcsschema.RegistryValue{
				Key: &hcsschema.RegistryKey{
					Hive: "System",
					Name: "CurrentControlSet\\Services\\gcs-sidecar",
				},
				Name:        "DisplayName",
				StringValue: "gcs-sidecar",
				Type_:       "String",
			},
			hcsschema.RegistryValue{
				Key: &hcsschema.RegistryKey{
					Hive: "System",
					Name: "CurrentControlSet\\Services\\gcs-sidecar",
				},
				Name:       "ErrorControl",
				DWordValue: 1,
				Type_:      "DWord",
			},
			hcsschema.RegistryValue{
				Key: &hcsschema.RegistryKey{
					Hive: "System",
					Name: "CurrentControlSet\\Services\\gcs-sidecar",
				},
				Name:        "ImagePath",
				StringValue: "C:\\Windows\\System32\\gcs-sidecar.exe",
				Type_:       "String",
			},
			hcsschema.RegistryValue{
				Key: &hcsschema.RegistryKey{
					Hive: "System",
					Name: "CurrentControlSet\\Services\\gcs-sidecar",
				},
				Name:        "ObjectName",
				StringValue: "LocalSystem",
				Type_:       "String",
			},
			hcsschema.RegistryValue{
				Key: &hcsschema.RegistryKey{
					Hive: "System",
					Name: "CurrentControlSet\\Services\\gcs-sidecar",
				},
				Name:       "Start",
				DWordValue: 2,
				Type_:      "DWord",
			},
			hcsschema.RegistryValue{
				Key: &hcsschema.RegistryKey{
					Hive: "System",
					Name: "CurrentControlSet\\Services\\gcs-sidecar",
				},
				Name:       "Type",
				DWordValue: 16,
				Type_:      "DWord",
			},
		)
	}

	processor := &hcsschema.VirtualMachineProcessor{
		Count:  uint32(uvm.processorCount),
		Limit:  uint64(opts.ProcessorLimit),
		Weight: uint64(opts.ProcessorWeight),
	}

	numa, numaProcessors, err := prepareVNumaTopology(opts.Options)
	if err != nil {
		return nil, err
	}

	if numa != nil {
		if opts.AllowOvercommit {
			return nil, fmt.Errorf("vNUMA supports only Physical memory backing type")
		}
		if err := validateNumaForVM(numa, processor.Count, memorySizeInMB); err != nil {
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
			StopOnReset: true,
			Chipset: &hcsschema.Chipset{
				Uefi: &hcsschema.Uefi{
					BootThis: &hcsschema.UefiBootEntry{
						DevicePath: filepath.Join(opts.BootFiles.OSRelativeBootDirPath, "bootmgfw.efi"),
						DeviceType: "VmbFs",
					},
				},
			},
			RegistryChanges: &registryChanges,
			ComputeTopology: &hcsschema.Topology{
				Memory: &hcsschema.VirtualMachineMemory{
					SizeInMB:        memorySizeInMB,
					AllowOvercommit: opts.AllowOvercommit,
					// EnableHotHint is not compatible with physical.
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
				VirtualSmb: virtualSMB,
			},
		},
	}

	// Expose ACPI information into UVM
	if numa != nil || numaProcessors != nil {
		firmwareFallbackMeasured := hcsschema.VirtualSlitType_FIRMWARE_FALLBACK_MEASURED
		doc.VirtualMachine.ComputeTopology.Memory.SlitType = &firmwareFallbackMeasured
	}

	maps.Copy(doc.VirtualMachine.Devices.HvSocket.HvSocketConfig.ServiceTable, opts.AdditionalHyperVConfig)

	// Handle StorageQoS if set
	if opts.StorageQoSBandwidthMaximum > 0 || opts.StorageQoSIopsMaximum > 0 {
		doc.VirtualMachine.StorageQoS = &hcsschema.StorageQoS{
			IopsMaximum:      opts.StorageQoSIopsMaximum,
			BandwidthMaximum: opts.StorageQoSBandwidthMaximum,
		}
	}

	// Set boot options
	if opts.DumpDirectoryPath != "" {
		if info, err := os.Stat(opts.DumpDirectoryPath); err != nil {
			return nil, fmt.Errorf("failed to stat dump directory %s: %w", opts.DumpDirectoryPath, err)
		} else if !info.IsDir() {
			return nil, fmt.Errorf("dump directory path %s isn't a directory", opts.DumpDirectoryPath)
		}
		if err := security.GrantVmGroupAccessWithMask(opts.DumpDirectoryPath, security.AccessMaskAll); err != nil {
			return nil, fmt.Errorf("failed to add SDL to dump directory: %w", err)
		}
		debugOpts := &hcsschema.DebugOptions{
			BugcheckSavedStateFileName:            filepath.Join(opts.DumpDirectoryPath, fmt.Sprintf("%s-savedstate.vmrs", uvm.id)),
			BugcheckNoCrashdumpSavedStateFileName: filepath.Join(opts.DumpDirectoryPath, fmt.Sprintf("%s-savedstate_nocrashdump.vmrs", uvm.id)),
		}
		doc.VirtualMachine.DebugOptions = debugOpts
	}

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
		id:                         opts.ID,
		owner:                      opts.Owner,
		operatingSystem:            "windows",
		scsiControllerCount:        opts.SCSIControllerCount,
		vsmbDirShares:              make(map[string]*VSMBShare),
		vsmbFileShares:             make(map[string]*VSMBShare),
		vpciDevices:                make(map[VPCIDeviceID]*VPCIDevice),
		noInheritHostTimezone:      opts.NoInheritHostTimezone,
		physicallyBacked:           !opts.AllowOvercommit,
		devicesPhysicallyBacked:    opts.FullyPhysicallyBacked,
		vsmbNoDirectMap:            opts.NoDirectMap,
		noWritableFileShares:       opts.NoWritableFileShares,
		createOpts:                 *opts,
		WCOWconfidentialUVMOptions: opts.WCOWConfidentialOptions,
	}

	defer func() {
		if err != nil {
			uvm.Close()
		}
	}()

	if err := verifyOptions(ctx, opts); err != nil {
		return nil, errors.Wrap(err, errBadUVMOpts.Error())
	}

	doc, err := prepareConfigDoc(ctx, uvm, opts)
	if err != nil {
		return nil, fmt.Errorf("error in preparing config doc: %w", err)
	}

	if err := wclayer.GrantVmAccess(ctx, uvm.id, opts.BootFiles.ScratchVHDPath); err != nil {
		return nil, errors.Wrap(err, "failed to grant vm access to scratch")
	}

	doc.VirtualMachine.Devices.Scsi = map[string]hcsschema.Scsi{}
	for i := 0; i < int(uvm.scsiControllerCount); i++ {
		doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[i]] = hcsschema.Scsi{
			Attachments: make(map[string]hcsschema.Attachment),
		}
	}

	doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[0]].Attachments["0"] = hcsschema.Attachment{

		Path:  opts.BootFiles.ScratchVHDPath,
		Type_: "VirtualDisk",
	}

	uvm.reservedSCSISlots = append(uvm.reservedSCSISlots, scsi.Slot{Controller: 0, LUN: 0})

	err = uvm.create(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("error while creating the compute system: %w", err)
	}

	if err = uvm.startExternalGcsListener(ctx); err != nil {
		return nil, err
	}

	uvm.ncProxyClientAddress = opts.NetworkConfigProxy

	return uvm, nil
}
