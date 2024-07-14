// Autogenerated code; DO NOT EDIT.

/*
 * Schema Open API
 *
 * No description provided (generated by Swagger Codegen https://github.com/swagger-api/swagger-codegen)
 *
 * API version: 2.4
 * Generated by: Swagger Codegen (https://github.com/swagger-api/swagger-codegen.git)
 */

package hcsschema

type NumaSetting struct {
	VirtualNodeNumber        uint32            `json:"VirtualNodeNumber,omitempty"`
	PhysicalNodeNumber       uint32            `json:"PhysicalNodeNumber,omitempty"`
	VirtualSocketNumber      uint32            `json:"VirtualSocketNumber,omitempty"`
	CountOfProcessors        uint32            `json:"CountOfProcessors,omitempty"`
	CountOfMemoryBlocks      uint64            `json:"CountOfMemoryBlocks,omitempty"`
	MemoryBackingType        MemoryBackingType `json:"MemoryBackingType,omitempty"`
	AccessTracingGranularity PageGranularity   `json:"AccessTracingGranularity,omitempty"`
}
