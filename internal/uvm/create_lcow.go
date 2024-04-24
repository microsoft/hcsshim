//go:build windows

package uvm

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/security"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/gcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/processorinfo"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
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
	// DmVerityVhdFile is the default file name for a dmverity_rootfs.vhd which
	// is mounted by the GuestStateFile during boot and used as the root file
	// system when booting in the SNP case.
	DefaultDmVerityRootfsVhd = "rootfs.vhd"
	DefaultDmVerityHashVhd   = "rootfs.hash.vhd"
	// KernelFile is the default file name for a kernel used to boot LCOW.
	KernelFile = "kernel"
	// UncompressedKernelFile is the default file name for an uncompressed
	// kernel used to boot LCOW with KernelDirect.
	UncompressedKernelFile = "vmlinux"
	// GuestStateFile is the default file name for a vmgs (VM Guest State) file
	// which combines kernel and initrd and is used to mount DmVerityVhdFile
	// when booting in the SNP case.
	GuestStateFile = "kernelinitrd.vmgs"
	// UVMReferenceInfoFile is the default file name for a COSE_Sign1
	// reference UVM info, which can be made available to workload containers
	// and can be used for validation purposes.
	UVMReferenceInfoFile = "reference_info.cose"
)

type ConfidentialOptions struct {
	GuestStateFile         string // The vmgs file to load
	UseGuestStateFile      bool   // Use a vmgs file that contains a kernel and initrd, required for SNP
	SecurityPolicy         string // Optional security policy
	SecurityPolicyEnabled  bool   // Set when there is a security policy to apply on actual SNP hardware, use this rathen than checking the string length
	SecurityPolicyEnforcer string // Set which security policy enforcer to use (open door, standard or rego). This allows for better fallback mechanic.
	UVMReferenceInfoFile   string // Filename under `BootFilesPath` for (potentially signed) UVM image reference information.
	BundleDirectory        string // pod bundle directory
	DmVerityRootFsVhd      string // The VHD file (bound to the vmgs file via embedded dmverity hash data file) to load.
	DmVerityHashVhd        string // The VHD file containing the hash tree
	DmVerityMode           bool   // override to be able to turn off dmverity for debugging
}

// OptionsLCOW are the set of options passed to CreateLCOW() to create a utility vm.
type OptionsLCOW struct {
	*Options
	*ConfidentialOptions

	// Folder in which kernel and root file system reside. Defaults to \Program Files\Linux Containers.
	//
	// It is preferred to use [UpdateBootFilesPath] to change this value and update associated fields.
	BootFilesPath           string
	KernelFile              string               // Filename under `BootFilesPath` for the kernel. Defaults to `kernel`
	KernelDirect            bool                 // Skip UEFI and boot directly to `kernel`
	RootFSFile              string               // Filename under `BootFilesPath` for the UVMs root file system. Defaults to `InitrdFile`
	KernelBootOptions       string               // Additional boot options for the kernel
	EnableGraphicsConsole   bool                 // If true, enable a graphics console for the utility VM
	ConsolePipe             string               // The named pipe path to use for the serial console.  eg \\.\pipe\vmpipe
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
}

// defaultLCOWOSBootFilesPath returns the default path used to locate the LCOW
// OS kernel and root FS files. This default is the subdirectory
// `LinuxBootFiles` in the directory of the executable that started the current
// process; or, if it does not exist, `%ProgramFiles%\Linux Containers`.
func defaultLCOWOSBootFilesPath() string {
	localDirPath := filepath.Join(filepath.Dir(os.Args[0]), "LinuxBootFiles")
	if _, err := os.Stat(localDirPath); err == nil {
		return localDirPath
	}
	return filepath.Join(os.Getenv("ProgramFiles"), "Linux Containers")
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
		EnableGraphicsConsole:   false,
		ConsolePipe:             "",
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
		ConfidentialOptions: &ConfidentialOptions{
			SecurityPolicyEnabled: false,
			UVMReferenceInfoFile:  UVMReferenceInfoFile,
		},
	}

	opts.UpdateBootFilesPath(context.TODO(), defaultLCOWOSBootFilesPath())

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
func fetchProcessor(ctx context.Context, opts *OptionsLCOW, uvm *UtilityVM) (*hcsschema.Processor2, error) {
	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host processor information: %w", err)
	}

	// To maintain compatibility with Docker we need to automatically downgrade
	// a user CPU count if the setting is not possible.
	uvm.processorCount = uvm.normalizeProcessorCount(ctx, opts.ProcessorCount, processorTopology)

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
	vmgsTemplatePath := filepath.Join(opts.BootFilesPath, opts.GuestStateFile)
	if _, err := os.Stat(vmgsTemplatePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("the GuestState vmgs file '%s' was not found", vmgsTemplatePath)
	}

	// The root file system comes from the dmverity vhd file which is mounted by the initrd in the vmgs file.
	dmVerityRootfsTemplatePath := filepath.Join(opts.BootFilesPath, opts.DmVerityRootFsVhd)
	if _, err := os.Stat(dmVerityRootfsTemplatePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("the DM Verity VHD file '%s' was not found", dmVerityRootfsTemplatePath)
	}

	// The root file system comes from the dmverity vhd file which is mounted by the initrd in the vmgs file.
	dmVerityHashTemplatePath := filepath.Join(opts.BootFilesPath, opts.DmVerityHashVhd)
	if _, err := os.Stat(dmVerityHashTemplatePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("the DM Verity Hash file '%s' was not found", dmVerityHashTemplatePath)
	}

	var processor *hcsschema.Processor2
	processor, err = fetchProcessor(ctx, opts, uvm)
	if err != nil {
		return nil, err
	}

	vmgsFileFullPath := filepath.Join(opts.BundleDirectory, opts.GuestStateFile)
	if err := createCopy(vmgsFileFullPath, vmgsTemplatePath); err != nil {
		return nil, fmt.Errorf("failed to copy VMGS template file: %w", err)
	}
	defer func() {
		if err != nil {
			os.Remove(vmgsFileFullPath)
		}
	}()

	dmVerityRootFsFullPath := filepath.Join(opts.BundleDirectory, DefaultDmVerityRootfsVhd)
	if err := createCopy(dmVerityRootFsFullPath, dmVerityRootfsTemplatePath); err != nil {
		return nil, fmt.Errorf("failed to copy DM Verity rootfs template file: %w", err)
	}
	defer func() {
		if err != nil {
			os.Remove(dmVerityRootFsFullPath)
		}
	}()

	dmVerityHashFullPath := filepath.Join(opts.BundleDirectory, DefaultDmVerityHashVhd)
	if err := createCopy(dmVerityHashFullPath, dmVerityHashTemplatePath); err != nil {
		return nil, fmt.Errorf("failed to copy DM Verity hash template file: %w", err)
	}
	defer func() {
		if err != nil {
			os.Remove(dmVerityHashFullPath)
		}
	}()

	for _, filename := range []string{vmgsFileFullPath, dmVerityRootFsFullPath, dmVerityHashFullPath} {
		if err := security.GrantVmGroupAccessWithMask(filename, security.AccessMaskAll); err != nil {
			return nil, fmt.Errorf("failed to grant VM group access ALL: %w", err)
		}
	}

	// Align the requested memory size.
	memorySizeInMB := uvm.normalizeMemorySize(ctx, opts.MemorySizeInMB)

	doc := &hcsschema.ComputeSystem{
		Owner:                             uvm.owner,
		SchemaVersion:                     schemaversion.SchemaV25(),
		ShouldTerminateOnLastHandleClosed: true,
		VirtualMachine: &hcsschema.VirtualMachine{
			StopOnReset: true,
			Chipset:     &hcsschema.Chipset{},
			ComputeTopology: &hcsschema.Topology{
				Memory: &hcsschema.Memory2{
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
	hvSockets := []uint32{entropyVsockPort, linuxLogVsockPort, gcs.LinuxGcsVsockPort, gcs.LinuxGcsVsockPort + 1}
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
		if opts.DmVerityMode {
			logrus.Debug("makeLCOWVMGSDoc DmVerityMode true")
			doc.VirtualMachine.Devices.Scsi = map[string]hcsschema.Scsi{
				"RootFileSystemVirtualDisk": {
					Attachments: map[string]hcsschema.Attachment{
						"0": {
							Type_:    "VirtualDisk",
							Path:     dmVerityRootFsFullPath,
							ReadOnly: true,
						},
						"1": {
							Type_:    "VirtualDisk",
							Path:     dmVerityHashFullPath,
							ReadOnly: true,
						},
					},
				},
			}
			uvm.reservedSCSISlots = append(uvm.reservedSCSISlots, scsi.Slot{Controller: 0, LUN: 0})
			uvm.reservedSCSISlots = append(uvm.reservedSCSISlots, scsi.Slot{Controller: 0, LUN: 1})
		}
		for i := 0; i < int(uvm.scsiControllerCount); i++ {
			doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[i]] = hcsschema.Scsi{
				Attachments: make(map[string]hcsschema.Attachment),
			}
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

func createCopy(dst, src string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
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
	logrus.Tracef("makeLCOWDoc %v\n", opts)

	kernelFullPath := filepath.Join(opts.BootFilesPath, opts.KernelFile)
	if _, err := os.Stat(kernelFullPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("kernel: '%s' not found", kernelFullPath)
	}
	rootfsFullPath := filepath.Join(opts.BootFilesPath, opts.RootFSFile)
	if _, err := os.Stat(rootfsFullPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("boot file: '%s' not found", rootfsFullPath)
	}

	var processor *hcsschema.Processor2
	processor, err = fetchProcessor(ctx, opts, uvm) // must happen after the file existence tests above.
	if err != nil {
		return nil, err
	}

	// Align the requested memory size.
	memorySizeInMB := uvm.normalizeMemorySize(ctx, opts.MemorySizeInMB)

	doc := &hcsschema.ComputeSystem{
		Owner:                             uvm.owner,
		SchemaVersion:                     schemaversion.SchemaV21(),
		ShouldTerminateOnLastHandleClosed: true,
		VirtualMachine: &hcsschema.VirtualMachine{
			StopOnReset: true,
			Chipset:     &hcsschema.Chipset{},
			ComputeTopology: &hcsschema.Topology{
				Memory: &hcsschema.Memory2{
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
						DefaultBindSecurityDescriptor: "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
						ServiceTable:                  make(map[string]hcsschema.HvSocketServiceConfig),
					},
				},
				Plan9: &hcsschema.Plan9{},
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
			doc.VirtualMachine.Devices.Scsi[guestrequest.ScsiControllerGuids[0]].Attachments["0"] = hcsschema.Attachment{
				Type_:    "VirtualDisk",
				Path:     rootfsFullPath,
				ReadOnly: true,
			}
			uvm.reservedSCSISlots = append(uvm.reservedSCSISlots, scsi.Slot{Controller: 0, LUN: 0})
		}
	}

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

	initArgs := fmt.Sprintf("%s %s", entropyArgs, execCmdArgs)
	if vmDebugging {
		// Launch a shell on the console.
		initArgs = entropyArgs + ` sh -c "` + execCmdArgs + ` & exec sh"`
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
	log.G(ctx).WithField("options", log.Format(ctx, opts)).Debug("uvm::CreateLCOW options")

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
		vpciDevices:             make(map[VPCIDeviceKey]*VPCIDevice),
		physicallyBacked:        !opts.AllowOvercommit,
		devicesPhysicallyBacked: opts.FullyPhysicallyBacked,
		createOpts:              opts,
		vpmemMultiMapping:       !opts.VPMemNoMultiMapping,
		encryptScratch:          opts.EnableScratchEncryption,
		noWritableFileShares:    opts.NoWritableFileShares,
		confidentialUVMOptions:  opts.ConfidentialOptions,
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
		log.G(ctx).Tracef("create_lcow::CreateLCOW makeLCOWSecurityDoc result doc: %v err %v", doc, err)
	} else {
		doc, err = makeLCOWDoc(ctx, opts, uvm)
		log.G(ctx).Tracef("create_lcow::CreateLCOW makeLCOWDoc result doc: %v err %v", doc, err)
	}
	if err != nil {
		return nil, err
	}

	if err = uvm.create(ctx, doc); err != nil {
		return nil, fmt.Errorf("error while creating the compute system: %w", err)
	}
	log.G(ctx).WithField("uvm", uvm).Trace("create_lcow::CreateLCOW uvm.create result")

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
		l, err := uvm.listenVsock(gcs.LinuxGcsVsockPort)
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
