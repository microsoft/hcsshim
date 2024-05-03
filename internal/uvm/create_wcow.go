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
	"go.opentelemetry.io/otel/attribute"

	"github.com/Microsoft/hcsshim/internal/gcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/internal/processorinfo"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/security"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/Microsoft/hcsshim/osversion"
)

// OptionsWCOW are the set of options passed to CreateWCOW() to create a utility vm.
type OptionsWCOW struct {
	*Options

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
	}
}

func (uvm *UtilityVM) startExternalGcsListener(ctx context.Context) error {
	log.G(ctx).WithField("vmID", uvm.runtimeID).Debug("Using external GCS bridge")

	l, err := winio.ListenHvsock(&winio.HvsockAddr{
		VMID:      uvm.runtimeID,
		ServiceID: gcs.WindowsGcsHvsockServiceID,
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

	processor := &hcsschema.Processor2{
		Count:  uvm.processorCount,
		Limit:  opts.ProcessorLimit,
		Weight: opts.ProcessorWeight,
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
				Memory: &hcsschema.Memory2{
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
	ctx, span := otelutil.StartSpan(ctx, "uvm::CreateWCOW")
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if opts.ID == "" {
		g, err := guid.NewV4()
		if err != nil {
			return nil, err
		}
		opts.ID = g.String()
	}

	span.SetAttributes(attribute.String(logfields.UVMID, opts.ID))
	log.G(ctx).WithField("options", log.Format(ctx, opts)).Debug("uvm::CreateWCOW options")

	uvm := &UtilityVM{
		id:                      opts.ID,
		owner:                   opts.Owner,
		operatingSystem:         "windows",
		scsiControllerCount:     opts.SCSIControllerCount,
		vsmbDirShares:           make(map[string]*VSMBShare),
		vsmbFileShares:          make(map[string]*VSMBShare),
		vpciDevices:             make(map[VPCIDeviceKey]*VPCIDevice),
		noInheritHostTimezone:   opts.NoInheritHostTimezone,
		physicallyBacked:        !opts.AllowOvercommit,
		devicesPhysicallyBacked: opts.FullyPhysicallyBacked,
		vsmbNoDirectMap:         opts.NoDirectMap,
		noWritableFileShares:    opts.NoWritableFileShares,
		createOpts:              *opts,
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
