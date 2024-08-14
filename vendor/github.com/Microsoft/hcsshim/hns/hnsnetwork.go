//go:build windows

package hns

import (
	"github.com/Microsoft/hcsshim/hns/internal"
)

// Subnet is associated with a network and represents a list
// of subnets available to the network
type Subnet = internal.Subnet

// MacPool is associated with a network and represents a list
// of macaddresses available to the network
type MacPool = internal.MacPool

// HNSNetwork represents a network in HNS
type HNSNetwork = internal.HNSNetwork

// HNSNetworkRequest makes a call into HNS to update/query a single network
func HNSNetworkRequest(method, path, request string) (*HNSNetwork, error) {
	return internal.HNSNetworkRequest(method, path, request)
}

// HNSListNetworkRequest makes a HNS call to query the list of available networks
func HNSListNetworkRequest(method, path, request string) ([]HNSNetwork, error) {
	return internal.HNSListNetworkRequest(method, path, request)
}

// GetHNSNetworkByID
func GetHNSNetworkByID(networkID string) (*HNSNetwork, error) {
	return internal.GetHNSNetworkByID(networkID)
}

// GetHNSNetworkName filtered by Name
func GetHNSNetworkByName(networkName string) (*HNSNetwork, error) {
	return internal.GetHNSNetworkByName(networkName)
}
