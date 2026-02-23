//go:build windows

package uvm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	"github.com/Microsoft/hcsshim/osversion"
)

// Options are the set of options passed to Create() to create a utility vm.
type Options struct {
	ID    string // Identifier for the uvm. Defaults to generated GUID.
	Owner string // Specifies the owner. Defaults to executable name.

	// MemorySizeInMB sets the UVM memory. If `0` will default to platform
	// default.
	MemorySizeInMB uint64

	LowMMIOGapInMB   uint64
	HighMMIOBaseInMB uint64
	HighMMIOGapInMB  uint64

	// Memory for UVM. Defaults to true. For physical backed memory, set to
	// false.
	AllowOvercommit bool

	// FullyPhysicallyBacked describes if a uvm should be entirely physically
	// backed, including in any additional devices
	FullyPhysicallyBacked bool

	// Memory for UVM. Defaults to false. For virtual memory with deferred
	// commit, set to true.
	EnableDeferredCommit bool

	// ProcessorCount sets the number of vCPU's. If `0` will default to platform
	// default.
	ProcessorCount int32

	// ProcessorLimit sets the maximum percentage of each vCPU's the UVM can
	// consume. If `0` will default to platform default.
	ProcessorLimit int32

	// ProcessorWeight sets the relative weight of these vCPU's vs another UVM's
	// when scheduling. If `0` will default to platform default.
	ProcessorWeight int32

	// StorageQoSIopsMaximum sets the maximum number of Iops. If `0` will
	// default to the platform default.
	StorageQoSIopsMaximum int32

	// StorageQoSIopsMaximum sets the maximum number of bytes per second. If `0`
	// will default to the platform default.
	StorageQoSBandwidthMaximum int32

	// DisableCompartmentNamespace sets whether to disable namespacing the network compartment in the UVM
	// for WCOW. Namespacing makes it so the compartment created for a container is essentially no longer
	// aware or able to see any of the other compartments on the host (in this case the UVM).
	// The compartment that the container is added to now behaves as the default compartment as
	// far as the container is concerned and it is only able to view the NICs in the compartment it's assigned to.
	// This is the compartment setup (and behavior) that is followed for V1 HCS schema containers (docker) so
	// this change brings parity as well. This behavior is gated behind a registry key currently to avoid any
	// unnecessary behavior and once this restriction is removed then we can remove the need for this variable
	// and the associated annotation as well.
	DisableCompartmentNamespace bool

	// CPUGroupID set the ID of a CPUGroup on the host that the UVM should be added to on start.
	// Defaults to an empty string which indicates the UVM should not be added to any CPUGroup.
	CPUGroupID string

	// ResourcePartitionID holds the resource partition guid.GUID the UVM should be assigned to.
	ResourcePartitionID *guid.GUID

	// NetworkConfigProxy holds the address of the network config proxy service.
	// This != "" determines whether to start the ComputeAgent TTRPC service
	// that receives the UVMs set of NICs from this proxy instead of enumerating
	// the endpoints locally.
	NetworkConfigProxy string

	// Sets the location for process dumps to be placed in. On Linux this is a kernel setting so it will be
	// applied to all containers. On Windows it's configurable per container, but we can mimic this for
	// Windows by just applying the location specified here per container.
	ProcessDumpLocation string

	// NoWritableFileShares disables adding any writable vSMB and Plan9 shares to the UVM
	NoWritableFileShares bool

	// The number of SCSI controllers. Defaults to 1 for WCOW and 4 for LCOW
	SCSIControllerCount uint32

	// DumpDirectoryPath is the path of the directory inside which all debug dumps etc are stored.
	DumpDirectoryPath string

	// 	AdditionalHyperVConfig are extra Hyper-V socket configurations to provide.
	AdditionalHyperVConfig map[string]hcsschema.HvSocketServiceConfig

	// The following options are for implicit vNUMA topology settings.
	// MaxMemorySizePerNumaNode is the maximum size of memory (in MiB) per vNUMA node.
	MaxMemorySizePerNumaNode uint64
	// MaxProcessorsPerNumaNode is the maximum number of processors per vNUMA node.
	MaxProcessorsPerNumaNode uint32
	// PhysicalNumaNodes are the preferred physical NUMA nodes to map to vNUMA nodes.
	PreferredPhysicalNumaNodes []uint32

	// The following options are for explicit vNUMA topology settings.
	// NumaMappedPhysicalNodes are the physical NUMA nodes mapped to vNUMA nodes.
	NumaMappedPhysicalNodes []uint32
	// NumaProcessorCounts are the number of processors per vNUMA node.
	NumaProcessorCounts []uint32
	// NumaMemoryBlocksCounts are the number of memory blocks per vNUMA node.
	NumaMemoryBlocksCounts []uint64

	EnableGraphicsConsole bool   // If true, enable a graphics console for the utility VM
	ConsolePipe           string // The named pipe path to use for the serial console (COM1).  eg \\.\pipe\vmpipe
}

type ConfidentialCommonOptions struct {
	GuestStateFilePath     string // The vmgs file path to load
	SecurityPolicy         string // Optional security policy
	SecurityPolicyEnabled  bool   // Set when there is a security policy to apply on actual SNP hardware, use this rathen than checking the string length
	SecurityPolicyEnforcer string // Set which security policy enforcer to use (open door or rego). This allows for better fallback mechanic.
	UVMReferenceInfoFile   string // Path to the file that contains the signed UVM measurements
	BundleDirectory        string // This allows paths to be constructed relative to a per-VM bundle directory.
}

func verifyWCOWBootFiles(bootFiles *WCOWBootFiles) error {
	if bootFiles == nil {
		return fmt.Errorf("boot files is nil")
	}
	switch bootFiles.BootType {
	case VmbFSBoot:
		if bootFiles.VmbFSFiles == nil {
			return fmt.Errorf("VmbFS boot files is empty")
		}
	case BlockCIMBoot:
		if bootFiles.BlockCIMFiles == nil {
			return fmt.Errorf("confidential boot files is empty")
		}
	default:
		return fmt.Errorf("invalid boot type (%d) specified", bootFiles.BootType)
	}
	return nil
}

// Verifies that the final UVM options are correct and supported.
func verifyOptions(_ context.Context, options interface{}) error {
	switch opts := options.(type) {
	case *OptionsLCOW:
		if opts.EnableDeferredCommit && !opts.AllowOvercommit {
			return errors.New("EnableDeferredCommit is not supported on physically backed VMs")
		}
		if opts.SCSIControllerCount > MaxSCSIControllers {
			return fmt.Errorf("SCSI controller count can't be more than %d", MaxSCSIControllers)
		}
		if opts.VPMemDeviceCount > MaxVPMEMCount {
			return fmt.Errorf("VPMem device count cannot be greater than %d", MaxVPMEMCount)
		}
		if opts.VPMemDeviceCount > 0 {
			if opts.VPMemSizeBytes%4096 != 0 {
				return errors.New("VPMemSizeBytes must be a multiple of 4096")
			}
		}
		if opts.KernelDirect && osversion.Build() < 18286 {
			return errors.New("KernelDirectBoot is not supported on builds older than 18286")
		}

		if opts.EnableColdDiscardHint && osversion.Build() < 18967 {
			return errors.New("EnableColdDiscardHint is not supported on builds older than 18967")
		}
		if opts.ResourcePartitionID != nil {
			if opts.CPUGroupID != "" {
				return errors.New("resource partition ID and CPU group ID cannot be set at the same time")
			}
		}
	case *OptionsWCOW:
		if opts.EnableDeferredCommit && !opts.AllowOvercommit {
			return errors.New("EnableDeferredCommit is not supported on physically backed VMs")
		}
		if opts.SCSIControllerCount != 1 {
			return errors.New("exactly 1 SCSI controller is required for WCOW")
		}
		if err := verifyWCOWBootFiles(opts.BootFiles); err != nil {
			return err
		}
		if opts.SecurityPolicyEnabled && opts.GuestStateFilePath == "" {
			return fmt.Errorf("GuestStateFilePath must be provided when enabling security policy")
		}
		if opts.IsolationType == "SecureNestedPaging" && opts.EnableGraphicsConsole {
			return fmt.Errorf("graphics console cannot be enabled with SecureNestedPaging isolation mode")
		}
		if opts.ResourcePartitionID != nil {
			if opts.CPUGroupID != "" {
				return errors.New("resource partition ID and CPU group ID cannot be set at the same time")
			}
		}
	}
	return nil
}

// newDefaultOptions returns the default base options for WCOW and LCOW.
//
// If `id` is empty it will be generated.
//
// If `owner` is empty it will be set to the calling executables name.
func newDefaultOptions(id, owner string) *Options {
	opts := &Options{
		ID:                     id,
		Owner:                  owner,
		MemorySizeInMB:         1024,
		AllowOvercommit:        true,
		EnableDeferredCommit:   false,
		ProcessorCount:         vmutils.DefaultProcessorCountForUVM(),
		FullyPhysicallyBacked:  false,
		NoWritableFileShares:   false,
		SCSIControllerCount:    1,
		AdditionalHyperVConfig: make(map[string]hcsschema.HvSocketServiceConfig),
	}

	if opts.Owner == "" {
		opts.Owner = filepath.Base(os.Args[0])
	}

	return opts
}

// ID returns the ID of the VM's compute system.
func (uvm *UtilityVM) ID() string {
	return uvm.hcsSystem.ID()
}

// RuntimeID returns Hyper-V VM GUID.
//
// Only valid after the utility VM has been created.
func (uvm *UtilityVM) RuntimeID() guid.GUID {
	return uvm.runtimeID
}

// OS returns the operating system of the utility VM.
func (uvm *UtilityVM) OS() string {
	return uvm.operatingSystem
}

func (uvm *UtilityVM) create(ctx context.Context, doc interface{}) error {
	uvm.exitCh = make(chan struct{})
	system, err := hcs.CreateComputeSystem(ctx, uvm.id, doc)
	if err != nil {
		return err
	}
	defer func() {
		if system != nil {
			_ = system.Terminate(ctx)
			_ = system.WaitCtx(ctx)
		}
	}()

	// Cache the VM ID of the utility VM.
	properties, err := system.Properties(ctx)
	if err != nil {
		return err
	}
	uvm.runtimeID = properties.RuntimeID
	uvm.hcsSystem = system
	system = nil

	log.G(ctx).WithFields(logrus.Fields{
		logfields.UVMID: uvm.id,
		"runtime-id":    uvm.runtimeID.String(),
	}).Debug("created utility VM")
	return nil
}

// Close terminates and releases resources associated with the utility VM.
func (uvm *UtilityVM) Close() error { return uvm.CloseCtx(context.Background()) }

// CloseCtx is similar to [UtilityVM.Close], but accepts a context.
//
// The context is used for all operations, including waits, so timeouts/cancellations may prevent
// proper uVM cleanup.
func (uvm *UtilityVM) CloseCtx(ctx context.Context) (err error) {
	ctx, span := oc.StartSpan(ctx, "uvm::Close")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.UVMID, uvm.id))

	// TODO: check if uVM already closed

	windows.Close(uvm.vmmemProcess)

	if uvm.hcsSystem != nil {
		_ = uvm.hcsSystem.Terminate(ctx)
		// uvm.Wait() waits on <-uvm.outputProcessingDone, which may not be closed until below
		// (for a Create -> Stop without a Start), or uvm.outputHandler may be blocked on IO and
		// take a while to close.
		// In either case, we want to wait on the system closing, not IO completion.
		_ = uvm.hcsSystem.WaitCtx(ctx)
	}

	if err := uvm.CloseGCSConnection(); err != nil {
		log.G(ctx).Errorf("close GCS connection failed: %s", err)
	}

	// outputListener will only be nil for a Create -> Stop without a Start. In
	// this case we have no goroutine processing output so its safe to close the
	// channel here.
	if uvm.outputListener != nil {
		close(uvm.outputProcessingDone)
		uvm.outputListener.Close()
		uvm.outputListener = nil
	}

	if uvm.hcsSystem != nil {
		// wait for IO to finish
		// [WaitCtx] calls [uvm.hcsSystem.WaitCtx] again, but since we waited on it above already
		// it should nop and return without issue.
		_ = uvm.WaitCtx(ctx)
	}

	if lopts, ok := uvm.createOpts.(*OptionsLCOW); ok && uvm.HasConfidentialPolicy() && lopts.GuestStateFilePath != "" {
		vmgsFullPath := filepath.Join(lopts.BundleDirectory, lopts.GuestStateFilePath)
		e := log.G(ctx).WithField("VMGS file", vmgsFullPath)
		e.Debug("removing VMGS file")
		if err := os.Remove(vmgsFullPath); err != nil {
			e.WithError(err).Error("failed to remove VMGS file")
		}
	}

	if uvm.hcsSystem != nil {
		return uvm.hcsSystem.CloseCtx(ctx)
	}

	return nil
}

// CreateContainer creates a container in the utility VM.
func (uvm *UtilityVM) CreateContainer(ctx context.Context, id string, settings interface{}) (cow.Container, error) {
	if uvm.gc != nil {
		c, err := uvm.gc.CreateContainer(ctx, id, settings)
		if err != nil {
			return nil, fmt.Errorf("failed to create container %s: %w", id, err)
		}
		return c, nil
	}
	doc := hcsschema.ComputeSystem{
		HostingSystemId:                   uvm.id,
		Owner:                             uvm.owner,
		SchemaVersion:                     schemaversion.SchemaV21(),
		ShouldTerminateOnLastHandleClosed: true,
		HostedSystem:                      settings,
	}
	c, err := hcs.CreateComputeSystem(ctx, id, &doc)
	if err != nil {
		return nil, err
	}
	return c, err
}

// CreateProcess creates a process in the utility VM.
func (uvm *UtilityVM) CreateProcess(ctx context.Context, settings interface{}) (cow.Process, error) {
	if uvm.gc != nil {
		return uvm.gc.CreateProcess(ctx, settings)
	}
	return uvm.hcsSystem.CreateProcess(ctx, settings)
}

// IsOCI returns false, indicating the parameters to CreateProcess should not
// include an OCI spec.
func (*UtilityVM) IsOCI() bool {
	return false
}

// Terminate requests that the utility VM be terminated.
func (uvm *UtilityVM) Terminate(ctx context.Context) error {
	return uvm.hcsSystem.Terminate(ctx)
}

// ExitError returns an error if the utility VM has terminated unexpectedly.
func (uvm *UtilityVM) ExitError() error {
	return uvm.hcsSystem.ExitError()
}

// ProcessorCount returns the number of processors actually assigned to the UVM.
func (uvm *UtilityVM) ProcessorCount() int32 {
	return uvm.processorCount
}

// PhysicallyBacked returns if the UVM is backed by physical memory
// (Over commit and deferred commit both false).
func (uvm *UtilityVM) PhysicallyBacked() bool {
	return uvm.physicallyBacked
}

// ProcessDumpLocation returns the location that process dumps will get written to for containers running
// in the UVM.
func (uvm *UtilityVM) ProcessDumpLocation() string {
	return uvm.processDumpLocation
}

// DevicesPhysicallyBacked describes if additional devices added to the UVM
// should be physically backed.
func (uvm *UtilityVM) DevicesPhysicallyBacked() bool {
	return uvm.devicesPhysicallyBacked
}

// VSMBNoDirectMap returns if VSMB devices should be mounted with `NoDirectMap` set to true.
func (uvm *UtilityVM) VSMBNoDirectMap() bool {
	return uvm.vsmbNoDirectMap
}

func (uvm *UtilityVM) NoWritableFileShares() bool {
	return uvm.noWritableFileShares
}

// Closes the external GCS connection if it is being used and also closes the
// listener for GCS connection.
func (uvm *UtilityVM) CloseGCSConnection() (err error) {
	// TODO: errors.Join to avoid ignoring an error
	if uvm.gc != nil {
		err = uvm.gc.Close()
	}
	if uvm.gcListener != nil {
		err = uvm.gcListener.Close()
	}
	return err
}
