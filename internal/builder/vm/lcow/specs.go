//go:build windows

package lcow

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	iannotations "github.com/Microsoft/hcsshim/internal/annotations"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	"github.com/Microsoft/hcsshim/osversion"
	shimannotations "github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/sandbox-spec/vm/v2"

	"github.com/sirupsen/logrus"
)

// shimName is the name of lcow shim implementation.
const shimName = "containerd-shim-lcow-v2"

// BuildSandboxConfig is the primary entry point for generating the HCS ComputeSystem
// document used to create an LCOW Utility VM.
func BuildSandboxConfig(
	ctx context.Context,
	bundlePath string,
	opts *runhcsoptions.Options,
	spec *vm.Spec,
) (*hcsschema.ComputeSystem, *SandboxOptions, error) {
	log.G(ctx).Info("BuildSandboxConfig: starting sandbox spec generation")

	var err error

	if opts == nil {
		return nil, nil, fmt.Errorf("no options provided")
	}
	if spec.Annotations == nil {
		spec.Annotations = map[string]string{}
	}

	// Process annotations prior to parsing them into hcs spec.
	if err = processAnnotations(ctx, opts, spec.Annotations); err != nil {
		return nil, nil, fmt.Errorf("failed to process annotations: %w", err)
	}

	// Validate sandbox platform and architecture.
	platform := strings.ToLower(opts.SandboxPlatform)
	log.G(ctx).WithField("platform", platform).Debug("validating sandbox platform")
	if platform != "linux/amd64" && platform != "linux/arm64" {
		return nil, nil, fmt.Errorf("unsupported sandbox platform: %s", opts.SandboxPlatform)
	}

	// ================== Parse general sandbox options ==============================
	// ===============================================================================

	// Parse general sandbox options which are not used in the HCS document
	// but are used by the shim during its other workflows.
	sandboxOptions, err := parseSandboxOptions(ctx, platform, spec.Annotations)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse sandbox options: %w", err)
	}

	// ================== Parse Topology (CPU, Memory, NUMA) options =================
	// ===============================================================================

	// Parse CPU configuration.
	cpuConfig, err := parseCPUOptions(ctx, opts, spec.Annotations)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CPU parameters: %w", err)
	}

	// Parse memory configuration.
	memoryConfig, err := parseMemoryOptions(ctx, opts, spec.Annotations, sandboxOptions.FullyPhysicallyBacked)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse memory parameters: %w", err)
	}

	// Parse NUMA settings only for non-confidential VMs.
	var numa *hcsschema.Numa
	var numaProcessors *hcsschema.NumaProcessors
	if sandboxOptions.ConfidentialConfig == nil {
		numa, numaProcessors, err = parseNUMAOptions(
			ctx,
			spec.Annotations,
			cpuConfig.Count,
			memoryConfig.SizeInMB,
			memoryConfig.AllowOvercommit,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse NUMA parameters: %w", err)
		}

		// Set Numa processor settings.
		cpuConfig.NumaProcessorsSettings = numaProcessors

		if numa != nil || numaProcessors != nil {
			firmwareFallbackMeasured := hcsschema.VirtualSlitType_FIRMWARE_FALLBACK_MEASURED
			memoryConfig.SlitType = &firmwareFallbackMeasured
		}
	}

	// ================== Parse Storage QOS options ==================================
	// ===============================================================================

	storageQOSConfig, err := parseStorageQOSOptions(ctx, spec.Annotations)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse storage parameters: %w", err)
	}

	// ================== Parse Boot options =========================================
	// ===============================================================================

	// For confidential VMs, we don't use the standard boot options - the UEFI secure boot
	// settings will be set by parseConfidentialOptions.
	bootOptions := &hcsschema.Chipset{}
	var rootFsFullPath string
	if sandboxOptions.ConfidentialConfig == nil {
		bootOptions, rootFsFullPath, err = parseBootOptions(ctx, opts, spec.Annotations)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse boot options: %w", err)
		}
	}

	// ================== Parse Device (SCSI, VPMEM, VPCI) options ===================
	// ===============================================================================

	// This should be done after parsing boot options, as some device options may depend on boot settings (e.g., rootfs path).
	vpmemCtrl, scsiCtrl, vpciDevices, err := parseDeviceOptions(
		ctx,
		spec.Annotations,
		spec.Devices,
		rootFsFullPath,
		numa != nil && numaProcessors != nil,     // isNumaEnabled
		sandboxOptions.FullyPhysicallyBacked,     // isFullyPhysicallyBacked
		sandboxOptions.ConfidentialConfig != nil, // isConfidential
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse device options: %w", err)
	}

	// ================== Parse ComPort and HVSocket options =========================
	// ===============================================================================

	// Parse additional options and settings.
	hvSocketConfig, comPorts, err := setAdditionalOptions(
		ctx,
		spec.Annotations,
		sandboxOptions.ConfidentialConfig != nil, // isConfidential
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse additional parameters: %w", err)
	}

	// ================== Parse Confidential options =================================
	// ===============================================================================

	// For confidential VMs, parse confidential options which includes secure boot settings.
	var securitySettings *hcsschema.SecuritySettings
	var guestState *hcsschema.GuestState
	var filesToCleanOnError []string
	if sandboxOptions.ConfidentialConfig != nil {
		bootOptions,
			securitySettings,
			guestState,
			filesToCleanOnError,
			err =
			parseConfidentialOptions(
				ctx,
				bundlePath,
				opts,
				spec.Annotations,
				scsiCtrl,       // We need to augment SCSI controller settings for confidential VMs to include the rootfs.vhd as a protected disk
				hvSocketConfig, // We need to tighten the HvSocket security descriptor for confidential VMs.
				sandboxOptions.ConfidentialConfig.SecurityPolicy,
			)
		// Register cleanup method prior to checking for error.
		defer func() {
			for _, file := range filesToCleanOnError {
				_ = os.Remove(file)
			}
		}()

		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse confidential options: %w", err)
		}

		// Set memory to physical backing (no overcommit) for confidential VMs
		log.G(ctx).Debug("disabling memory overcommit for confidential VM")
		memoryConfig.AllowOvercommit = false
	}

	// ================== Parse and set Kernel Args ==================================
	// ===============================================================================

	// Build the kernel command line after all options are parsed.
	// For confidential VMs (SNP mode), kernel args are embedded in VMGS file, so skip this.
	var kernelArgs string
	if sandboxOptions.ConfidentialConfig == nil {
		kernelArgs, err = buildKernelArgs(
			ctx,
			opts,
			spec.Annotations,
			cpuConfig.Count,
			bootOptions.LinuxKernelDirect != nil, // isKernelDirectBoot
			vpmemCtrl != nil,                     // isVPMem
			comPorts != nil,                      // hasConsole
			filepath.Base(rootFsFullPath),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build kernel args: %w", err)
		}

		// Other boot options were already added earlier in parseBootOptions.
		// Set the kernel args here which are constructed based on all other options.
		if bootOptions.LinuxKernelDirect != nil {
			bootOptions.LinuxKernelDirect.KernelCmdLine = kernelArgs
		} else if bootOptions.Uefi != nil && bootOptions.Uefi.BootThis != nil {
			bootOptions.Uefi.BootThis.OptionalData = kernelArgs
		}
		log.G(ctx).WithField("kernelArgs", kernelArgs).Debug("kernel arguments configured")
	}

	// ================== Create the final HCS document ==============================
	// ===============================================================================

	// Finally, build the HCS document with all the parsed and processed options.
	log.G(ctx).Debug("assembling final sandbox hcs spec")
	// Use Schema V21 for non-confidential cases.
	// Use Schema V25 for confidential cases.
	schema := schemaversion.SchemaV21()
	if sandboxOptions.ConfidentialConfig != nil {
		schema = schemaversion.SchemaV25()
	}

	// Build the document.
	doc := &hcsschema.ComputeSystem{
		Owner:         shimName,
		SchemaVersion: schema,
		// Terminate the UVM when the last handle is closed.
		// To support impactless updates this will need to be configurable.
		ShouldTerminateOnLastHandleClosed: true,
		VirtualMachine: &hcsschema.VirtualMachine{
			StopOnReset: true,
			Chipset:     bootOptions,
			ComputeTopology: &hcsschema.Topology{
				Memory:    memoryConfig,
				Processor: cpuConfig,
				Numa:      numa,
			},
			StorageQoS: storageQOSConfig,
			Devices: &hcsschema.Devices{
				Scsi:        scsiCtrl,
				VirtualPMem: vpmemCtrl,
				VirtualPci:  vpciDevices,
				HvSocket: &hcsschema.HvSocket2{
					HvSocketConfig: hvSocketConfig,
				},
				ComPorts: comPorts,
				Plan9:    &hcsschema.Plan9{},
			},
			GuestState:       guestState,
			SecuritySettings: securitySettings,
		},
	}

	log.G(ctx).Info("sandbox spec generation completed successfully")

	return doc, sandboxOptions, nil
}

// processAnnotations applies defaults and normalizes annotation values.
func processAnnotations(ctx context.Context, opts *runhcsoptions.Options, annotations map[string]string) error {
	log.G(ctx).WithField("annotationCount", len(annotations)).Debug("processAnnotations: starting annotations processing")

	// Apply default annotations.
	for key, value := range opts.DefaultContainerAnnotations {
		// Only set default if not already set in annotations
		if _, exists := annotations[key]; !exists {
			annotations[key] = value
		}
	}

	err := oci.ProcessAnnotations(ctx, annotations)
	if err != nil {
		return fmt.Errorf("failed to process OCI annotations: %w", err)
	}

	// Check for explicitly unsupported annotations.
	ncProxy := oci.ParseAnnotationsString(annotations, shimannotations.NetworkConfigProxy, "")
	if ncProxy != "" {
		return fmt.Errorf("%s annotation is not supported", shimannotations.NetworkConfigProxy)
	}

	log.G(ctx).Debug("processAnnotations completed successfully")
	return nil
}

// parseSandboxOptions parses general sandbox options from annotations and options.
// These options are not used in the HCS document but are used by the shim during its other workflows.
func parseSandboxOptions(ctx context.Context, platform string, annotations map[string]string) (*SandboxOptions, error) {
	// This is additional configuration that is not part of the HCS document, but it used by the shim.
	log.G(ctx).WithField("platform", platform).Debug("parseSandboxOptions: starting sandbox options parsing")
	sandboxOptions := &SandboxOptions{
		// Extract architecture from platform string (e.g., "linux/amd64" -> "amd64")
		Architecture:          platform[strings.IndexByte(platform, '/')+1:],
		FullyPhysicallyBacked: oci.ParseAnnotationsBool(ctx, annotations, shimannotations.FullyPhysicallyBacked, false),
		PolicyBasedRouting:    oci.ParseAnnotationsBool(ctx, annotations, iannotations.NetworkingPolicyBasedRouting, false),
		NoWritableFileShares:  oci.ParseAnnotationsBool(ctx, annotations, shimannotations.DisableWritableFileShares, false),
		// Multi-mapping is enabled by default on 19H1+, can be disabled via annotation.
		VPMEMMultiMapping: !(oci.ParseAnnotationsBool(ctx, annotations, shimannotations.VPMemNoMultiMapping, osversion.Build() < osversion.V19H1)),
	}

	// Parse the list of additional kernel drivers to be injected into the VM.
	drivers := oci.ParseAnnotationCommaSeparated(shimannotations.VirtualMachineKernelDrivers, annotations)
	for _, driver := range drivers {
		if _, err := os.Stat(driver); err != nil {
			return nil, fmt.Errorf("failed to find path to drivers at %s: %w", driver, err)
		}
	}
	sandboxOptions.GuestDrivers = drivers

	// Determine if this is a confidential VM early, as it affects boot options parsing
	securityPolicy := oci.ParseAnnotationsString(annotations, shimannotations.LCOWSecurityPolicy, "")
	noSecurityHardware := oci.ParseAnnotationsBool(ctx, annotations, shimannotations.NoSecurityHardware, false)
	if securityPolicy != "" && !noSecurityHardware {
		sandboxOptions.ConfidentialConfig = &ConfidentialConfig{
			SecurityPolicy:         securityPolicy,
			SecurityPolicyEnforcer: oci.ParseAnnotationsString(annotations, shimannotations.LCOWSecurityPolicyEnforcer, ""),
			UvmReferenceInfoFile:   oci.ParseAnnotationsString(annotations, shimannotations.LCOWReferenceInfoFile, vmutils.DefaultUVMReferenceInfoFile),
		}

		log.G(ctx).WithFields(logrus.Fields{
			"securityPolicy":     securityPolicy,
			"noSecurityHardware": noSecurityHardware,
		}).Debug("determined confidential VM mode")
	}

	// Default for enable_scratch_encryption is false for non-confidential VMs,
	// true for confidential VMs. Can be overridden by annotation.
	sandboxOptions.EnableScratchEncryption = oci.ParseAnnotationsBool(ctx, annotations, shimannotations.LCOWEncryptedScratchDisk, sandboxOptions.ConfidentialConfig != nil)

	log.G(ctx).Debug("parseSandboxOptions completed successfully")
	return sandboxOptions, nil
}

// parseStorageQOSOptions parses storage QOS options from annotations.
func parseStorageQOSOptions(ctx context.Context, annotations map[string]string) (*hcsschema.StorageQoS, error) {
	log.G(ctx).Debug("parseStorageQOSOptions: starting storage QOS options parsing")

	storageQOSConfig := &hcsschema.StorageQoS{
		IopsMaximum:      oci.ParseAnnotationsInt32(ctx, annotations, shimannotations.StorageQoSIopsMaximum, 0),
		BandwidthMaximum: oci.ParseAnnotationsInt32(ctx, annotations, shimannotations.StorageQoSBandwidthMaximum, 0),
	}

	log.G(ctx).WithFields(logrus.Fields{
		"qosBandwidthMax": storageQOSConfig.BandwidthMaximum,
		"qosIopsMax":      storageQOSConfig.IopsMaximum,
	}).Debug("parseStorageQOSOptions completed successfully")

	return storageQOSConfig, nil
}

// setAdditionalOptions sets additional options from annotations.
func setAdditionalOptions(ctx context.Context, annotations map[string]string, isConfidential bool) (*hcsschema.HvSocketSystemConfig, map[string]hcsschema.ComPort, error) {
	log.G(ctx).Debug("setAdditionalOptions: starting additional options parsing")

	hvSocketConfig := &hcsschema.HvSocketSystemConfig{
		// Allow administrators and SYSTEM to bind to vsock sockets
		// so that we can create a GCS log socket.
		// We will change these in Confidential cases.
		DefaultBindSecurityDescriptor: "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
		ServiceTable:                  make(map[string]hcsschema.HvSocketServiceConfig),
	}

	hvSocketServiceTable := oci.ParseHVSocketServiceTable(ctx, annotations)
	maps.Copy(hvSocketConfig.ServiceTable, hvSocketServiceTable)

	// Console pipe is only supported for non-confidential VMs.
	var comPorts map[string]hcsschema.ComPort
	if !isConfidential {
		consolePipe := oci.ParseAnnotationsString(annotations, iannotations.UVMConsolePipe, "")
		if consolePipe != "" {
			if !strings.HasPrefix(consolePipe, `\\.\pipe\`) {
				return nil, nil, fmt.Errorf("listener for serial console is not a named pipe")
			}

			comPorts = map[string]hcsschema.ComPort{
				"0": { // COM1
					NamedPipe: consolePipe,
				},
			}
		}
	}

	log.G(ctx).Debug("setAdditionalOptions completed successfully")

	return hvSocketConfig, comPorts, nil
}
