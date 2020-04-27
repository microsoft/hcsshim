package uvm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/mergemaps"
	"github.com/Microsoft/hcsshim/internal/oc"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/uvmfolder"
	"github.com/Microsoft/hcsshim/internal/wcow"
	"go.opencensus.io/trace"
)

const (
	PROTECTED_DACL_ALLOW_ALL_TO_SHIM_SDDL_FMT = "D:P(A;;FA;;;%s)"
)

// OptionsWCOW are the set of options passed to CreateWCOW() to create a utility vm.
type OptionsWCOW struct {
	*Options

	LayerFolders []string // Set of folders for base layers and scratch. Ordered from top most read-only through base read-only layer, followed by scratch

	// should this uvm be created with the ability to be saved as a template
	SaveAsTemplateCapable bool
}

// NewDefaultOptionsWCOW creates the default options for a bootable version of
// WCOW. The caller `MUST` set the `LayerFolders` path on the returned value.
//
// `id` the ID of the compute system. If not passed will generate a new GUID.
//
// `owner` the owner of the compute system. If not passed will use the
// executable files name.
func NewDefaultOptionsWCOW(id, owner string) *OptionsWCOW {
	return &OptionsWCOW{
		Options: newDefaultOptions(id, owner),
	}
}

// Finds a scratch Folder for this uvm. If it is not already created by the caller creates
// a new folder.
func getScratchFolder(ctx context.Context, opts *OptionsWCOW) (_ string, err error) {
	// TODO: BUGBUG Remove this. @jhowardmsft
	//       It should be the responsiblity of the caller to do the creation and population.
	//       - Update runhcs too (vm.go).
	//       - Remove comment in function header
	//       - Update tests that rely on this current behaviour.
	// Create the RW scratch in the top-most layer folder, creating the folder if it doesn't already exist.
	scratchFolder := opts.LayerFolders[len(opts.LayerFolders)-1]

	// Create the directory if it doesn't exist
	if _, err = os.Stat(scratchFolder); os.IsNotExist(err) {
		if err = os.MkdirAll(scratchFolder, 0777); err != nil {
			return "", fmt.Errorf("failed to create utility VM scratch folder: %s", err)
		}
	}

	return scratchFolder, nil
}

func prepareConfigDoc(ctx context.Context, uvm *UtilityVM, opts *OptionsWCOW, uvmFolder, scratchPath string) (*hcsschema.ComputeSystem, error) {

	// To maintain compatability with Docker we need to automatically downgrade
	// a user CPU count if the setting is not possible.
	uvm.normalizeProcessorCount(ctx, opts.ProcessorCount)

	// Align the requested memory size.
	memorySizeInMB := uvm.normalizeMemorySize(ctx, opts.MemorySizeInMB)

	doc := &hcsschema.ComputeSystem{
		Owner:                             uvm.owner,
		SchemaVersion:                     schemaversion.SchemaV21(),
		ShouldTerminateOnLastHandleClosed: true,
		VirtualMachine: &hcsschema.VirtualMachine{
			StopOnReset: true,
			Chipset: &hcsschema.Chipset{
				Uefi: &hcsschema.Uefi{
					BootThis: &hcsschema.UefiBootEntry{
						DevicePath: `\EFI\Microsoft\Boot\bootmgfw.efi`,
						DeviceType: "VmbFs",
					},
				},
			},
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
				Processor: &hcsschema.Processor2{
					Count:  uvm.processorCount,
					Limit:  opts.ProcessorLimit,
					Weight: opts.ProcessorWeight,
				},
			},
			Devices: &hcsschema.Devices{
				Scsi: map[string]hcsschema.Scsi{
					"0": {
						Attachments: map[string]hcsschema.Attachment{
							"0": {
								Path:  scratchPath,
								Type_: "VirtualDisk",
							},
						},
					},
				},
				HvSocket: &hcsschema.HvSocket2{
					HvSocketConfig: &hcsschema.HvSocketSystemConfig{
						// Allow administrators and SYSTEM to bind to vsock sockets
						// so that we can create a GCS log socket.
						DefaultBindSecurityDescriptor: "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
					},
				},
				VirtualSmb: &hcsschema.VirtualSmb{
					DirectFileMappingInMB: 1024, // Sensible default, but could be a tuning parameter somewhere
					Shares: []hcsschema.VirtualSmbShare{
						{
							Name: "os",
							Path: filepath.Join(uvmFolder, `UtilityVM\Files`),
							Options: &hcsschema.VirtualSmbShareOptions{
								ReadOnly:            true,
								PseudoOplocks:       true,
								TakeBackupPrivilege: true,
								CacheIo:             true,
								ShareRead:           true,
							},
						},
					},
				},
			},
		},
	}

	if !opts.ExternalGuestConnection {
		doc.VirtualMachine.GuestConnection = &hcsschema.GuestConnection{}
	}

	// Handle StorageQoS if set
	if opts.StorageQoSBandwidthMaximum > 0 || opts.StorageQoSIopsMaximum > 0 {
		doc.VirtualMachine.StorageQoS = &hcsschema.StorageQoS{
			IopsMaximum:      opts.StorageQoSIopsMaximum,
			BandwidthMaximum: opts.StorageQoSBandwidthMaximum,
		}
	}

	return doc, nil
}

func (uvm *UtilityVM) startExternalGcsListener(ctx context.Context) error {
	log.G(ctx).WithField("UVM ID", uvm.runtimeID).Debug("Using external GCS connection for uvm")
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

// CreateWCOW creates an HCS compute system representing a utility VM.
//
// WCOW Notes:
//   - The scratch is always attached to SCSI 0:0
//
func CreateWCOW(ctx context.Context, opts *OptionsWCOW) (_ *UtilityVM, err error) {
	ctx, span := trace.StartSpan(ctx, "uvm::CreateWCOW")
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
	log.G(ctx).WithField("options", fmt.Sprintf("%+v", opts)).Debug("uvm::CreateWCOW options")

	uvm := &UtilityVM{
		id:                  opts.ID,
		owner:               opts.Owner,
		operatingSystem:     "windows",
		scsiControllerCount: 1,
		vsmbDirShares:       make(map[string]*VSMBShare),
		vsmbFileShares:      make(map[string]*VSMBShare),
	}
	defer func() {
		if err != nil {
			uvm.Close()
		}
	}()

	if len(opts.LayerFolders) < 2 {
		return nil, fmt.Errorf("at least 2 LayerFolders must be supplied")
	}

	uvmFolder, err := uvmfolder.LocateUVMFolder(ctx, opts.LayerFolders)
	if err != nil {
		return nil, fmt.Errorf("failed to locate utility VM folder from layer folders: %s", err)
	}

	scratchFolder, err := getScratchFolder(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Create sandbox.vhdx in the scratch folder based on the template, granting the correct permissions to it
	scratchPath := filepath.Join(scratchFolder, "sandbox.vhdx")

	if _, err = os.Stat(scratchPath); os.IsNotExist(err) {
		if err = wcow.CreateUVMScratch(ctx, uvmFolder, scratchFolder, opts.ID); err != nil {
			return nil, fmt.Errorf("failed to create scratch: %s", err)
		}
	}

	doc, err := prepareConfigDoc(ctx, uvm, opts, uvmFolder, scratchPath)
	if err != nil {
		return nil, fmt.Errorf("Error in preparing config doc: %s", err)
	}

	// Add appropriate VSMB share options if this UVM needs to be saved as a template
	if opts.SaveAsTemplateCapable {
		for _, share := range doc.VirtualMachine.Devices.VirtualSmb.Shares {
			share.Options.PseudoDirnotify = true
			share.Options.NoLocks = true
		}
	}

	uvm.scsiLocations[0][0] = &SCSIMount{
		vm:       uvm,
		HostPath: doc.VirtualMachine.Devices.Scsi["0"].Attachments["0"].Path,
		refCount: 1,
	}

	fullDoc, err := mergemaps.MergeJSON(doc, ([]byte)(opts.AdditionHCSDocumentJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to merge additional JSON '%s': %s", opts.AdditionHCSDocumentJSON, err)
	}

	err = uvm.create(ctx, fullDoc)
	if err != nil {
		return nil, err
	}

	if opts.ExternalGuestConnection {
		if err = uvm.startExternalGcsListener(ctx); err != nil {
			return nil, err
		}
	}

	return uvm, nil
}

// CloneWCOW creates an HCS compute system representing a utility VM by creating a clone
// from a given template
//
func CloneWCOW(ctx context.Context, opts *OptionsWCOW, utc *UVMTemplateConfig) (_ *UtilityVM, err error) {
	ctx, span := trace.StartSpan(ctx, "uvm::CloneWCOW")
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
	log.G(ctx).WithField("options", fmt.Sprintf("%+v", opts)).Debug("uvm::CloneeWCOW options")

	uvm := &UtilityVM{
		id:                  opts.ID,
		owner:               opts.Owner,
		operatingSystem:     "windows",
		scsiControllerCount: 1,
		vsmbDirShares:       make(map[string]*VSMBShare),
		vsmbFileShares:      make(map[string]*VSMBShare),
	}
	defer func() {
		if err != nil {
			uvm.Close()
		}
	}()

	if len(opts.LayerFolders) < 2 {
		return nil, fmt.Errorf("at least 2 LayerFolders must be supplied")
	}

	uvmFolder, err := uvmfolder.LocateUVMFolder(ctx, opts.LayerFolders)
	if err != nil {
		return nil, fmt.Errorf("failed to locate utility VM folder from layer folders: %s", err)
	}

	scratchFolder, err := getScratchFolder(ctx, opts)
	if err != nil {
		return nil, err
	}

	// It is okay to pass empty path here becaues in the clone loop below we will
	// anyway overwrite the scsi map.
	doc, err := prepareConfigDoc(ctx, uvm, opts, uvmFolder, "")
	if err != nil {
		return nil, fmt.Errorf("Error in preparing config doc: %s", err)
	}

	doc.VirtualMachine.RestoreState = &hcsschema.RestoreState{}
	doc.VirtualMachine.RestoreState.TemplateSystemId = utc.UVMID

	for _, cloneableResource := range utc.Resources {
		cloneableResource.Clone(ctx, uvm, &CloneData{
			doc:           doc,
			scratchFolder: scratchFolder,
			UVMID:         opts.ID,
		})
	}

	fullDoc, err := mergemaps.MergeJSON(doc, ([]byte)(opts.AdditionHCSDocumentJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to merge additional JSON '%s': %s", opts.AdditionHCSDocumentJSON, err)
	}

	err = uvm.create(ctx, fullDoc)
	if err != nil {
		return nil, err
	}

	uvm.startExternalGcsListener(ctx)

	return uvm, nil
}
