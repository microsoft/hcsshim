package guestresource

import (
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/go-winio/pkg/guid"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// Arguably, many of these (at least CombinedLayers) should have been generated
// by swagger.
//
// This will also change package name due to an inbound breaking change.

const (
	// These are constants for v2 schema modify guest requests.

	// ResourceTypeMappedDirectory is the modify resource type for mapped
	// directories
	ResourceTypeMappedDirectory guestrequest.ResourceType = "MappedDirectory"
	// ResourceTypeSCSIDevice is the modify resources type for SCSI devices.
	// Note this type is not related to mounting a device in the guest, only
	// for operations on the SCSI device itself.
	// Currently it only supports Remove, to cleanly remove a SCSI device.
	ResourceTypeSCSIDevice guestrequest.ResourceType = "SCSIDevice"
	// ResourceTypeMappedVirtualDisk is the modify resource type for mapped
	// virtual disks
	ResourceTypeMappedVirtualDisk guestrequest.ResourceType = "MappedVirtualDisk"
	// ResourceTypeMappedVirtualDiskForContainerScratch is the modify resource type
	// specifically for refs formatting and mounting scratch vhds for c-wcow cases only.
	ResourceTypeMappedVirtualDiskForContainerScratch guestrequest.ResourceType = "MappedVirtualDiskForContainerScratch"
	ResourceTypeWCOWBlockCims                        guestrequest.ResourceType = "WCOWBlockCims"
	// ResourceTypeNetwork is the modify resource type for the `NetworkAdapterV2`
	// device.
	ResourceTypeNetwork          guestrequest.ResourceType = "Network"
	ResourceTypeNetworkNamespace guestrequest.ResourceType = "NetworkNamespace"
	// ResourceTypeCombinedLayers is the modify resource type for combined
	// layers
	ResourceTypeCombinedLayers guestrequest.ResourceType = "CombinedLayers"
	// ResourceTypeCWCOWCombinedLayers is the modify resource type for combined
	// layers call for cwcow cases. This resource type wraps containerID around
	// ResourceTypeCombinedLayers.
	ResourceTypeCWCOWCombinedLayers guestrequest.ResourceType = "CWCOWCombinedLayers"
	// ResourceTypeVPMemDevice is the modify resource type for VPMem devices
	ResourceTypeVPMemDevice guestrequest.ResourceType = "VPMemDevice"
	// ResourceTypeVPCIDevice is the modify resource type for vpci devices
	ResourceTypeVPCIDevice guestrequest.ResourceType = "VPCIDevice"
	// ResourceTypeContainerConstraints is the modify resource type for updating
	// container constraints
	ResourceTypeContainerConstraints guestrequest.ResourceType = "ContainerConstraints"
	ResourceTypeHvSocket             guestrequest.ResourceType = "HvSocket"
	// ResourceTypeSecurityPolicy is the modify resource type for updating the security
	// policy
	ResourceTypeSecurityPolicy guestrequest.ResourceType = "SecurityPolicy"
	// ResourceTypePolicyFragment is the modify resource type for injecting policy fragments.
	ResourceTypePolicyFragment guestrequest.ResourceType = "SecurityPolicyFragment"
)

// This class is used by a modify request to add or remove a combined layers
// structure in the guest. For windows, the GCS applies a filter in ContainerRootPath
// using the specified layers as the parent content. Ignores property ScratchPath
// since the container path is already the scratch path. For linux, the GCS unions
// the specified layers and ScratchPath together, placing the resulting union
// filesystem at ContainerRootPath.
type LCOWCombinedLayers struct {
	ContainerID       string            `json:",omitempty"`
	ContainerRootPath string            `json:",omitempty"`
	Layers            []hcsschema.Layer `json:",omitempty"`
	ScratchPath       string            `json:",omitempty"`
}

type WCOWCombinedLayers struct {
	ContainerRootPath string                         `json:"ContainerRootPath,omitempty"`
	Layers            []hcsschema.Layer              `json:"Layers,omitempty"`
	ScratchPath       string                         `json:"ScratchPath,omitempty"`
	FilterType        hcsschema.FileSystemFilterType `json:"FilterType,omitempty"`
}

type CWCOWCombinedLayers struct {
	ContainerID    string             `json:"ContainerID,omitempty"`
	CombinedLayers WCOWCombinedLayers `json:"CombinedLayers,omitempty"`
}

// Defines the schema for hosted settings passed to GCS and/or OpenGCS

// SCSIDevice represents a SCSI device that is attached to the system.
type SCSIDevice struct {
	Controller uint8 `json:"Controller,omitempty"`
	Lun        uint8 `json:"Lun,omitempty"`
}

// LCOWMappedVirtualDisk represents a disk on the host which is mapped into a
// directory in the guest in the V2 schema.
type LCOWMappedVirtualDisk struct {
	MountPath  string   `json:"MountPath,omitempty"`
	Lun        uint8    `json:"Lun,omitempty"`
	Controller uint8    `json:"Controller,omitempty"`
	Partition  uint64   `json:"Partition,omitempty"`
	ReadOnly   bool     `json:"ReadOnly,omitempty"`
	Encrypted  bool     `json:"Encrypted,omitempty"`
	Options    []string `json:"Options,omitempty"`
	BlockDev   bool     `json:"BlockDev,omitempty"`
	// Deprecated: verity info is read by the guest
	VerityInfo       *DeviceVerityInfo `json:"VerityInfo,omitempty"`
	EnsureFilesystem bool              `json:"EnsureFilesystem,omitempty"`
	Filesystem       string            `json:"Filesystem,omitempty"`
}

type BlockCIMDevice struct {
	CimName string
	Lun     int32
}

type WCOWBlockCIMMounts struct {
	// BlockCIMs should be ordered from merged CIM followed by Layer n .. layer 1
	BlockCIMs  []BlockCIMDevice `json:"BlockCIMs,omitempty"`
	VolumeGUID guid.GUID        `json:"VolumeGUID,omitempty"`
	MountFlags uint32           `json:"MountFlags,omitempty"`
}

type WCOWMappedVirtualDisk struct {
	ContainerPath string `json:"ContainerPath,omitempty"`
	Lun           int32  `json:"Lun,omitempty"`
}

// LCOWMappedDirectory represents a directory on the host which is mapped to a
// directory on the guest through Plan9 in the V2 schema.
type LCOWMappedDirectory struct {
	MountPath string `json:"MountPath,omitempty"`
	Port      int32  `json:"Port,omitempty"`
	ShareName string `json:"ShareName,omitempty"` // If empty not using ANames (not currently supported)
	ReadOnly  bool   `json:"ReadOnly,omitempty"`
}

// LCOWVPMemMappingInfo is one of potentially multiple read-only layers mapped on a VPMem device
type LCOWVPMemMappingInfo struct {
	DeviceOffsetInBytes uint64 `json:"DeviceOffsetInBytes,omitempty"`
	DeviceSizeInBytes   uint64 `json:"DeviceSizeInBytes,omitempty"`
}

// DeviceVerityInfo represents dm-verity metadata of a block device.
// Most of the fields can be directly mapped to table entries https://www.kernel.org/doc/html/latest/admin-guide/device-mapper/verity.html
// Deprecated: verity info is now read inside the guest and this message will be
// removed.
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
	DeviceNumber uint32 `json:"DeviceNumber,omitempty"`
	MountPath    string `json:"MountPath,omitempty"`
	// MappingInfo is used when multiple devices are mapped onto a single VPMem device
	MappingInfo *LCOWVPMemMappingInfo `json:"MappingInfo,omitempty"`
	// VerityInfo is used when the VPMem has read-only integrity protection enabled
	// Deprecated: verity info is now read inside the guest.
	VerityInfo *DeviceVerityInfo `json:"VerityInfo,omitempty"`
}

type LCOWMappedVPCIDevice struct {
	VMBusGUID string `json:"VMBusGUID,omitempty"`
}

// LCOWNetworkAdapter represents a network interface and its associated
// configuration in a namespace.
type LCOWNetworkAdapter struct {
	NamespaceID   string         `json:",omitempty"`
	ID            string         `json:",omitempty"`
	MacAddress    string         `json:",omitempty"`
	DNSSuffix     string         `json:",omitempty"`
	DNSServerList string         `json:",omitempty"`
	EncapOverhead uint16         `json:",omitempty"`
	VPCIAssigned  bool           `json:",omitempty"`
	IPConfigs     []LCOWIPConfig `json:",omitempty"`
	Routes        []LCOWRoute    `json:",omitempty"`
	// PolicyBasedRouting determines if we should use old policy based routing in the
	// guest when configuring the endpoint.
	PolicyBasedRouting bool `json:",omitempty"`
	// EnableLowMetric is ONLY used by the guest when PolicyBasedRouting is set to
	// indicate which endpoints should be added with a low metric (higher number).
	EnableLowMetric bool `json:",omitempty"`
}

type LCOWIPConfig struct {
	IPAddress    string `json:",omitempty"`
	PrefixLength uint8  `json:",omitempty"`
}

type LCOWRoute struct {
	NextHop           string `json:",omitempty"`
	DestinationPrefix string `json:",omitempty"`
	Metric            uint16 `json:",omitempty"`
}

type LCOWContainerConstraints struct {
	Windows specs.WindowsResources `json:",omitempty"`
	Linux   specs.LinuxResources   `json:",omitempty"`
}

// SignalProcessOptionsLCOW is the options passed to LCOW to signal a given
// process.
type SignalProcessOptionsLCOW struct {
	Signal int `json:",omitempty"`
}

// SignalProcessOptionsWCOW is the options passed to WCOW to signal a given
// process.
type SignalProcessOptionsWCOW struct {
	Signal guestrequest.SignalValueWCOW `json:",omitempty"`
}

// LCOWConfidentialOptions is used to set various confidential container specific
// options.
type LCOWConfidentialOptions struct {
	EnforcerType          string `json:"EnforcerType,omitempty"`
	EncodedSecurityPolicy string `json:"EncodedSecurityPolicy,omitempty"`
	EncodedUVMReference   string `json:"EncodedUVMReference,omitempty"`
}

type LCOWSecurityPolicyFragment struct {
	Fragment string `json:"Fragment,omitempty"`
}

type WCOWConfidentialOptions struct {
	EnforcerType          string `json:"EnforcerType,omitempty"`
	EncodedSecurityPolicy string `json:"EncodedSecurityPolicy,omitempty"`
	// Optional security policy
	WCOWSecurityPolicy string
	// Set when there is a security policy to apply on actual SNP hardware, use this rathen than checking the string length
	WCOWSecurityPolicyEnabled bool
	// Set which security policy enforcer to use (open door or rego). This allows for better fallback mechanic.
	WCOWSecurityPolicyEnforcer string
}
