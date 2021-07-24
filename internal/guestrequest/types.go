package guestrequest

import (
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// Arguably, many of these (at least CombinedLayers) should have been generated
// by swagger.
//
// This will also change package name due to an inbound breaking change.

// This class is used by a modify request to add or remove a combined layers
// structure in the guest. For windows, the GCS applies a filter in ContainerRootPath
// using the specified layers as the parent content. Ignores property ScratchPath
// since the container path is already the scratch path. For linux, the GCS unions
// the specified layers and ScratchPath together, placing the resulting union
// filesystem at ContainerRootPath.
type CombinedLayers struct {
	ContainerRootPath string            `json:"ContainerRootPath,omitempty"`
	Layers            []hcsschema.Layer `json:"Layers,omitempty"`
	ScratchPath       string            `json:"ScratchPath,omitempty"`
}

// Defines the schema for hosted settings passed to GCS and/or OpenGCS

// SCSI. Scratch space for remote file-system commands, or R/W layer for containers
type LCOWMappedVirtualDisk struct {
	MountPath  string   `json:"MountPath,omitempty"`
	Lun        uint8    `json:"Lun,omitempty"`
	Controller uint8    `json:"Controller,omitempty"`
	ReadOnly   bool     `json:"ReadOnly,omitempty"`
	Options    []string `json:"Options,omitempty"`
}

type WCOWMappedVirtualDisk struct {
	ContainerPath string `json:"ContainerPath,omitempty"`
	Lun           int32  `json:"Lun,omitempty"`
}

type LCOWMappedDirectory struct {
	MountPath string `json:"MountPath,omitempty"`
	Port      int32  `json:"Port,omitempty"`
	ShareName string `json:"ShareName,omitempty"` // If empty not using ANames (not currently supported)
	ReadOnly  bool   `json:"ReadOnly,omitempty"`
}

// LCOWMappedLayer is one of potentially multiple read-only layers mapped on a VPMem device
type LCOWMappedLayer struct {
	DeviceOffsetInBytes uint64 `json:"DeviceOffsetInBytes,omitempty"`
	DeviceSizeInBytes   uint64 `json:"DeviceSizeInBytes,omitempty"`
}

// DeviceVerityInfo represents dm-verity metadata of a block device.
// Most of the fields can be directly mapped to table entries https://www.kernel.org/doc/html/latest/admin-guide/device-mapper/verity.html
type DeviceVerityInfo struct {
	// Ext4SizeInBytes is the size of ext4 file system
	Ext4SizeInBytes int64 `json:",omitempty"`
	// Version is the on-disk hash format
	Version int `json:",omitempty"`
	// Algorithm is the algo used to produce the hashes for dm-verity hash tree
	Algorithm string `json:",omitempty"`
	// SuperBlock is set to true if dm-verity super block is present on the device
	SuperBlock bool `json:",omitempty"`
	// RootDigest is the root hash of the dm-verity hash tree
	RootDigest string `json:",omitempty"`
	// Salt is the salt used to compute the root hash
	Salt string `json:",omitempty"`
	// BlockSize is the data device block size
	BlockSize int `json:",omitempty"`
}

// Read-only layers over VPMem
type LCOWMappedVPMemDevice struct {
	DeviceNumber uint32            `json:"DeviceNumber,omitempty"`
	MountPath    string            `json:"MountPath,omitempty"`
	MappingInfo  *LCOWMappedLayer  `json:"MappingInfo,omitempty"`
	VerityInfo   *DeviceVerityInfo `json:"VerityInfo,omitempty"`
}

type LCOWMappedVPCIDevice struct {
	VMBusGUID string `json:"VMBusGUID,omitempty"`
}

type LCOWNetworkAdapter struct {
	NamespaceID     string `json:",omitempty"`
	ID              string `json:",omitempty"`
	MacAddress      string `json:",omitempty"`
	IPAddress       string `json:",omitempty"`
	PrefixLength    uint8  `json:",omitempty"`
	GatewayAddress  string `json:",omitempty"`
	DNSSuffix       string `json:",omitempty"`
	DNSServerList   string `json:",omitempty"`
	EnableLowMetric bool   `json:",omitempty"`
	EncapOverhead   uint16 `json:",omitempty"`
	IsVPCI          bool   `json:",omitempty"`
}

type LCOWContainerConstraints struct {
	Windows specs.WindowsResources `json:",omitempty"`
	Linux   specs.LinuxResources   `json:",omitempty"`
}

type ResourceType string

const (
	// These are constants for v2 schema modify guest requests.
	ResourceTypeMappedDirectory      ResourceType = "MappedDirectory"
	ResourceTypeMappedVirtualDisk    ResourceType = "MappedVirtualDisk"
	ResourceTypeNetwork              ResourceType = "Network"
	ResourceTypeNetworkNamespace     ResourceType = "NetworkNamespace"
	ResourceTypeCombinedLayers       ResourceType = "CombinedLayers"
	ResourceTypeVPMemDevice          ResourceType = "VPMemDevice"
	ResourceTypeVPCIDevice           ResourceType = "VPCIDevice"
	ResourceTypeContainerConstraints ResourceType = "ContainerConstraints"
	ResourceTypeHvSocket             ResourceType = "HvSocket"
)

// GuestRequest is for modify commands passed to the guest.
type GuestRequest struct {
	RequestType  string       `json:"RequestType,omitempty"`
	ResourceType ResourceType `json:"ResourceType,omitempty"`
	Settings     interface{}  `json:"Settings,omitempty"`
}

type NetworkModifyRequest struct {
	AdapterId   string      `json:"AdapterId,omitempty"`
	RequestType string      `json:"RequestType,omitempty"`
	Settings    interface{} `json:"Settings,omitempty"`
}

type RS4NetworkModifyRequest struct {
	AdapterInstanceId string      `json:"AdapterInstanceId,omitempty"`
	RequestType       string      `json:"RequestType,omitempty"`
	Settings          interface{} `json:"Settings,omitempty"`
}

// SignalProcessOptionsLCOW is the options passed to LCOW to signal a given
// process.
type SignalProcessOptionsLCOW struct {
	Signal int `json:",omitempty"`
}

type SignalValueWCOW string

const (
	SignalValueWCOWCtrlC        SignalValueWCOW = "CtrlC"
	SignalValueWCOWCtrlBreak    SignalValueWCOW = "CtrlBreak"
	SignalValueWCOWCtrlClose    SignalValueWCOW = "CtrlClose"
	SignalValueWCOWCtrlLogOff   SignalValueWCOW = "CtrlLogOff"
	SignalValueWCOWCtrlShutdown SignalValueWCOW = "CtrlShutdown"
)

// SignalProcessOptionsWCOW is the options passed to WCOW to signal a given
// process.
type SignalProcessOptionsWCOW struct {
	Signal SignalValueWCOW `json:",omitempty"`
}
