package uvm

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/mergemaps"
	"github.com/Microsoft/hcsshim/internal/oc"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/uvmfolder"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/Microsoft/hcsshim/internal/wcow"
	"go.opencensus.io/trace"
	"golang.org/x/sys/windows"
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

// Creates a scratch Folder for this uvm if it is not already created by the caller.
// Then creates a scratch VHDX inside that folder. This scratch VHDX can either be newly
// created (denoted by passing an empty string for copyFrom parameter) or it can be copied
// from an existing VHDX (for cloning scenario) by providing a path of that source VHDX
// in copyFrom parameter.
func createScratchVhdx(ctx context.Context, opts *OptionsWCOW, uvmFolder, copyFrom string) (_ string, _ string, err error) {
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
			return "", "", fmt.Errorf("failed to create utility VM scratch folder: %s", err)
		}
	}

	// Create sandbox.vhdx in the scratch folder based on the template, granting the correct permissions to it
	scratchPath := filepath.Join(scratchFolder, "sandbox.vhdx")

	if copyFrom == "" {
		if _, err = os.Stat(scratchPath); os.IsNotExist(err) {
			if err = wcow.CreateUVMScratch(ctx, uvmFolder, scratchFolder, opts.ID); err != nil {
				return "", "", fmt.Errorf("failed to create scratch: %s", err)
			}
		}
	} else {
		// copy vhdx from the template
		err = copyfile.CopyFile(ctx, copyFrom, scratchPath, true)
		if err != nil {
			return "", "", fmt.Errorf("failed to create a copy of VHD at %s : %s", copyFrom, err)
		}
		if err = wclayer.GrantVmAccess(ctx, opts.ID, scratchPath); err != nil {
			os.Remove(scratchPath)
			return "", "", fmt.Errorf("failed to grant access to %s : %s", scratchPath, err)
		}
	}

	return scratchFolder, scratchPath, nil
}

// Get the SID associated with the current process and adds it into the DACL and returns that
// string
func getSecurityDescriptorForExternalGuestConnection() (string, error) {
	token := windows.GetCurrentProcessToken()
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("Error when getting the current toekn user: %s", err)
	}
	sidStr, err := tokenUser.User.Sid.String()
	if err != nil {
		return "", fmt.Errorf("Error converting SID to string: %s", err)
	}
	descriptor := fmt.Sprintf(PROTECTED_DACL_ALLOW_ALL_TO_SHIM_SDDL_FMT, sidStr)
	return descriptor, nil

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
						ServiceTable:                  map[string]hcsschema.HvSocketServiceConfig{},
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
		// If using external guest connection setup the SID of this process in
		// config doc so that connections from this process are allowed by the GCS.
		// This is specific to WCOW.
		securityDescriptor, err := getSecurityDescriptorForExternalGuestConnection()
		if err != nil {
			return nil, err
		}
		doc.VirtualMachine.Devices.HvSocket.HvSocketConfig.ServiceTable[gcs.WindowsGcsHvsockServiceID.String()] = hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor: securityDescriptor,
		}
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

	_, scratchPath, err := createScratchVhdx(ctx, opts, uvmFolder, "")
	if err != nil {
		return nil, err
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
		l, err := winio.ListenHvsock(&winio.HvsockAddr{
			VMID:      uvm.runtimeID,
			ServiceID: gcs.WindowsGcsHvsockServiceID,
		})
		if err != nil {
			return nil, err
		}
		uvm.gcListener = l
	}

	return uvm, nil
}

// CloneeWCOW creates an HCS compute system representing a utility VM by creating a clone
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

	scratchFolder, scratchPath, err := createScratchVhdx(ctx, opts, uvmFolder, utc.UVMVhdPath)
	if err != nil {
		return nil, err
	}

	doc, err := prepareConfigDoc(ctx, uvm, opts, uvmFolder, scratchPath)
	if err != nil {
		return nil, fmt.Errorf("Error in preparing config doc: %s", err)
	}

	doc.VirtualMachine.RestoreState = &hcsschema.RestoreState{}
	doc.VirtualMachine.RestoreState.TemplateSystemId = utc.UVMID

	uvm.scsiLocations[0][0] = &SCSIMount{
		vm:       uvm,
		HostPath: doc.VirtualMachine.Devices.Scsi["0"].Attachments["0"].Path,
		refCount: 1,
	}

	for _, scsiMount := range utc.SCSIMounts {
		conStr := fmt.Sprintf("%d", scsiMount.Controller)
		lunStr := fmt.Sprintf("%d", scsiMount.LUN)
		var dstVhdPath string = scsiMount.HostPath
		if scsiMount.IsScratch {
			// Copy this scsi disk
			// TODO(ambarve): This is a SCSI mount that belongs to some container
			// which is being automatically cloned here as a part of UVM cloning
			// process. We will receive a request for creation of this container
			// later on which will specify the storage path for this container.
			// However, that storage location is not available now so we just use
			// the storage of the uvm instead. Find a better way for handling this.
			dir, err := ioutil.TempDir(scratchFolder, fmt.Sprintf("clone-mount-%d-%d", scsiMount.Controller, scsiMount.LUN))
			if err != nil {
				return nil, fmt.Errorf("Error while creating directory for scsi mounts of clone vm: %s", err)
			}

			// copy the VHDX of source VM
			dstVhdPath = filepath.Join(dir, "sandbox.vhdx")
			if err = copyfile.CopyFile(ctx, scsiMount.HostPath, dstVhdPath, true); err != nil {
				return nil, err
			}

			if err = wclayer.GrantVmAccess(ctx, opts.ID, dstVhdPath); err != nil {
				os.Remove(dstVhdPath)
				return nil, err
			}

		}

		doc.VirtualMachine.Devices.Scsi[conStr].Attachments[lunStr] = hcsschema.Attachment{
			Path:  dstVhdPath,
			Type_: scsiMount.Type,
		}

		uvm.scsiLocations[scsiMount.Controller][scsiMount.LUN] = &SCSIMount{
			vm:       uvm,
			HostPath: dstVhdPath,
			refCount: 1,
		}
	}

	for _, vsmbShare := range utc.VSMBShares {
		doc.VirtualMachine.Devices.VirtualSmb.Shares = append(doc.VirtualMachine.Devices.VirtualSmb.Shares, hcsschema.VirtualSmbShare{
			Name: vsmbShare.ShareName,
			Path: vsmbShare.HostPath,
			Options: &hcsschema.VirtualSmbShareOptions{
				ReadOnly:            true,
				PseudoOplocks:       true,
				TakeBackupPrivilege: true,
				CacheIo:             true,
				ShareRead:           true,
			},
		})
		uvm.vsmbCounter++
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
		l, err := winio.ListenHvsock(&winio.HvsockAddr{
			VMID:      uvm.runtimeID,
			ServiceID: gcs.WindowsGcsHvsockServiceID,
		})
		if err != nil {
			return nil, err
		}
		uvm.gcListener = l
	}

	return uvm, nil
}
