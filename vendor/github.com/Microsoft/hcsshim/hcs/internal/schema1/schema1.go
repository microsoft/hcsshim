//go:build windows

package schema1

import (
	"time"

	"github.com/Microsoft/go-winio/pkg/guid"
	hcsschema "github.com/Microsoft/hcsshim/hcs/internal/schema2"
)

// ContainerProperties holds the properties for a container and the processes running in that container
type ContainerProperties struct {
	ID                           string `json:"Id"`
	State                        string
	Name                         string
	SystemType                   string
	RuntimeOSType                string `json:"RuntimeOsType,omitempty"`
	Owner                        string
	SiloGUID                     string                              `json:"SiloGuid,omitempty"`
	RuntimeID                    guid.GUID                           `json:"RuntimeId,omitempty"`
	IsRuntimeTemplate            bool                                `json:",omitempty"`
	RuntimeImagePath             string                              `json:",omitempty"`
	Stopped                      bool                                `json:",omitempty"`
	ExitType                     string                              `json:",omitempty"`
	AreUpdatesPending            bool                                `json:",omitempty"`
	ObRoot                       string                              `json:",omitempty"`
	Statistics                   Statistics                          `json:",omitempty"`
	ProcessList                  []ProcessListItem                   `json:",omitempty"`
	MappedVirtualDiskControllers map[int]MappedVirtualDiskController `json:",omitempty"`
	GuestConnectionInfo          GuestConnectionInfo                 `json:",omitempty"`
}

// MemoryStats holds the memory statistics for a container
type MemoryStats struct {
	UsageCommitBytes            uint64 `json:"MemoryUsageCommitBytes,omitempty"`
	UsageCommitPeakBytes        uint64 `json:"MemoryUsageCommitPeakBytes,omitempty"`
	UsagePrivateWorkingSetBytes uint64 `json:"MemoryUsagePrivateWorkingSetBytes,omitempty"`
}

// ProcessorStats holds the processor statistics for a container
type ProcessorStats struct {
	TotalRuntime100ns  uint64 `json:",omitempty"`
	RuntimeUser100ns   uint64 `json:",omitempty"`
	RuntimeKernel100ns uint64 `json:",omitempty"`
}

// StorageStats holds the storage statistics for a container
type StorageStats struct {
	ReadCountNormalized  uint64 `json:",omitempty"`
	ReadSizeBytes        uint64 `json:",omitempty"`
	WriteCountNormalized uint64 `json:",omitempty"`
	WriteSizeBytes       uint64 `json:",omitempty"`
}

// NetworkStats holds the network statistics for a container
type NetworkStats struct {
	BytesReceived          uint64 `json:",omitempty"`
	BytesSent              uint64 `json:",omitempty"`
	PacketsReceived        uint64 `json:",omitempty"`
	PacketsSent            uint64 `json:",omitempty"`
	DroppedPacketsIncoming uint64 `json:",omitempty"`
	DroppedPacketsOutgoing uint64 `json:",omitempty"`
	EndpointId             string `json:",omitempty"`
	InstanceId             string `json:",omitempty"`
}

// Statistics is the structure returned by a statistics call on a container
type Statistics struct {
	Timestamp          time.Time      `json:",omitempty"`
	ContainerStartTime time.Time      `json:",omitempty"`
	Uptime100ns        uint64         `json:",omitempty"`
	Memory             MemoryStats    `json:",omitempty"`
	Processor          ProcessorStats `json:",omitempty"`
	Storage            StorageStats   `json:",omitempty"`
	Network            []NetworkStats `json:",omitempty"`
}

// ProcessList is the structure of an item returned by a ProcessList call on a container
type ProcessListItem struct {
	CreateTimestamp              time.Time `json:",omitempty"`
	ImageName                    string    `json:",omitempty"`
	KernelTime100ns              uint64    `json:",omitempty"`
	MemoryCommitBytes            uint64    `json:",omitempty"`
	MemoryWorkingSetPrivateBytes uint64    `json:",omitempty"`
	MemoryWorkingSetSharedBytes  uint64    `json:",omitempty"`
	ProcessId                    uint32    `json:",omitempty"`
	UserTime100ns                uint64    `json:",omitempty"`
}

// MappedVirtualDiskController is the structure of an item returned by a MappedVirtualDiskList call on a container
type MappedVirtualDiskController struct {
	MappedVirtualDisks map[int]MappedVirtualDisk `json:",omitempty"`
}

type MappedVirtualDisk struct {
	HostPath          string `json:",omitempty"` // Path to VHD on the host
	ContainerPath     string // Platform-specific mount point path in the container
	CreateInUtilityVM bool   `json:",omitempty"`
	ReadOnly          bool   `json:",omitempty"`
	Cache             string `json:",omitempty"` // "" (Unspecified); "Disabled"; "Enabled"; "Private"; "PrivateAllowSharing"
	AttachOnly        bool   `json:",omitempty"`
}

// GuestDefinedCapabilities is part of the GuestConnectionInfo returned by a GuestConnection call on a utility VM
type GuestDefinedCapabilities struct {
	NamespaceAddRequestSupported  bool `json:",omitempty"`
	SignalProcessSupported        bool `json:",omitempty"`
	DumpStacksSupported           bool `json:",omitempty"`
	DeleteContainerStateSupported bool `json:",omitempty"`
	UpdateContainerSupported      bool `json:",omitempty"`
}

// GuestConnectionInfo is the structure of an iterm return by a GuestConnection call on a utility VM
type GuestConnectionInfo struct {
	SupportedSchemaVersions  []hcsschema.Version      `json:",omitempty"`
	ProtocolVersion          uint32                   `json:",omitempty"`
	GuestDefinedCapabilities GuestDefinedCapabilities `json:",omitempty"`
}

// Type of Request Support in ModifySystem
type RequestType string

// Type of Resource Support in ModifySystem
type ResourceType string

// ResourceModificationRequestResponse is the structure used to send request to the container to modify the system
// Supported resource types are Network and Request Types are Add/Remove
type ResourceModificationRequestResponse struct {
	Resource ResourceType `json:"ResourceType"`
	Data     interface{}  `json:"Settings"`
	Request  RequestType  `json:"RequestType,omitempty"`
}
