//go:build windows

package hcs

import (
	"github.com/Microsoft/hcsshim/hcs/internal/schema1"
)

// ContainerProperties holds the properties for a container and the processes running in that container
type ContainerProperties = schema1.ContainerProperties

// MemoryStats holds the memory statistics for a container
type MemoryStats = schema1.MemoryStats

// ProcessorStats holds the processor statistics for a container
type ProcessorStats = schema1.ProcessorStats

// StorageStats holds the storage statistics for a container
type StorageStats = schema1.StorageStats

// NetworkStats holds the network statistics for a container
type NetworkStats = schema1.NetworkStats

// Statistics is the structure returned by a statistics call on a container
type Statistics = schema1.Statistics

// ProcessList is the structure of an item returned by a ProcessList call on a container
type ProcessListItem = schema1.ProcessListItem

// MappedVirtualDiskController is the structure of an item returned by a MappedVirtualDiskList call on a container
type MappedVirtualDiskController = schema1.MappedVirtualDiskController

// Type of Request Support in ModifySystem
type RequestType = schema1.RequestType

// Type of Resource Support in ModifySystem
type ResourceType = schema1.ResourceType

type GuestDefinedCapabilities = schema1.GuestDefinedCapabilities

type ResourceModificationRequestResponse = schema1.ResourceModificationRequestResponse

// RequestType const
const (
	Add     RequestType  = "Add"
	Remove  RequestType  = "Remove"
	Network ResourceType = "Network"
)
