//go:build windows

package uvm

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/copyfile"
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
	"github.com/Microsoft/hcsshim/osversion"
)

// General information about how this works at a high level.
//
// The purpose is to start an LCOW Utility VM or UVM using the Host Compute Service, an API to create and manipulate running virtual machines
// HCS takes json descriptions of the work to be done.
//
// When a pod (there is a one to one mapping of pod to UVM) is to be created various annotations and defaults are combined into an options object which is
// passed to CreateLCOW (see below) where the options are transformed into a json document to be presented to the HCS VM creation code.
//
// There are two paths in CreateLCOW to creating the json document. The most flexible case is makeLCOWDoc which is used where no specialist hardware security
// applies, then there is makeLCOWSecurityDoc which is used in the case of AMD SEV-SNP memory encryption and integrity protection. There is quite
// a lot of difference between the two paths, for example the regular path has options about the type of kernel and initrd binary whereas the AMD SEV-SNP
// path has only one file but there are many other detail differences, so the code is split for clarity.
//
// makeLCOW*Doc returns an instance of hcsschema.ComputeSystem. That is then serialised to the json string provided to the flat C api. A similar scheme is used
// for later adjustments, for example adding a newtwork adpator.
//
// Examples of the eventual json are inline as comments by these two functions to show the eventual effect of the code.
//
// Note that the schema files, ie the Go objects that represent the json, are generated outside of the local build process.

type PreferredRootFSType int

const (
	PreferredRootFSTypeInitRd PreferredRootFSType = iota
	PreferredRootFSTypeVHD
	PreferredRootFSTypeNA

	entropyVsockPort  = 1
	linuxLogVsockPort = 109
)

const (
	// InitrdFile is the default file name for an initrd.img used to boot LCOW.
	InitrdFile = "initrd.img"
	// VhdFile is the default file name for a rootfs.vhd used to boot LCOW.
	VhdFile = "rootfs.vhd"
	// DefaultDmVerityRootfsVhd is the default file name for a dmverity_rootfs.vhd,
	// which is mounted by the GuestStateFile during boot and used as the root file
	// system when booting in the SNP case. Similar to layer VHDs, the Merkle tree
	// is appended after ext4 filesystem ends.
	DefaultDmVerityRootfsVhd = "rootfs.vhd"
	// KernelFile is the default file name for a kernel used to boot LCOW.
	KernelFile = "kernel"
	// UncompressedKernelFile is the default file name for an uncompressed
	// kernel used to boot LCOW with KernelDirect.
	UncompressedKernelFile = "vmlinux"
	// GuestStateFile is the default file name for a vmgs (VM Guest State) file
	// which contains the kernel and kernel command which mounts DmVerityVhdFile
	// when booting in the SNP case.
	GuestStateFile = "kernel.vmgs"
	// UVMReferenceInfoFile is the default file name for a COSE_Sign1
	// reference UVM info, which can be made available to workload containers
	// and can be used for validation purposes.
	UVMReferenceInfoFile = "reference_info.cose"
)

type ConfidentialLCOWOptions struct {
	*ConfidentialCommonOptions
	UseGuestStateFile  bool   // Use a vmgs file that contains a kernel and initrd, required for SNP
	DmVerityRootFsVhd  string // The VHD file (bound to the vmgs file via embedded dmverity hash data file) to load.
	DmVerityMode       bool   // override to be able to turn off dmverity for debugging
	DmVerityCreateArgs string // set dm-verity args when booting with verity in non-SNP mode
}

// OptionsLCOW are the set of options passed to CreateLCOW() to create a utility vm.
type OptionsLCOW struct {
	*Options
	*ConfidentialLCOWOptions

	// Folder in which kernel and root file system reside. Defaults to \Program Files\Linux Containers.
	//
	// It is preferred to use [UpdateBootFilesPath] to change this value and update associated fields.
	BootFilesPath           string
	KernelFile              string               // Filename under `BootFilesPath` for the kernel. Defaults to `kernel`
	KernelDirect            bool                 // Skip UEFI and boot directly to `kernel`
	RootFSFile              string               // Filename under `BootFilesPath` for the UVMs root file system. Defaults to `InitrdFile`
	KernelBootOptions       string               // Additional boot options for the kernel
	UseGuestConnection      bool                 // Whether the HCS should connect to the UVM's GCS. Defaults to true
	ExecCommandLine         string               // The command line to exec from init. Defaults to GCS
	ForwardStdout           bool                 // Whether stdout will be forwarded from the executed program. Defaults to false
	ForwardStderr           bool                 // Whether stderr will be forwarded from the executed program. Defaults to true
	OutputHandlerCreator    OutputHandlerCreator `json:"-"` // Creates an [OutputHandler] that controls how output received over HVSocket from the UVM is handled. Defaults to parsing output as logrus messages
	VPMemDeviceCount        uint32               // Number of VPMem devices. Defaults to `DefaultVPMEMCount`. Limit at 128. If booting UVM from VHD, device 0 is taken.
	VPMemSizeBytes          uint64               // Size of the VPMem devices. Defaults to `DefaultVPMemSizeBytes`.
	VPMemNoMultiMapping     bool                 // Disables LCOW layer multi mapping
	PreferredRootFSType     PreferredRootFSType  // If `KernelFile` is `InitrdFile` use `PreferredRootFSTypeInitRd`. If `KernelFile` is `VhdFile` use `PreferredRootFSTypeVHD`
	EnableColdDiscardHint   bool                 // Whether the HCS should use cold discard hints. Defaults to false
	VPCIEnabled             bool                 // Whether the kernel should enable pci
	EnableScratchEncryption bool                 // Whether the scratch should be encrypted
	DisableTimeSyncService  bool                 // Disables the time synchronization service
	HclEnabled              *bool                // Whether to enable the host compatibility layer
	ExtraVSockPorts         []uint32             // Extra vsock ports to allow
	AssignedDevices         []VPCIDeviceID       // AssignedDevices are devices to add on pod boot
	PolicyBasedRouting      bool                 // Whether we should use policy based routing when configuring net interfaces in guest
	WritableOverlayDirs     bool                 // Whether init should create writable overlay mounts for /var and /etc
}

// NewDefaultOptionsLCOW creates the default options for a bootable version of
// LCOW.
//
// `id` the ID of the compute system. If not passed will generate a new GUID.
//
// `owner` the owner of the compute system. If not passed will use the
// executable files name.
func NewDefaultOptionsLCOW(id, owner string) *OptionsLCOW {
	// Use KernelDirect boot by default on all builds that support it.
	kernelDirectSupported := osversion.Build() >= 18286
	opts := &OptionsLCOW{
		Options:                 newDefaultOptions(id, owner),
		KernelFile:              KernelFile,
		KernelDirect:            kernelDirectSupported,
		RootFSFile:              InitrdFile,
		KernelBootOptions:       "",
		UseGuestConnection:      true,
		ExecCommandLine:         fmt.Sprintf("/bin/gcs -v4 -log-format json -loglevel %s", logrus.StandardLogger().Level.String()),
		ForwardStdout:           false,
		ForwardStderr:           true,
		OutputHandlerCreator:    parseLogrus,
		VPMemDeviceCount:        DefaultVPMEMCount,
		VPMemSizeBytes:          DefaultVPMemSizeBytes,
		VPMemNoMultiMapping:     osversion.Get().Build < osversion.V19H1,
		PreferredRootFSType:     PreferredRootFSTypeInitRd,
		EnableColdDiscardHint:   false,
		VPCIEnabled:             false,
		EnableScratchEncryption: false,
		DisableTimeSyncService:  false,
		ConfidentialLCOWOptions: &ConfidentialLCOWOptions{
			ConfidentialCommonOptions: &ConfidentialCommonOptions{
				SecurityPolicyEnabled: false,
				UVMReferenceInfoFile:  UVMReferenceInfoFile,
			},
		},
	}

	opts.UpdateBootFilesPath(context.TODO(), vmutils.DefaultLCOWOSBootFilesPath())

	return opts
}

// UpdateBootFilesPath updates the LCOW BootFilesPath field and associated settings.
// Specifically, if [VhdFile] is found in path, RootFS is updated, and, if KernelDirect is set,
// KernelFile is also updated if [UncompressedKernelFile] is found in path.
//
// This is a nop if the current BootFilesPath is equal to path (case-insensitive).
func (opts *OptionsLCOW) UpdateBootFilesPath(ctx context.Context, path string) {
	if p, err := filepath.Abs(path); err == nil {
		path = p
	} else {
		// if its a filesystem issue, we'll error out elsewhere when we try to access the boot files
		// otherwise, it might be transient, or a Go issue, so log and move on
		log.G(ctx).WithFields(logrus.Fields{
			logfields.Path:  p,
			logrus.ErrorKey: err,
		}).Warning("could not make boot files path absolute")
	}

	if strings.EqualFold(opts.BootFilesPath, path) { // Windows is case-insensitive, so compare paths that way too
		return
	}

	opts.BootFilesPath = path

	if _, err := os.Stat(filepath.Join(opts.BootFilesPath, VhdFile)); err == nil {
		// We have a rootfs.vhd in the boot files path. Use it over an initrd.img
		opts.RootFSFile = VhdFile
		opts.PreferredRootFSType = PreferredRootFSTypeVHD

		log.G(ctx).WithFields(logrus.Fields{
			logfields.UVMID: opts.ID,
			VhdFile:         filepath.Join(opts.BootFilesPath, VhdFile),
		}).Debug("updated LCOW root filesystem to " + VhdFile)
	}

	if opts.KernelDirect {
		// KernelDirect supports uncompressed kernel if the kernel is present.
		// Default to uncompressed if on box. NOTE: If `kernel` is already
		// uncompressed and simply named 'kernel' it will still be used
		// uncompressed automatically.
		if _, err := os.Stat(filepath.Join(opts.BootFilesPath, UncompressedKernelFile)); err == nil {
			opts.KernelFile = UncompressedKernelFile

			log.G(ctx).WithFields(logrus.Fields{
				logfields.UVMID:        opts.ID,
				UncompressedKernelFile: filepath.Join(opts.BootFilesPath, UncompressedKernelFile),
			}).Debug("updated LCOW kernel file to " + UncompressedKernelFile)
		}
	}
}

// Get an acceptable number of processors given option and actual constraints.
func fetchProcessor(ctx context.Context, opts *OptionsLCOW, uvm *UtilityVM) (*hcsschema.VirtualMachineProcessor, error) {
	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host processor information: %w", err)
	}

	// To maintain compatibility with Docker we need to automatically downgrade
	// a user CPU count if the setting is not possible.
	uvm.processorCount = vmutils.NormalizeProcessorCount(ctx, uvm.id, opts.ProcessorCount, processorTopology)

	processor := &hcsschema.VirtualMachineProcessor{
		Count:  uint32(uvm.processorCount),
		Limit:  uint64(opts.ProcessorLimit),
		Weight: uint64(opts.ProcessorWeight),
	}
	// We can set a cpu group for the VM at creation time in recent builds.
	if opts.CPUGroupID != "" {
		if osversion.Build() < osversion.V21H1 {
			return nil, errCPUGroupCreateNotSupported
		}
		processor.CpuGroup = &hcsschema.CpuGroup{Id: opts.CPUGroupID}
	}
	return processor, nil
}

/*
Example JSON document produced once the hcsschema.ComputeSytem returned by makeLCOWSecurityDoc is serialised:
{
    "Owner": "containerd-shim-runhcs-v1.exe",
    "SchemaVersion": {
        "Major": 2,
        "Minor": 5
    },
    "ShouldTerminateOnLastHandleClosed": true,
    "VirtualMachine": {
        "Chipset": {
            "Uefi": {
                "ApplySecureBootTemplate": "Apply",
                "SecureBootTemplateId": "1734c6e8-3154-4dda-ba5f-a874cc483422"
            }
        },
        "ComputeTopology": {
            "Memory": {
                "SizeInMB": 1024
            },
            "Processor": {
                "Count": 2
            }
        },
        "Devices": {
            "Scsi" : { "0" : {} },
            "HvSocket": {
                "HvSocketConfig": {
                    "DefaultBindSecurityDescriptor":  "D:P(A;;FA;;;WD)",
                    "DefaultConnectSecurityDescriptor": "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
                    "ServiceTable" : {
                         "00000808-facb-11e6-bd58-64006a7986d3" :  {
                             "AllowWildcardBinds" : true,
                             "BindSecurityDescriptor":   "D:P(A;;FA;;;WD)",
                             "ConnectSecurityDescriptor": "D:P(A;;FA;;;SY)(A;;FA;;;BA)"
                         },
                         "0000006d-facb-11e6-bd58-64006a7986d3" :  {
                             "AllowWildcardBinds" : true,
                             "BindSecurityDescriptor":   "D:P(A;;FA;;;WD)",
                             "ConnectSecurityDescriptor": "D:P(A;;FA;;;SY)(A;;FA;;;BA)"
                         },
                         "00000001-facb-11e6-bd58-64006a7986d3" :  {
                             "AllowWildcardBinds" : true,
                             "BindSecurityDescriptor":   "D:P(A;;FA;;;WD)",
                             "ConnectSecurityDescriptor": "D:P(A;;FA;;;SY)(A;;FA;;;BA)"
                         },
                         "40000000-facb-11e6-bd58-64006a7986d3" :  {
                             "AllowWildcardBinds" : true,
                             "BindSecurityDescriptor":  "D:P(A;;FA;;;WD)",
                             "ConnectSecurityDescriptor": "D:P(A;;FA;;;SY)(A;;FA;;;BA)"
                         }
                     }
                }
            },
            "Plan9": {}
        },
        "GuestState": {
            "GuestStateFilePath": "d:\\ken\\aug27\\gcsinitnew.vmgs",
            "GuestStateFileType": "FileMode",
			"ForceTransientState": true
        },
        "SecuritySettings": {
            "Isolation": {
                "IsolationType": "SecureNestedPaging",
                "LaunchData": "kBifgKNijdHjxdSUshmavrNofo2B01LiIi1cr8R4ytI=",
				"HclEnabled": false
            }
        },
        "Version": {
            "Major": 254,
            "Minor": 0
        }
    }
}
*/

// A large part of difference between the SNP case and the usual kernel+option+initrd case is to do with booting
// from a VMGS file. The VMGS part may be used other than with SNP so is split out here.

// Make a hcsschema.ComputeSytem with the parts that target booting from a VMGS file.
func makeLCOWVMGSDoc(ctx context.Context, opts *OptionsLCOW, uvm *UtilityVM) (_ *hcsschema.ComputeSystem, err error) {
	// Raise an error if instructed to use a particular sort of rootfs.
	if opts.PreferredRootFSType != PreferredRootFSTypeNA {
		return nil, errors.New("specifying a PreferredRootFSType is incompatible with SNP mode")
	}

	// The kernel and minimal initrd are combined into a single vmgs file.
	vmgsTemplatePath := filepath.Join(opts.BootFilesPath, opts.GuestStateFilePath)
	if _, err := os.Stat(vmgsTemplatePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("the GuestState vmgs file '%s' was not found", vmgsTemplatePath)
	}

	// The root file system comes from the dmverity vhd file which is mounted by the initrd in the vmgs file.
	dmVerityRootfsTemplatePath := filepath.Join(opts.BootFilesPath, opts.DmVerityRootFsVhd)
	if _, err := os.Stat(dmVerityRootfsTemplatePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("the DM Verity VHD file '%s' was not found", dmVerityRootfsTemplatePath)
	}

	var processor *hcsschema.VirtualMachineProcessor
	processor, err = fetchProcessor(ctx, opts, uvm)
	if err != nil {
		return nil, err
	}

	vmgsFileFullPath := filepath.Join(opts.BundleDirectory, opts.GuestStateFilePath)
	if err := copyfile.CopyFile(ctx, vmgsTemplatePath, vmgsFileFullPath, true); err != nil {
		return nil, fmt.Errorf("failed to copy VMGS template file: %w", err)
	}
	defer func() {
		if err != nil {
			os.Remove(vmgsFileFullPath)
		}
	}()

	dmVerityRootFsFullPath := filepath.Join(opts.BundleDirectory, DefaultDmVerityRootfsVhd)
	if err := copyfile.CopyFile(ctx, dmVerityRootfsTemplatePath, dmVerityRootFsFullPath, true); err != nil {
		return nil, fmt.Errorf("failed to copy DM Verity rootfs template file: %w", err)
	}
	defer func() {
		if err != nil {
			os.Remove(dmVerityRootFsFullPath)
		}
	}()

	for _, filename := range []string{
		vmgsFileFullPath,
		dmVerityRootFsFullPath,
	} {
		if err := security.GrantVmGroupAccessWithMask(filename, security.AccessMaskAll); err != nil {
			return nil, fmt.Errorf("failed to grant VM group access ALL: %w", err)
		}
	}

	// Align the requested memory size.
	memorySizeInMB := vmutils.NormalizeMemorySize(ctx, uvm.id, opts.MemorySizeInMB)

	doc := &hcsschema.ComputeSystem{
		Owner:                             uvm.owner,
		SchemaVersion:                     schemaversion.SchemaV25(),
		ShouldTerminateOnLastHandleClosed: true,
		VirtualMachine: &hcsschema.VirtualMachine{
			StopOnReset: true,
			Chipset:     &hcsschema.Chipset{},
			ComputeTopology: &hcsschema.Topology{
				Memory: &hcsschema.VirtualMachineMemory{
					SizeInMB:              memorySizeInMB,
					AllowOvercommit:       opts.AllowOvercommit,
					EnableDeferredCommit:  opts.EnableDeferredCommit,
					EnableColdDiscardHint: opts.EnableColdDiscardHint,
					LowMMIOGapInMB:        opts.LowMMIOGapInMB,
					HighMMIOBaseInMB:      opts.HighMMIOBaseInMB,
					HighMMIOGapInMB:       opts.HighMMIOGapInMB,
				},
				Processor: processor,
			},
			Devices: &hcsschema.Devices{
				HvSocket: &hcsschema.HvSocket2{
					HvSocketConfig: &hcsschema.HvSocketSystemConfig{
						// Allow administrators and SYSTEM to bind to vsock sockets
						// so that we can create a GCS log socket.
						DefaultBindSecurityDescriptor:    "D:P(A;;FA;;;WD)", // Differs for SNP
						DefaultConnectSecurityDescriptor: "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
						ServiceTable:                     make(map[string]hcsschema.HvSocketServiceConfig),
					},
				},
				Plan9: &hcsschema.Plan9{},
			},
		},
	}

	maps.Copy(doc.VirtualMachine.Devices.HvSocket.HvSocketConfig.ServiceTable, opts.AdditionalHyperVConfig)

	// Set permissions for the VSock ports:
	//		entropyVsockPort - 1 is the entropy port,
	//		linuxLogVsockPort - 109 used by vsockexec to log stdout/stderr logging,
	//		0x40000000 + 1 (LinuxGcsVsockPort + 1) is the bridge (see guestconnectiuon.go)
	hvSockets := []uint32{entropyVsockPort, linuxLogVsockPort, prot.LinuxGcsVsockPort, prot.LinuxGcsVsockPort + 1}
	hvSockets = append(hvSockets, opts.ExtraVSockPorts...)
	for _, whichSocket := range hvSockets {
		key := winio.VsockServiceID(whichSocket).String()
		doc.VirtualMachine.Devices.HvSocket.HvSocketConfig.ServiceTable[key] = hcsschema.HvSocketServiceConfig{
			AllowWildcardBinds:        true,
			BindSecurityDescriptor:    "D:P(A;;FA;;;WD)",
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

	if uvm.scsiControllerCount > 0 {
		logrus.Debug("makeLCOWVMGSDoc configuring scsi devices")
		doc.VirtualMachine.Devices.Scsi = map[string]hcsschema.Scsi{}
		for i := 0; i < int(uvm.scsiControllerCount); i++ {
			doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[i]] = hcsschema.Scsi{
				Attachments: make(map[string]hcsschema.Attachment),
			}
		}
		if opts.DmVerityMode {
			logrus.Debug("makeLCOWVMGSDoc DmVerityMode true")
			scsiController0 := guestrequest.ScsiControllerGuids[0]
			doc.VirtualMachine.Devices.Scsi[scsiController0].Attachments["0"] = hcsschema.Attachment{
				Type_:    "VirtualDisk",
				Path:     dmVerityRootFsFullPath,
				ReadOnly: true,
			}
			uvm.reservedSCSISlots = append(uvm.reservedSCSISlots, scsi.Slot{Controller: 0, LUN: 0})
		}
	}

	// Required by HCS for the isolated boot scheme, see also https://docs.microsoft.com/en-us/windows-server/virtualization/hyper-v/learn-more/generation-2-virtual-machine-security-settings-for-hyper-v
	// A complete explanation of the why's and wherefores of starting an encrypted, isolated VM are beond the scope of these comments.
	doc.VirtualMachine.Chipset.Uefi = &hcsschema.Uefi{
		ApplySecureBootTemplate: "Apply",
		SecureBootTemplateId:    "1734c6e8-3154-4dda-ba5f-a874cc483422", // aka MicrosoftWindowsSecureBootTemplateGUID equivalent to "Microsoft Windows" template from Get-VMHost | select SecureBootTemplates,

	}

	// Point at the file that contains the linux kernel and initrd images.
	doc.VirtualMachine.GuestState = &hcsschema.GuestState{
		GuestStateFilePath:  vmgsFileFullPath,
		GuestStateFileType:  "FileMode",
		ForceTransientState: true, // tell HCS that this is just the source of the images, not ongoing state
	}

	return doc, nil
}

// Programatically make the hcsschema.ComputeSystem document for the SNP case.
// This is done prior to json seriaisation and sending to the HCS layer to actually do the work of creating the VM.
// Many details are quite different (see the typical JSON examples), in particular it boots from a VMGS file
// which contains both the kernel and initrd as well as kernel boot options.
func makeLCOWSecurityDoc(ctx context.Context, opts *OptionsLCOW, uvm *UtilityVM) (_ *hcsschema.ComputeSystem, err error) {
	doc, vmgsErr := makeLCOWVMGSDoc(ctx, opts, uvm)
	if vmgsErr != nil {
		return nil, vmgsErr
	}

	// Part of the protocol to ensure that the rules in the user's Security Policy are
	// respected is to provide a hash of the policy to the hardware. This is immutable
	// and can be used to check that the policy used by opengcs is the required one as
	// a condition of releasing secrets to the container.

	policyDigest, err := securitypolicy.NewSecurityPolicyDigest(opts.SecurityPolicy)
	if err != nil {
		return nil, err
	}
	// HCS API expect a base64 encoded string as LaunchData. Internally it
	// decodes it to bytes. SEV later returns the decoded byte blob as HostData
	// field of the report.
	hostData := base64.StdEncoding.EncodeToString(policyDigest)

	// Put the measurement into the LaunchData field of the HCS creation command.
	// This will end-up in HOST_DATA of SNP_LAUNCH_FINISH command the and ATTESTATION_REPORT
	// retrieved by the guest later.
	doc.VirtualMachine.SecuritySettings = &hcsschema.SecuritySettings{
		EnableTpm: false,
		Isolation: &hcsschema.IsolationSettings{
			IsolationType: "SecureNestedPaging",
			LaunchData:    hostData,
			// HclEnabled:    true, /* Not available in schema 2.5 - REQUIRED when using BlockStorage in 2.6 */
			HclEnabled: opts.HclEnabled,
		},
	}

	return doc, nil
}

/*
Example JSON document produced once the hcsschema.ComputeSytem returned by makeLCOWDoc is serialised. Note that the boot scheme is entirely different.
{
    "Owner": "containerd-shim-runhcs-v1.exe",
    "SchemaVersion": {
        "Major": 2,
        "Minor": 1
    },
    "VirtualMachine": {
        "StopOnReset": true,
        "Chipset": {
            "LinuxKernelDirect": {
                "KernelFilePath": "C:\\ContainerPlat\\LinuxBootFiles\\vmlinux",
                "InitRdPath": "C:\\ContainerPlat\\LinuxBootFiles\\initrd.img",
                "KernelCmdLine": " 8250_core.nr_uarts=0 panic=-1 quiet pci=off nr_cpus=2 brd.rd_nr=0 pmtmr=0 -- -e 1 /bin/vsockexec -e 109 /bin/gcs -v4 -log-format json -loglevel debug"
            }
        },
        "ComputeTopology": {
            "Memory": {
                "SizeInMB": 1024,
                "AllowOvercommit": true
            },
            "Processor": {
                "Count": 2
            }
        },
        "Devices": {
            "Scsi": {
                "0": {}
            },
            "HvSocket": {
                "HvSocketConfig": {
                    "DefaultBindSecurityDescriptor": "D:P(A;;FA;;;SY)(A;;FA;;;BA)"
                }
            },
            "Plan9": {}
        }
    },
    "ShouldTerminateOnLastHandleClosed": true
}
*/

// Make the ComputeSystem document object that will be serialized to json to be presented to the HCS api.
func makeLCOWDoc(ctx context.Context, opts *OptionsLCOW, uvm *UtilityVM) (_ *hcsschema.ComputeSystem, err error) {
	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		log.G(ctx).WithField("options", log.Format(ctx, opts)).Trace("makeLCOWDoc")
	}

	kernelFullPath := filepath.Join(opts.BootFilesPath, opts.KernelFile)
	if _, err := os.Stat(kernelFullPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("kernel: '%s' not found", kernelFullPath)
	}
	rootfsFullPath := filepath.Join(opts.BootFilesPath, opts.RootFSFile)
	if _, err := os.Stat(rootfsFullPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("boot file: '%s' not found", rootfsFullPath)
	}

	var processor *hcsschema.VirtualMachineProcessor
	processor, err = fetchProcessor(ctx, opts, uvm) // must happen after the file existence tests above.
	if err != nil {
		return nil, err
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

	// Align the requested memory size.
	memorySizeInMB := vmutils.NormalizeMemorySize(ctx, uvm.id, opts.MemorySizeInMB)

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

	doc := &hcsschema.ComputeSystem{
		Owner:                             uvm.owner,
		SchemaVersion:                     schemaversion.SchemaV21(),
		ShouldTerminateOnLastHandleClosed: true,
		VirtualMachine: &hcsschema.VirtualMachine{
			StopOnReset: true,
			Chipset:     &hcsschema.Chipset{},
			ComputeTopology: &hcsschema.Topology{
				Memory: &hcsschema.VirtualMachineMemory{
					SizeInMB:              memorySizeInMB,
					AllowOvercommit:       opts.AllowOvercommit,
					EnableDeferredCommit:  opts.EnableDeferredCommit,
					EnableColdDiscardHint: opts.EnableColdDiscardHint,
					LowMMIOGapInMB:        opts.LowMMIOGapInMB,
					HighMMIOBaseInMB:      opts.HighMMIOBaseInMB,
					HighMMIOGapInMB:       opts.HighMMIOGapInMB,
				},
				Processor: processor,
				Numa:      numa,
			},
			Devices: &hcsschema.Devices{
				HvSocket: &hcsschema.HvSocket2{
					HvSocketConfig: &hcsschema.HvSocketSystemConfig{
						// Allow administrators and SYSTEM to bind to vsock sockets
						// so that we can create a GCS log socket.
						DefaultBindSecurityDescriptor: "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
						ServiceTable:                  make(map[string]hcsschema.HvSocketServiceConfig),
					},
				},
				Plan9: &hcsschema.Plan9{},
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

	// Add optional devices that were specified on the UVM spec
	if len(opts.AssignedDevices) > 0 {
		if doc.VirtualMachine.Devices.VirtualPci == nil {
			doc.VirtualMachine.Devices.VirtualPci = make(map[string]hcsschema.VirtualPciDevice)
		}
		for _, d := range opts.AssignedDevices {
			// we don't need to hold the modify lock here because the UVM has
			// not yet been created.
			existingDevice := uvm.vpciDevices[d]
			if existingDevice != nil {
				return nil, fmt.Errorf("device %s with index %d is specified multiple times", d.deviceInstanceID, d.virtualFunctionIndex)
			}

			vmbusGUID, err := guid.NewV4()
			if err != nil {
				return nil, err
			}

			var propagateAffinity *bool
			T := true
			if osversion.Get().Build >= osversion.V25H1Server && (numa != nil || numaProcessors != nil) {
				propagateAffinity = &T
			}
			doc.VirtualMachine.Devices.VirtualPci[vmbusGUID.String()] = hcsschema.VirtualPciDevice{
				Functions: []hcsschema.VirtualPciFunction{
					{
						DeviceInstancePath: d.deviceInstanceID,
						VirtualFunction:    d.virtualFunctionIndex,
					},
				},
				PropagateNumaAffinity: propagateAffinity,
			}

			device := &VPCIDevice{
				vm:                   uvm,
				VMBusGUID:            vmbusGUID.String(),
				deviceInstanceID:     d.deviceInstanceID,
				virtualFunctionIndex: d.virtualFunctionIndex,
				refCount:             1,
			}
			uvm.vpciDevices[d] = device
		}
	}

	maps.Copy(doc.VirtualMachine.Devices.HvSocket.HvSocketConfig.ServiceTable, opts.AdditionalHyperVConfig)

	// Handle StorageQoS if set
	if opts.StorageQoSBandwidthMaximum > 0 || opts.StorageQoSIopsMaximum > 0 {
		doc.VirtualMachine.StorageQoS = &hcsschema.StorageQoS{
			IopsMaximum:      opts.StorageQoSIopsMaximum,
			BandwidthMaximum: opts.StorageQoSBandwidthMaximum,
		}
	}

	if uvm.scsiControllerCount > 0 {
		doc.VirtualMachine.Devices.Scsi = map[string]hcsschema.Scsi{}
		for i := 0; i < int(uvm.scsiControllerCount); i++ {
			doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[i]] = hcsschema.Scsi{
				Attachments: make(map[string]hcsschema.Attachment),
			}
		}
	}

	if uvm.vpmemMaxCount > 0 {
		doc.VirtualMachine.Devices.VirtualPMem = &hcsschema.VirtualPMemController{
			MaximumCount:     uvm.vpmemMaxCount,
			MaximumSizeBytes: uvm.vpmemMaxSizeBytes,
		}
	}

	var kernelArgs string
	switch opts.PreferredRootFSType {
	case PreferredRootFSTypeInitRd:
		if !opts.KernelDirect {
			kernelArgs = "initrd=/" + opts.RootFSFile
		}
	case PreferredRootFSTypeVHD:
		if uvm.vpmemMaxCount > 0 {
			// Support for VPMem VHD(X) booting rather than initrd..
			kernelArgs = "root=/dev/pmem0 ro rootwait init=/init"
			imageFormat := "Vhd1"
			if strings.ToLower(filepath.Ext(opts.RootFSFile)) == "vhdx" {
				imageFormat = "Vhdx"
			}
			doc.VirtualMachine.Devices.VirtualPMem.Devices = map[string]hcsschema.VirtualPMemDevice{
				"0": {
					HostPath:    rootfsFullPath,
					ReadOnly:    true,
					ImageFormat: imageFormat,
				},
			}
			if uvm.vpmemMultiMapping {
				pmem := newPackedVPMemDevice()
				pmem.maxMappedDeviceCount = 1

				st, stErr := os.Stat(rootfsFullPath)
				if stErr != nil {
					return nil, errors.Wrapf(stErr, "failed to stat rootfs: %q", rootfsFullPath)
				}
				devSize := pageAlign(uint64(st.Size()))
				memReg, pErr := pmem.Allocate(devSize)
				if pErr != nil {
					return nil, errors.Wrap(pErr, "failed to allocate memory for rootfs")
				}
				defer func() {
					if err != nil {
						if err = pmem.Release(memReg); err != nil {
							log.G(ctx).WithError(err).Debug("failed to release memory region")
						}
					}
				}()

				dev := newVPMemMappedDevice(opts.RootFSFile, "/", devSize, memReg)
				if err := pmem.mapVHDLayer(ctx, dev); err != nil {
					return nil, errors.Wrapf(err, "failed to save internal state for a multi-mapped rootfs device")
				}
				uvm.vpmemDevicesMultiMapped[0] = pmem
			} else {
				dev := newDefaultVPMemInfo(opts.RootFSFile, "/")
				uvm.vpmemDevicesDefault[0] = dev
			}
		} else {
			kernelArgs = "root=/dev/sda ro rootwait init=/init"
			if opts.DmVerityMode {
				if len(opts.DmVerityCreateArgs) == 0 {
					return nil, errors.New("DmVerityCreateArgs must be set when DmVerityMode is true and not booting from a vmgs file.")
				}
				kernelArgs = fmt.Sprintf("root=/dev/dm-0 dm-mod.create=%q init=/init", opts.DmVerityCreateArgs)
			}
			doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[0]].Attachments["0"] = hcsschema.Attachment{
				Type_:    "VirtualDisk",
				Path:     rootfsFullPath,
				ReadOnly: true,
			}
			uvm.reservedSCSISlots = append(uvm.reservedSCSISlots, scsi.Slot{Controller: 0, LUN: 0})
		}
	}

	// Explicitly disable virtio_vsock_init, to make sure that we use hv_sock transport. For kernels built without
	// virtio-vsock this is a no-op.
	kernelArgs += " initcall_blacklist=virtio_vsock_init"

	vmDebugging := false
	if opts.ConsolePipe != "" {
		vmDebugging = true
		kernelArgs += " 8250_core.nr_uarts=1 8250_core.skip_txen_test=1 console=ttyS0,115200"
		doc.VirtualMachine.Devices.ComPorts = map[string]hcsschema.ComPort{
			"0": { // Which is actually COM1
				NamedPipe: opts.ConsolePipe,
			},
		}
	} else {
		kernelArgs += " 8250_core.nr_uarts=0"
	}

	if opts.EnableGraphicsConsole {
		vmDebugging = true
		kernelArgs += " console=tty"
		doc.VirtualMachine.Devices.Keyboard = &hcsschema.Keyboard{}
		doc.VirtualMachine.Devices.EnhancedModeVideo = &hcsschema.EnhancedModeVideo{}
		doc.VirtualMachine.Devices.VideoMonitor = &hcsschema.VideoMonitor{}
	}

	if !vmDebugging {
		// Terminate the VM if there is a kernel panic.
		kernelArgs += " panic=-1 quiet"
	}

	// Add Kernel Boot options
	if opts.KernelBootOptions != "" {
		kernelArgs += " " + opts.KernelBootOptions
	}

	if !opts.VPCIEnabled {
		kernelArgs += ` pci=off`
	}

	// Inject initial entropy over vsock during init launch.
	entropyArgs := fmt.Sprintf("-e %d", entropyVsockPort)

	// With default options, run GCS with stderr pointing to the vsock port
	// created below in order to forward guest logs to logrus.
	execCmdArgs := "/bin/vsockexec"

	if opts.ForwardStdout {
		execCmdArgs += fmt.Sprintf(" -o %d", linuxLogVsockPort)
	}

	if opts.ForwardStderr {
		execCmdArgs += fmt.Sprintf(" -e %d", linuxLogVsockPort)
	}

	if opts.DisableTimeSyncService {
		opts.ExecCommandLine = fmt.Sprintf("%s -disable-time-sync", opts.ExecCommandLine)
	}

	if log.IsScrubbingEnabled() {
		opts.ExecCommandLine += " -scrub-logs"
	}

	execCmdArgs += " " + opts.ExecCommandLine

	if opts.ProcessDumpLocation != "" {
		execCmdArgs += " -core-dump-location " + opts.ProcessDumpLocation
	}

	initArgs := entropyArgs
	if opts.WritableOverlayDirs {
		switch opts.PreferredRootFSType {
		case PreferredRootFSTypeInitRd:
			log.G(ctx).Warn("ignoring `WritableOverlayDirs` option since rootfs is already writable")
		case PreferredRootFSTypeVHD:
			initArgs += " -w"
		}
	}
	if vmDebugging {
		// Launch a shell on the console.
		initArgs += ` sh -c "` + execCmdArgs + ` & exec sh"`
	} else {
		initArgs += " " + execCmdArgs
	}

	kernelArgs += fmt.Sprintf(" nr_cpus=%d", opts.ProcessorCount)
	kernelArgs += ` brd.rd_nr=0 pmtmr=0 -- ` + initArgs

	if !opts.KernelDirect {
		doc.VirtualMachine.Chipset.Uefi = &hcsschema.Uefi{
			BootThis: &hcsschema.UefiBootEntry{
				DevicePath:    `\` + opts.KernelFile,
				DeviceType:    "VmbFs",
				VmbFsRootPath: opts.BootFilesPath,
				OptionalData:  kernelArgs,
			},
		}
	} else {
		doc.VirtualMachine.Chipset.LinuxKernelDirect = &hcsschema.LinuxKernelDirect{
			KernelFilePath: kernelFullPath,
			KernelCmdLine:  kernelArgs,
		}
		if opts.PreferredRootFSType == PreferredRootFSTypeInitRd {
			doc.VirtualMachine.Chipset.LinuxKernelDirect.InitRdPath = rootfsFullPath
		}
	}
	return doc, nil
}

// CreateLCOW creates an HCS compute system representing a utility VM. It
// consumes a set of options derived from various defaults and options
// expressed as annotations.
func CreateLCOW(ctx context.Context, opts *OptionsLCOW) (_ *UtilityVM, err error) {
	ctx, span := oc.StartSpan(ctx, "uvm::CreateLCOW")
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
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		log.G(ctx).WithField("options", log.Format(ctx, opts)).Debug("uvm::CreateLCOW options")
	}

	// We don't serialize OutputHandlerCreator so if it is missing we need to put it back to the default.
	if opts.OutputHandlerCreator == nil {
		opts.OutputHandlerCreator = parseLogrus
	}

	uvm := &UtilityVM{
		id:                      opts.ID,
		owner:                   opts.Owner,
		operatingSystem:         "linux",
		scsiControllerCount:     opts.SCSIControllerCount,
		vpmemMaxCount:           opts.VPMemDeviceCount,
		vpmemMaxSizeBytes:       opts.VPMemSizeBytes,
		vpciDevices:             make(map[VPCIDeviceID]*VPCIDevice),
		physicallyBacked:        !opts.AllowOvercommit,
		devicesPhysicallyBacked: opts.FullyPhysicallyBacked,
		createOpts:              opts,
		vpmemMultiMapping:       !opts.VPMemNoMultiMapping,
		encryptScratch:          opts.EnableScratchEncryption,
		noWritableFileShares:    opts.NoWritableFileShares,
		policyBasedRouting:      opts.PolicyBasedRouting,
	}

	defer func() {
		if err != nil {
			uvm.Close()
		}
	}()

	// vpmemMaxCount has been set to 0 which means we are going to need multiple SCSI controllers
	// to support lots of layers.
	if osversion.Build() >= osversion.RS5 && uvm.vpmemMaxCount == 0 {
		uvm.scsiControllerCount = 4
	}

	if err = verifyOptions(ctx, opts); err != nil {
		return nil, errors.Wrap(err, errBadUVMOpts.Error())
	}

	// HCS config for SNP isolated vm is quite different to the usual case
	var doc *hcsschema.ComputeSystem
	if opts.SecurityPolicyEnabled {
		doc, err = makeLCOWSecurityDoc(ctx, opts, uvm)
		if logrus.IsLevelEnabled(logrus.TraceLevel) {
			log.G(ctx).WithFields(logrus.Fields{
				"doc":           log.Format(ctx, doc),
				logrus.ErrorKey: err,
			}).Trace("create_lcow::CreateLCOW makeLCOWSecurityDoc result")
		}
	} else {
		doc, err = makeLCOWDoc(ctx, opts, uvm)
		if logrus.IsLevelEnabled(logrus.TraceLevel) {
			log.G(ctx).WithFields(logrus.Fields{
				"doc":           log.Format(ctx, doc),
				logrus.ErrorKey: err,
			}).Trace("create_lcow::CreateLCOW makeLCOWDoc result")
		}
	}
	if err != nil {
		return nil, err
	}

	if err = uvm.create(ctx, doc); err != nil {
		return nil, fmt.Errorf("error while creating the compute system: %w", err)
	}
	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		log.G(ctx).WithField("uvm", log.Format(ctx, uvm)).Trace("create_lcow::CreateLCOW uvm.create result")
	}

	// Create a socket to inject entropy during boot.
	uvm.entropyListener, err = uvm.listenVsock(entropyVsockPort)
	if err != nil {
		return nil, err
	}

	// Create a socket that the executed program can send to. This is usually
	// used by GCS to send log data.
	if opts.ForwardStdout || opts.ForwardStderr {
		uvm.outputHandler = opts.OutputHandlerCreator(opts.Options)
		uvm.outputProcessingDone = make(chan struct{})
		uvm.outputListener, err = uvm.listenVsock(linuxLogVsockPort)
		if err != nil {
			return nil, err
		}
	}

	if opts.UseGuestConnection {
		log.G(ctx).WithField("vmID", uvm.runtimeID).Debug("Using external GCS bridge")
		l, err := uvm.listenVsock(prot.LinuxGcsVsockPort)
		if err != nil {
			return nil, err
		}
		uvm.gcListener = l
	}

	uvm.ncProxyClientAddress = opts.NetworkConfigProxy

	return uvm, nil
}

func (uvm *UtilityVM) listenVsock(port uint32) (net.Listener, error) {
	return winio.ListenHvsock(&winio.HvsockAddr{
		VMID:      uvm.runtimeID,
		ServiceID: winio.VsockServiceID(port),
	})
}
