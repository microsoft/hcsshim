package uvm

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/gcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/processorinfo"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/osversion"
)

// General infomation about how this works at a high level.
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

// OutputHandler is used to process the output from the program run in the UVM.
type OutputHandler func(io.Reader)

const (
	// InitrdFile is the default file name for an initrd.img used to boot LCOW.
	InitrdFile = "initrd.img"
	// VhdFile is the default file name for a rootfs.vhd used to boot LCOW.
	VhdFile = "rootfs.vhd"
	// KernelFile is the default file name for a kernel used to boot LCOW.
	KernelFile = "kernel"
	// UncompressedKernelFile is the default file name for an uncompressed
	// kernel used to boot LCOW with KernelDirect.
	UncompressedKernelFile = "vmlinux"
	// In the SNP case both the kernel (bzImage) and initrd are stored in a vmgs (VM Guest State) file
	GuestStateFile = "kernelinitrd.vmgs"
)

// OptionsLCOW are the set of options passed to CreateLCOW() to create a utility vm.
type OptionsLCOW struct {
	*Options

	BootFilesPath           string              // Folder in which kernel and root file system reside. Defaults to \Program Files\Linux Containers
	KernelFile              string              // Filename under `BootFilesPath` for the kernel. Defaults to `kernel`
	KernelDirect            bool                // Skip UEFI and boot directly to `kernel`
	RootFSFile              string              // Filename under `BootFilesPath` for the UVMs root file system. Defaults to `InitrdFile`
	KernelBootOptions       string              // Additional boot options for the kernel
	EnableGraphicsConsole   bool                // If true, enable a graphics console for the utility VM
	ConsolePipe             string              // The named pipe path to use for the serial console.  eg \\.\pipe\vmpipe
	SCSIControllerCount     uint32              // The number of SCSI controllers. Defaults to 1. Currently we only support 0 or 1.
	UseGuestConnection      bool                // Whether the HCS should connect to the UVM's GCS. Defaults to true
	ExecCommandLine         string              // The command line to exec from init. Defaults to GCS
	ForwardStdout           bool                // Whether stdout will be forwarded from the executed program. Defaults to false
	ForwardStderr           bool                // Whether stderr will be forwarded from the executed program. Defaults to true
	OutputHandler           OutputHandler       `json:"-"` // Controls how output received over HVSocket from the UVM is handled. Defaults to parsing output as logrus messages
	VPMemDeviceCount        uint32              // Number of VPMem devices. Defaults to `DefaultVPMEMCount`. Limit at 128. If booting UVM from VHD, device 0 is taken.
	VPMemSizeBytes          uint64              // Size of the VPMem devices. Defaults to `DefaultVPMemSizeBytes`.
	VPMemNoMultiMapping     bool                // Disables LCOW layer multi mapping
	PreferredRootFSType     PreferredRootFSType // If `KernelFile` is `InitrdFile` use `PreferredRootFSTypeInitRd`. If `KernelFile` is `VhdFile` use `PreferredRootFSTypeVHD`
	EnableColdDiscardHint   bool                // Whether the HCS should use cold discard hints. Defaults to false
	VPCIEnabled             bool                // Whether the kernel should enable pci
	EnableScratchEncryption bool                // Whether the scratch should be encrypted
	SecurityPolicy          string              // Optional security policy
	SecurityPolicyEnabled   bool                // Set when there is a security policy to apply on actual SNP hardware, use this rathen than checking the string length
	UseGuestStateFile       bool                // Use a vmgs file that contains a kernel and initrd, required for SNP
	GuestStateFile          string              // The vmgs file to load
	DisableTimeSyncService  bool                // Disables the time synchronization service
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
		BootFilesPath:           defaultLCOWOSBootFilesPath(),
		KernelFile:              KernelFile,
		KernelDirect:            kernelDirectSupported,
		RootFSFile:              InitrdFile,
		KernelBootOptions:       "",
		EnableGraphicsConsole:   false,
		ConsolePipe:             "",
		SCSIControllerCount:     1,
		UseGuestConnection:      true,
		ExecCommandLine:         fmt.Sprintf("/bin/gcs -v4 -log-format json -loglevel %s", logrus.StandardLogger().Level.String()),
		ForwardStdout:           false,
		ForwardStderr:           true,
		OutputHandler:           parseLogrus(id),
		VPMemDeviceCount:        DefaultVPMEMCount,
		VPMemSizeBytes:          DefaultVPMemSizeBytes,
		VPMemNoMultiMapping:     osversion.Get().Build < osversion.V19H1,
		PreferredRootFSType:     PreferredRootFSTypeInitRd,
		EnableColdDiscardHint:   false,
		VPCIEnabled:             false,
		EnableScratchEncryption: false,
		SecurityPolicyEnabled:   false,
		SecurityPolicy:          "",
		GuestStateFile:          "",
		DisableTimeSyncService:  false,
	}

	if _, err := os.Stat(filepath.Join(opts.BootFilesPath, VhdFile)); err == nil {
		// We have a rootfs.vhd in the boot files path. Use it over an initrd.img
		opts.RootFSFile = VhdFile
		opts.PreferredRootFSType = PreferredRootFSTypeVHD
	}

	if kernelDirectSupported {
		// KernelDirect supports uncompressed kernel if the kernel is present.
		// Default to uncompressed if on box. NOTE: If `kernel` is already
		// uncompressed and simply named 'kernel' it will still be used
		// uncompressed automatically.
		if _, err := os.Stat(filepath.Join(opts.BootFilesPath, UncompressedKernelFile)); err == nil {
			opts.KernelFile = UncompressedKernelFile
		}
	}
	return opts
}

// Get an acceptable number of processors given option and actual constraints.
func fetchProcessor(ctx context.Context, opts *OptionsLCOW, uvm *UtilityVM) (*hcsschema.Processor2, error) {
	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host processor information: %s", err)
	}

	// To maintain compatability with Docker we need to automatically downgrade
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
            }
        },
        "GuestState": {
            "GuestStateFilePath": "d:\\ken\\aug27\\gcsinitnew.vmgs",
            "GuestStateFileType": "BlockStorage",
			"ForceTransientState": true
        },
        "SecuritySettings": {
            "Isolation": {
                "IsolationType": "SecureNestedPaging",
                "LaunchData": "kBifgKNijdHjxdSUshmavrNofo2B01LiIi1cr8R4ytI=",
                "HclEnabled": true
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

// Make a hcsschema.ComputeSytem with the parts that target booting from a VMGS file
func makeLCOWVMGSDoc(ctx context.Context, opts *OptionsLCOW, uvm *UtilityVM) (_ *hcsschema.ComputeSystem, err error) {

	// Kernel and initrd are combined into a single vmgs file.
	vmgsFullPath := filepath.Join(opts.BootFilesPath, opts.GuestStateFile)
	if _, err := os.Stat(vmgsFullPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("the GuestState vmgs file '%s' was not found", vmgsFullPath)
	}

	var processor *hcsschema.Processor2
	processor, err = fetchProcessor(ctx, opts, uvm)
	if err != nil {
		return nil, err
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
			},
		},
	}

	// Set permissions for the VSock ports:
	//		entropyVsockPort - 1 is the entropy port,
	//		linuxLogVsockPort - 109 used by vsockexec to log stdout/stderr logging,
	//		0x40000000 + 1 (LinuxGcsVsockPort + 1) is the bridge (see guestconnectiuon.go)

	hvSockets := [...]uint32{entropyVsockPort, linuxLogVsockPort, gcs.LinuxGcsVsockPort, gcs.LinuxGcsVsockPort + 1}
	for _, whichSocket := range hvSockets {
		key := fmt.Sprintf("%08x-facb-11e6-bd58-64006a7986d3", whichSocket) // format of a linux hvsock GUID is port#-facb-11e6-bd58-64006a7986d3
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
		// TODO: JTERRY75 - this should enumerate scsicount and add an entry per value.
		doc.VirtualMachine.Devices.Scsi = map[string]hcsschema.Scsi{
			"0": {
				Attachments: make(map[string]hcsschema.Attachment),
			},
		}
	}

	// The rootfs must be provided as an initrd within the VMGS file.
	// Raise an error if instructed to use a particular sort of rootfs.

	if opts.PreferredRootFSType != PreferredRootFSTypeNA {
		return nil, fmt.Errorf("cannot override rootfs when using VMGS file")
	}

	// Required by HCS for the isolated boot scheme, see also https://docs.microsoft.com/en-us/windows-server/virtualization/hyper-v/learn-more/generation-2-virtual-machine-security-settings-for-hyper-v
	// A complete explanation of the why's and wherefores of starting an encrypted, isolated VM are beond the scope of these comments.

	doc.VirtualMachine.Chipset.Uefi = &hcsschema.Uefi{
		ApplySecureBootTemplate: "Apply",
		SecureBootTemplateId:    "1734c6e8-3154-4dda-ba5f-a874cc483422", // aka MicrosoftWindowsSecureBootTemplateGUID equivilent to "Microsoft Windows" template from Get-VMHost | select SecureBootTemplates,

	}

	// Point at the file that contains the linux kernel and initrd images.

	doc.VirtualMachine.GuestState = &hcsschema.GuestState{
		GuestStateFilePath:  vmgsFullPath,
		GuestStateFileType:  "BlockStorage",
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

	// First, decode the base64 string into a human readable (json) string .
	jsonPolicy, err := base64.StdEncoding.DecodeString(opts.SecurityPolicy)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 SecurityPolicy")
	}

	// make a sha256 hashing object
	hostData := sha256.New()
	// give it the jsaon string to measure
	hostData.Write(jsonPolicy)
	// get the measurement out
	securityPolicyHash := base64.StdEncoding.EncodeToString(hostData.Sum(nil))

	// Put the measurement into the LaunchData field of the HCS creation command.
	// This will endup in HOST_DATA of SNP_LAUNCH_FINISH command the and ATTESTATION_REPORT
	// retrieved by the guest later.

	doc.VirtualMachine.SecuritySettings = &hcsschema.SecuritySettings{
		EnableTpm: false,
		Isolation: &hcsschema.IsolationSettings{
			IsolationType: "SecureNestedPaging",
			LaunchData:    securityPolicyHash,
			HclEnabled:    true,
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

// Make the ComputeSystem document object that will be serialised to json to be presented to the HCS api.
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
	processor, err = fetchProcessor(ctx, opts, uvm) // must happen after the file existance tests above.
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
					},
				},
				Plan9: &hcsschema.Plan9{},
			},
		},
	}

	// Handle StorageQoS if set
	if opts.StorageQoSBandwidthMaximum > 0 || opts.StorageQoSIopsMaximum > 0 {
		doc.VirtualMachine.StorageQoS = &hcsschema.StorageQoS{
			IopsMaximum:      opts.StorageQoSIopsMaximum,
			BandwidthMaximum: opts.StorageQoSBandwidthMaximum,
		}
	}

	if uvm.scsiControllerCount > 0 {
		// TODO: JTERRY75 - this should enumerate scsicount and add an entry per value.
		doc.VirtualMachine.Devices.Scsi = map[string]hcsschema.Scsi{
			"0": {
				Attachments: make(map[string]hcsschema.Attachment),
			},
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

			st, err := os.Stat(rootfsFullPath)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to stat rootfs: %q", rootfsFullPath)
			}
			devSize := pageAlign(uint64(st.Size()))
			memReg, err := pmem.Allocate(devSize)
			if err != nil {
				return nil, errors.Wrap(err, "failed to allocate memory for rootfs")
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
	initArgs := fmt.Sprintf("-e %d", entropyVsockPort)

	// With default options, run GCS with stderr pointing to the vsock port
	// created below in order to forward guest logs to logrus.
	initArgs += " /bin/vsockexec"

	if opts.ForwardStdout {
		initArgs += fmt.Sprintf(" -o %d", linuxLogVsockPort)
	}

	if opts.ForwardStderr {
		initArgs += fmt.Sprintf(" -e %d", linuxLogVsockPort)
	}

	if opts.DisableTimeSyncService {
		opts.ExecCommandLine = fmt.Sprintf("%s --disable-time-sync", opts.ExecCommandLine)
	}

	initArgs += " " + opts.ExecCommandLine

	if opts.ProcessDumpLocation != "" {
		initArgs += " -core-dump-location " + opts.ProcessDumpLocation
	}

	if vmDebugging {
		// Launch a shell on the console.
		initArgs = `sh -c "` + initArgs + ` & exec sh"`
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

// Creates an HCS compute system representing a utility VM. It consumes a set of options derived
// from various defaults and options expressed as annotations.
func CreateLCOW(ctx context.Context, opts *OptionsLCOW) (_ *UtilityVM, err error) {
	ctx, span := trace.StartSpan(ctx, "uvm::CreateLCOW")
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
	log.G(ctx).WithField("options", fmt.Sprintf("%+v", opts)).Debug("uvm::CreateLCOW options")

	// We dont serialize OutputHandler so if it is missing we need to put it back to the default.
	if opts.OutputHandler == nil {
		opts.OutputHandler = parseLogrus(opts.ID)
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
	}

	defer func() {
		if err != nil {
			uvm.Close()
		}
	}()

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

	err = uvm.create(ctx, doc)

	log.G(ctx).Tracef("create_lcow::CreateLCOW uvm.create result uvm: %v err %v", uvm, err)

	if err != nil {
		return nil, fmt.Errorf("error while creating the compute system: %s", err)
	}

	// Cerate a socket to inject entropy during boot.
	uvm.entropyListener, err = uvm.listenVsock(entropyVsockPort)
	if err != nil {
		return nil, err
	}

	// Create a socket that the executed program can send to. This is usually
	// used by GCS to send log data.
	if opts.ForwardStdout || opts.ForwardStderr {
		uvm.outputHandler = opts.OutputHandler
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
