//go:build windows

package hns

import (
	hns2 "github.com/Microsoft/hcsshim/hns/internal"
)

// HNSEndpoint represents a network endpoint in HNS
type HNSEndpoint = hns2.HNSEndpoint

// HNSEndpointStats represent the stats for an networkendpoint in HNS
type HNSEndpointStats = hns2.EndpointStats

// Namespace represents a Compartment.
type Namespace = hns2.Namespace

// SystemType represents the type of the system on which actions are done
type SystemType string

// SystemType const
const (
	ContainerType      SystemType = "Container"
	VirtualMachineType SystemType = "VirtualMachine"
	HostType           SystemType = "Host"
)

// EndpointAttachDetachRequest is the structure used to send request to the container to modify the system
// Supported resource types are Network and Request Types are Add/Remove
type EndpointAttachDetachRequest = hns2.EndpointAttachDetachRequest

// EndpointResquestResponse is object to get the endpoint request response
type EndpointResquestResponse = hns2.EndpointResquestResponse

// HNSEndpointRequest makes a HNS call to modify/query a network endpoint
func HNSEndpointRequest(method, path, request string) (*HNSEndpoint, error) {
	return hns2.HNSEndpointRequest(method, path, request)
}

// HNSListEndpointRequest makes a HNS call to query the list of available endpoints
func HNSListEndpointRequest() ([]HNSEndpoint, error) {
	return hns2.HNSListEndpointRequest()
}

// GetHNSEndpointByID get the Endpoint by ID
func GetHNSEndpointByID(endpointID string) (*HNSEndpoint, error) {
	return hns2.GetHNSEndpointByID(endpointID)
}

// GetHNSEndpointByName gets the endpoint filtered by Name
func GetHNSEndpointByName(endpointName string) (*HNSEndpoint, error) {
	return hns2.GetHNSEndpointByName(endpointName)
}

// GetHNSEndpointStats gets the endpoint stats by ID
func GetHNSEndpointStats(endpointName string) (*HNSEndpointStats, error) {
	return hns2.GetHNSEndpointStats(endpointName)
}

func CreateNamespace() (string, error) {
	return hns2.CreateNamespace()
}

func GetNamespaceEndpoints(id string) ([]string, error) {
	return hns2.GetNamespaceEndpoints(id)
}

func RemoveNamespaceEndpoint(id string, endpointID string) error {
	return hns2.RemoveNamespaceEndpoint(id, endpointID)
}

func RemoveNamespace(id string) error {
	return hns2.RemoveNamespace(id)
}
