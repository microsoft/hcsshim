// Autogenerated code; DO NOT EDIT.

// Schema retrieved from branch 'fe_release' and build '20348.1.210507-1500'.

/*
 * Schema Open API
 *
 * No description provided (generated by Swagger Codegen https://github.com/swagger-api/swagger-codegen)
 *
 * API version: 2.4
 * Generated by: Swagger Codegen (https://github.com/swagger-api/swagger-codegen.git)
 */

package hcsschema

type VirtualMachineMemory struct {
	SizeInMB uint64             `json:"SizeInMB,omitempty"`
	Backing  *MemoryBackingType `json:"Backing,omitempty"`
	// If enabled, then the VM's memory is backed by the Windows pagefile rather than physically backed, statically allocated memory.
	AllowOvercommit bool                   `json:"AllowOvercommit,omitempty"`
	BackingPageSize *MemoryBackingPageSize `json:"BackingPageSize,omitempty"`
	// Fault clustering size for primary RAM.
	FaultClusterSizeShift uint32 `json:"FaultClusterSizeShift,omitempty"`
	// Fault clustering size for direct mapped memory.
	DirectMapFaultClusterSizeShift uint32 `json:"DirectMapFaultClusterSizeShift,omitempty"`
	// If enabled, then each backing page is physically pinned on first access.
	PinBackingPages bool `json:"PinBackingPages,omitempty"`
	// If enabled, then backing page chunks smaller than the backing page size are never used unless the system is under extreme memory pressure. If the backing page size is Small, then it is forced to Large when this option is enabled.
	ForbidSmallBackingPages       bool `json:"ForbidSmallBackingPages,omitempty"`
	EnablePrivateCompressionStore bool `json:"EnablePrivateCompressionStore,omitempty"`
	// If enabled, then the memory hot hint feature is exposed to the VM, allowing it to prefetch pages into its working set. (if supported by the guest operating system).
	EnableHotHint bool `json:"EnableHotHint,omitempty"`
	// If enabled, then the memory cold hint feature is exposed to the VM, allowing it to trim zeroed pages from its working set (if supported by the guest operating system).
	EnableColdHint bool `json:"EnableColdHint,omitempty"`
	// If enabled, then the memory cold discard hint feature is exposed to the VM, allowing it to trim non-zeroed pages from the working set (if supported by the guest operating system).
	EnableColdDiscardHint bool `json:"EnableColdDiscardHint,omitempty"`
	// If enabled, then the base address of direct-mapped host images is exposed to the guest.
	ImageBaseAddressesExposed  bool     `json:"ImageBaseAddressesExposed,omitempty"`
	SharedMemoryMB             int64    `json:"SharedMemoryMB,omitempty"`
	DisableSharedMemoryMapping bool     `json:"DisableSharedMemoryMapping,omitempty"`
	SharedMemoryAccessSids     []string `json:"SharedMemoryAccessSids,omitempty"`
	EnableEpf                  bool     `json:"EnableEpf,omitempty"`
	// If enabled, then commit is not charged for each backing page until first access.
	EnableDeferredCommit bool `json:"EnableDeferredCommit,omitempty"`
	// Low MMIO region allocated below 4GB
	LowMMIOGapInMB uint64 `json:"LowMmioGapInMB,omitempty"`
	// High MMIO region allocated above 4GB (base and size)
	HighMMIOBaseInMB uint64     `json:"HighMmioBaseInMB,omitempty"`
	HighMMIOGapInMB  uint64     `json:"HighMmioGapInMB,omitempty"`
	SgxMemory        *SgxMemory `json:"SgxMemory,omitempty"`
}
