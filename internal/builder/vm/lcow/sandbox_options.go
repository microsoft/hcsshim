//go:build windows

package lcow

// SandboxOptions carries configuration fields that are needed by the shim
// but do not have a direct representation in the HCS ComputeSystem document.
// These fields are consumed by downstream code (e.g., container creation,
// layer management) after the UVM is created.
type SandboxOptions struct {
	// NoWritableFileShares disallows writable file shares to the UVM.
	NoWritableFileShares bool

	// EnableScratchEncryption enables encryption for scratch disks.
	EnableScratchEncryption bool

	// GuestDrivers lists guest drivers which need to be installed on the UVM.
	GuestDrivers []string

	// PolicyBasedRouting enables policy-based routing in the guest network stack.
	PolicyBasedRouting bool

	// Architecture is the processor architecture (e.g., "amd64", "arm64").
	Architecture string

	// FullyPhysicallyBacked indicates all memory allocations are backed by physical memory.
	FullyPhysicallyBacked bool

	// VPMEMMultiMapping indicates whether VPMem multi-mapping is enabled,
	// which allows multiple VHDs to be mapped to a single VPMem device.
	VPMEMMultiMapping bool

	// ConfidentialConfig carries confidential computing fields that are not
	// part of the HCS document but are needed for confidential VM setup.
	ConfidentialConfig *ConfidentialConfig
}

// ConfidentialConfig carries confidential computing configuration that is not
// part of the HCS ComputeSystem document but is needed during confidential VM setup.
type ConfidentialConfig struct {
	// SecurityPolicy is the security policy enforced inside the guest environment.
	SecurityPolicy string

	// SecurityPolicyEnforcer is the security policy enforcer type.
	SecurityPolicyEnforcer string

	// UvmReferenceInfoFile is the path to the signed UVM reference info file for attestation.
	UvmReferenceInfoFile string
}
