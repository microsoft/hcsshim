// Package lcow encapsulates the business logic to parse annotations, devices,
// and runhcs options into an hcsschema.ComputeSystem document which will be used
// by the shim to create UVMs (Utility VMs) via the Host Compute Service (HCS).
//
// The primary entry point is [BuildSandboxConfig], which takes an owner string,
// containerd runtime options, OCI annotations, and device assignments, and produces
// an hcsschema.ComputeSystem document along with a [SandboxOptions] struct that
// carries sidecar configuration not representable in the HCS document (e.g.,
// security policy, guest drivers, scratch encryption settings).
//
// # Sandbox Specification Components
//
// The package handles parsing and validation of multiple configuration areas:
//
//   - Boot Configuration: Kernel, initrd, root filesystem, and boot file paths
//   - CPU Configuration: Processor count, limits, and NUMA topology
//   - Memory Configuration: Memory size, MMIO gaps, and memory affinity
//   - Device Configuration: VPMem devices, vPCI devices, and SCSI controllers
//   - Storage Configuration: Storage QoS settings
//   - Confidential Computing: Security policies, SNP settings, and encryption
//   - Kernel Arguments: Command line parameters derived from all configuration sources
//
// # Annotation Support
//
// The package extensively uses OCI annotations to allow fine-grained control over
// UVM creation. Annotations can override default behaviors or provide additional
// configuration not available through standard containerd options.
//
// # Platform Support
//
// The package supports both AMD64 and ARM64 Linux platforms running on Windows
// hosts via the Host Compute Service.
package lcow
