package uvm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"golang.org/x/sys/windows"
)

// Options are the set of options passed to Create() to create a utility vm.
type Options struct {
	ID                      string // Identifier for the uvm. Defaults to generated GUID.
	Owner                   string // Specifies the owner. Defaults to executable name.
	AdditionHCSDocumentJSON string // Optional additional JSON to merge into the HCS document prior

	// MemorySizeInMB sets the UVM memory. If `0` will default to platform
	// default.
	MemorySizeInMB int32

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

	// ExternalGuestConnection sets whether the guest RPC connection is performed
	// internally by the OS platform or externally by this package.
	ExternalGuestConnection bool
}

// Verifies that the final UVM options are correct and supported.
func verifyOptions(ctx context.Context, options interface{}) error {
	switch opts := options.(type) {
	case *OptionsLCOW:
		if opts.EnableDeferredCommit && !opts.AllowOvercommit {
			return errors.New("EnableDeferredCommit is not supported on physically backed VMs")
		}
		if opts.SCSIControllerCount > 1 {
			return errors.New("SCSI controller count must be 0 or 1") // Future extension here for up to 4
		}
		if opts.VPMemDeviceCount > MaxVPMEMCount {
			return fmt.Errorf("VPMem device count cannot be greater than %d", MaxVPMEMCount)
		}
		if opts.VPMemDeviceCount > 0 {
			if opts.VPMemSizeBytes%4096 != 0 {
				return errors.New("VPMemSizeBytes must be a multiple of 4096")
			}
		} else {
			if opts.PreferredRootFSType == PreferredRootFSTypeVHD {
				return errors.New("PreferredRootFSTypeVHD requires at least one VPMem device")
			}
		}
		if opts.KernelDirect && osversion.Get().Build < 18286 {
			return errors.New("KernelDirectBoot is not supported on builds older than 18286")
		}

		if opts.EnableColdDiscardHint && osversion.Get().Build < 18967 {
			return errors.New("EnableColdDiscardHint is not supported on builds older than 18967")
		}
	case *OptionsWCOW:
		if opts.EnableDeferredCommit && !opts.AllowOvercommit {
			return errors.New("EnableDeferredCommit is not supported on physically backed VMs")
		}
		if len(opts.LayerFolders) < 2 {
			return errors.New("at least 2 LayerFolders must be supplied")
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
		ID:                      id,
		Owner:                   owner,
		MemorySizeInMB:          1024,
		AllowOvercommit:         true,
		EnableDeferredCommit:    false,
		ProcessorCount:          defaultProcessorCount(),
		ExternalGuestConnection: true,
		FullyPhysicallyBacked:   false,
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
			system.Terminate(ctx)
			system.Wait()
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
func (uvm *UtilityVM) Close() (err error) {
	ctx, span := trace.StartSpan(context.Background(), "uvm::Close")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.UVMID, uvm.id))

	windows.Close(uvm.vmmemProcess)

	if uvm.hcsSystem != nil {
		uvm.hcsSystem.Terminate(ctx)
		uvm.Wait()
	}
	if uvm.gc != nil {
		uvm.gc.Close()
	}
	if uvm.gcListener != nil {
		uvm.gcListener.Close()
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
		return uvm.hcsSystem.Close()
	}
	return nil
}

// CreateContainer creates a container in the utility VM.
func (uvm *UtilityVM) CreateContainer(ctx context.Context, id string, settings interface{}) (cow.Container, error) {
	if uvm.gc != nil {
		c, err := uvm.gc.CreateContainer(ctx, id, settings)
		if err != nil {
			return nil, fmt.Errorf("failed to create container %s: %s", id, err)
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
func (uvm *UtilityVM) IsOCI() bool {
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

func defaultProcessorCount() int32 {
	if runtime.NumCPU() == 1 {
		return 1
	}
	return 2
}

// normalizeProcessorCount sets `uvm.processorCount` to `Min(requested,
// logical CPU count)`.
func (uvm *UtilityVM) normalizeProcessorCount(ctx context.Context, requested int32, processorTopology *hcsschema.ProcessorTopology) int32 {
	// Use host processor information retrieved from HCS instead of runtime.NumCPU,
	// GetMaximumProcessorCount or other OS level calls for two reasons.
	// 1. Go uses GetProcessAffinityMask and falls back to GetSystemInfo both of
	// which will not return LPs in another processor group.
	// 2. GetMaximumProcessorCount will return all processors on the system
	// but in configurations where the host partition doesn't see the full LP count
	// i.e "Minroot" scenarios this won't be sufficient.
	// (https://docs.microsoft.com/en-us/windows-server/virtualization/hyper-v/manage/manage-hyper-v-minroot-2016)
	hostCount := int32(processorTopology.LogicalProcessorCount)
	if requested > hostCount {
		log.G(ctx).WithFields(logrus.Fields{
			logfields.UVMID: uvm.id,
			"requested":     requested,
			"assigned":      hostCount,
		}).Warn("Changing user requested CPUCount to current number of processors")
		return hostCount
	} else {
		return requested
	}
}

// ProcessorCount returns the number of processors actually assigned to the UVM.
func (uvm *UtilityVM) ProcessorCount() int32 {
	return uvm.processorCount
}

// PhysicallyBacked returns if the UVM is backed by physical memory
// (Over commit and deferred commit both false)
func (uvm *UtilityVM) PhysicallyBacked() bool {
	return uvm.physicallyBacked
}

func (uvm *UtilityVM) normalizeMemorySize(ctx context.Context, requested int32) int32 {
	actual := (requested + 1) &^ 1 // align up to an even number
	if requested != actual {
		log.G(ctx).WithFields(logrus.Fields{
			logfields.UVMID: uvm.id,
			"requested":     requested,
			"assigned":      actual,
		}).Warn("Changing user requested MemorySizeInMB to align to 2MB")
	}
	return actual
}

// DevicesPhysicallyBacked describes if additional devices added to the UVM
// should be physically backed
func (uvm *UtilityVM) DevicesPhysicallyBacked() bool {
	return uvm.devicesPhysicallyBacked
}
