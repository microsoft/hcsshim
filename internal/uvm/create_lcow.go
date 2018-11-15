package uvm

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guid"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/mergemaps"
	"github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/linuxkit/virtsock/pkg/hvsock"
	"github.com/sirupsen/logrus"
)

type PreferredRootFSType int

const (
	PreferredRootFSTypeInitRd = 0
	PreferredRootFSTypeVHD    = 1

	initrdFile = "initrd.img"
	vhdFile    = "rootfs.vhd"
)

// OptionsLCOW are the set of options passed to CreateLCOW() to create a utility vm.
type OptionsLCOW struct {
	*Options

	BootFilesPath string // Folder in which kernel and root file system reside. Defaults to \Program Files\Linux Containers
	KernelFile    string // Filename under BootFilesPath for the kernel. Defaults to `kernel`
	RootFSFile    string // Filename under BootFilesPath for the UVMs root file system. Defaults are `initrd.img` or `rootfs.vhd` based on `PreferredRootFSType`.
	// PreferredRootFSType controls searching for the RootFSFile. If left default will use `initrd.img`
	PreferredRootFSType   PreferredRootFSType
	KernelBootOptions     string // Additional boot options for the kernel
	EnableGraphicsConsole bool   // If true, enable a graphics console for the utility VM
	ConsolePipe           string // The named pipe path to use for the serial console.  eg \\.\pipe\vmpipe
	SCSIControllerCount   *uint  // The number of SCSI controllers. Defaults to 1 if omitted. Currently we only support 0 or 1.

	// Fields that can be configured via OCI annotations in runhcs.
	VPMemDeviceCount *uint32 // Number of VPMem devices. Limit at 128. If booting UVM from VHD, device 0 is taken. LCOW Only. io.microsoft.virtualmachine.devices.virtualpmem.maximumcount
	VPMemSizeBytes   *uint64 // Size of the VPMem devices. LCOW Only. Defaults to 4GB. io.microsoft.virtualmachine.devices.virtualpmem.maximumsizebytes
}

const linuxLogVsockPort = 109

// CreateLCOW creates an HCS compute system representing a utility VM.
func CreateLCOW(opts *OptionsLCOW) (_ *UtilityVM, err error) {
	logrus.Debugf("uvm::CreateLCOW %+v", opts)

	if opts.Options == nil {
		opts.Options = &Options{}
	}

	uvm := &UtilityVM{
		id:                  opts.ID,
		owner:               opts.Owner,
		operatingSystem:     "linux",
		scsiControllerCount: 1,
		vpmemMaxCount:       DefaultVPMEMCount,
		vpmemMaxSizeBytes:   DefaultVPMemSizeBytes,
	}

	// Defaults if omitted by caller.
	// TODO: Change this. Don't auto generate ID if omitted. Avoids the chicken-and-egg problem
	if uvm.id == "" {
		uvm.id = guid.New().String()
	}
	if uvm.owner == "" {
		uvm.owner = filepath.Base(os.Args[0])
	}

	if opts.BootFilesPath == "" {
		opts.BootFilesPath = filepath.Join(os.Getenv("ProgramFiles"), "Linux Containers")
	}
	if opts.KernelFile == "" {
		opts.KernelFile = "kernel"
	}
	if _, err := os.Stat(filepath.Join(opts.BootFilesPath, opts.KernelFile)); os.IsNotExist(err) {
		return nil, fmt.Errorf("kernel '%s' not found", filepath.Join(opts.BootFilesPath, opts.KernelFile))
	}

	if opts.RootFSFile == "" {
		switch opts.PreferredRootFSType {
		case PreferredRootFSTypeInitRd:
			opts.RootFSFile = initrdFile
		case PreferredRootFSTypeVHD:
			opts.RootFSFile = "rootfs.vhd"
		}
	}

	if _, err := os.Stat(filepath.Join(opts.BootFilesPath, opts.RootFSFile)); os.IsNotExist(err) {
		return nil, fmt.Errorf("%s not found under %s", opts.RootFSFile, opts.BootFilesPath)
	}

	if opts.SCSIControllerCount != nil {
		if *opts.SCSIControllerCount > 1 {
			return nil, fmt.Errorf("SCSI controller count must be 0 or 1") // Future extension here for up to 4
		}
		uvm.scsiControllerCount = *opts.SCSIControllerCount
	}
	if opts.VPMemDeviceCount != nil {
		if *opts.VPMemDeviceCount > MaxVPMEMCount {
			return nil, fmt.Errorf("vpmem device count cannot be greater than %d", MaxVPMEMCount)
		}
		uvm.vpmemMaxCount = *opts.VPMemDeviceCount
	}
	if uvm.vpmemMaxCount > 0 {
		if opts.VPMemSizeBytes != nil {
			if *opts.VPMemSizeBytes%4096 != 0 {
				return nil, fmt.Errorf("opts.VPMemSizeBytes must be a multiple of 4096")
			}
			uvm.vpmemMaxSizeBytes = *opts.VPMemSizeBytes
		}
	} else {
		if opts.PreferredRootFSType == PreferredRootFSTypeVHD {
			return nil, fmt.Errorf("PreferredRootFSTypeVHD requires at least one VPMem device")
		}
	}

	doc := &hcsschema.ComputeSystem{
		Owner:         uvm.owner,
		SchemaVersion: schemaversion.SchemaV21(),
		VirtualMachine: &hcsschema.VirtualMachine{
			Chipset: &hcsschema.Chipset{
				Uefi: &hcsschema.Uefi{
					BootThis: &hcsschema.UefiBootEntry{
						DevicePath: `\` + opts.KernelFile,
						DeviceType: "VmbFs",
					},
				},
			},
			ComputeTopology: &hcsschema.Topology{
				Memory: &hcsschema.Memory2{
					SizeInMB: getMemory(opts.Resources),
					// AllowOvercommit `true` by default if not passed.
					AllowOvercommit: opts.AllowOvercommit == nil || *opts.AllowOvercommit,
					// EnableDeferredCommit `false` by default if not passed.
					EnableDeferredCommit: opts.EnableDeferredCommit != nil && *opts.EnableDeferredCommit,
				},
				Processor: &hcsschema.Processor2{
					Count: getProcessors(opts.Resources),
				},
			},
			GuestConnection: &hcsschema.GuestConnection{
				UseVsock:            true,
				UseConnectedSuspend: true,
			},
			Devices: &hcsschema.Devices{
				HvSocket: &hcsschema.HvSocket2{
					HvSocketConfig: &hcsschema.HvSocketSystemConfig{
						// Allow administrators and SYSTEM to bind to vsock sockets
						// so that we can create a GCS log socket.
						DefaultBindSecurityDescriptor: "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
					},
				},
				VirtualSmb: &hcsschema.VirtualSmb{
					Shares: []hcsschema.VirtualSmbShare{
						{
							Name: "os",
							Path: opts.BootFilesPath,
							Options: &hcsschema.VirtualSmbShareOptions{
								ReadOnly:            true,
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

	if uvm.scsiControllerCount > 0 {
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
		kernelArgs = "initrd=/" + opts.RootFSFile
	case PreferredRootFSTypeVHD:
		// Support for VPMem VHD(X) booting rather than initrd..
		kernelArgs = "root=/dev/pmem0 init=/init"
		imageFormat := "Vhd1"
		if strings.ToLower(filepath.Ext(opts.RootFSFile)) == "vhdx" {
			imageFormat = "Vhdx"
		}
		doc.VirtualMachine.Devices.VirtualPMem.Devices = map[string]hcsschema.VirtualPMemDevice{
			"0": {
				HostPath:    filepath.Join(opts.BootFilesPath, opts.RootFSFile),
				ReadOnly:    true,
				ImageFormat: imageFormat,
			},
		}
		if err := wclayer.GrantVmAccess(uvm.id, filepath.Join(opts.BootFilesPath, opts.RootFSFile)); err != nil {
			return nil, fmt.Errorf("faied to grantvmaccess to %s: %s", filepath.Join(opts.BootFilesPath, opts.RootFSFile), err)
		}
		// Add to our internal structure
		uvm.vpmemDevices[0] = vpmemInfo{
			hostPath: opts.RootFSFile,
			uvmPath:  "/",
			refCount: 1,
		}
	}

	vmDebugging := false
	if opts.ConsolePipe != "" {
		vmDebugging = true
		kernelArgs += " console=ttyS0,115200"
		doc.VirtualMachine.Devices.ComPorts = map[string]hcsschema.ComPort{
			"0": { // Which is actually COM1
				NamedPipe: opts.ConsolePipe,
			},
		}
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
		kernelArgs += " panic=-1"
	}

	if opts.KernelBootOptions != "" {
		kernelArgs += " " + opts.KernelBootOptions
	}

	// Start GCS with stderr pointing to the vsock port created below in
	// order to forward guest logs to logrus.
	initArgs := fmt.Sprintf("/bin/vsockexec -e %d /bin/gcs -log-format json -loglevel %s",
		linuxLogVsockPort,
		logrus.StandardLogger().Level.String())

	if vmDebugging {
		// Launch a shell on the console.
		initArgs = `sh -c "` + initArgs + ` & exec sh"`
	}

	kernelArgs += ` -- ` + initArgs
	doc.VirtualMachine.Chipset.Uefi.BootThis.OptionalData = kernelArgs

	fullDoc, err := mergemaps.MergeJSON(doc, ([]byte)(opts.AdditionHCSDocumentJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to merge additional JSON '%s': %s", opts.AdditionHCSDocumentJSON, err)
	}

	hcsSystem, err := hcs.CreateComputeSystem(uvm.id, fullDoc)
	if err != nil {
		logrus.Debugln("failed to create UVM: ", err)
		return nil, err
	}

	uvm.hcsSystem = hcsSystem
	defer func() {
		if err != nil {
			uvm.Close()
		}
	}()

	// Create a socket that the GCS can send logrus log data to.
	uvm.gcslog, err = uvm.listenVsock(linuxLogVsockPort)
	if err != nil {
		return nil, err
	}

	return uvm, nil
}

func (uvm *UtilityVM) listenVsock(port uint32) (net.Listener, error) {
	properties, err := uvm.hcsSystem.Properties()
	if err != nil {
		return nil, err
	}
	vmID, err := hvsock.GUIDFromString(properties.RuntimeID)
	if err != nil {
		return nil, err
	}
	serviceID, _ := hvsock.GUIDFromString("00000000-facb-11e6-bd58-64006a7986d3")
	binary.LittleEndian.PutUint32(serviceID[0:4], port)
	return hvsock.Listen(hvsock.Addr{VMID: vmID, ServiceID: serviceID})
}

// PMemMaxSizeBytes returns the maximum size of a PMEM layer (LCOW)
func (uvm *UtilityVM) PMemMaxSizeBytes() uint64 {
	return uvm.vpmemMaxSizeBytes
}
