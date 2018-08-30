// Shim for the Host Compute Network Service (HCN) to manage networking for
// Windows Server containers and Hyper-V containers.

package hcsshim

import (
	"github.com/Microsoft/hcsshim/internal/hcnshim"
)

// SchemaVersion for HNS Objects/Queries.
type SchemaVersion = hcnshim.SchemaVersion

// HcnQuery is the format for HNS queries.
type HcnQuery = hcnshim.HostComputeQuery

//////////// Support

// HcnSupportedFeatures are the features provided by the HNS Service.
type HcnSupportedFeatures = hcnshim.HNSSupportedFeatures

// HcnAclFeatures are the supported ACL possibilities.
type HcnAclFeatures = hcnshim.HNSAclFeatures

// HcnApiSupport are the supported API possibilities.
type HcnApiSupport = hcnshim.HNSApiSupport

// GetHcnSupportedFeatures returns the features supported by the HNS Service.
func GetHcnSupportedFeatures() HcnSupportedFeatures {
	return hcnshim.GetHNSSupportedFeatures()
}

//////////// Network

// HcnRoute is assoicated with a subnet.
// Note: HcnRoute because V1 api already has a Route
type HcnRoute = hcnshim.Route

// HcnSubnet is assoicated with a Ipam.
// Note: HcnSubnet because V1 api already has a Subnet
type HcnSubnet = hcnshim.Subnet

// Ipam (Internet Protocol Addres Management) is assoicated with a network
// and represents the address space(s) of a network.
type Ipam = hcnshim.Ipam

// MacRange is associated with MacPool and respresents the start and end addresses.
type MacRange = hcnshim.MacRange

// HcnMacPool is assoicated with a network and represents pool of MacRanges.
// Note: HcnMacPool because V1 api already has a MacPool
type HcnMacPool = hcnshim.MacPool

// Dns (Domain Name System is associated with a network.
type Dns = hcnshim.Dns

// HostComputeNetwork represents a network in HNS
type HostComputeNetwork = hcnshim.HostComputeNetwork

// ListNetworks makes a HNS call to list all available networks.
func ListNetworks() ([]HostComputeNetwork, error) {
	return hcnshim.ListNetworks()
}

// ListNetworksQuery makes a HNS call to query the list of available networks.
func ListNetworksQuery(query string) ([]HostComputeNetwork, error) {
	return hcnshim.ListNetworksQuery(query)
}

// GetNetworkByID returns the network specified by Id.
func GetNetworkByID(networkId string) (*HostComputeNetwork, error) {
	return hcnshim.GetNetworkByID(networkId)
}

// GetNetworkByName returns the network specified by Name.
func GetNetworkByName(networkName string) (*HostComputeNetwork, error) {
	return hcnshim.GetNetworkByName(networkName)
}

//////////// Endpoint

// IpConfig is assoicated with an endpoint and represents an IpAddress and PrefixLen.
type IpConfig = hcnshim.IpConfig

// HostComputeEndpoint represents a network endpoint in HNS
type HostComputeEndpoint = hcnshim.HostComputeEndpoint

// ModifyEndpointSettingRequest is the structure used to update endpoint settings.
// Used to change port or policy (ex: ACLs) to an endpoint.
type ModifyEndpointSettingRequest = hcnshim.ModifyEndpointSettingRequest

// ListEndpoints makes a HNS call to list all available endpoints.
func ListEndpoints() ([]HostComputeEndpoint, error) {
	return hcnshim.ListEndpoints()
}

// ListEndpointsQuery makes a HNS call to query the list of available endpoints.
func ListEndpointsQuery(query string) ([]HostComputeEndpoint, error) {
	return hcnshim.ListEndpointsQuery(query)
}

// ListEndpointsOfNetwork queries the list of endpoints on a network.
func ListEndpointsOfNetwork(networkId string) ([]HostComputeEndpoint, error) {
	return hcnshim.ListEndpointsOfNetwork(networkId)
}

// GetEndpointByID returns the endpoint specified by Id.
func GetEndpointByID(endpointId string) (*HostComputeEndpoint, error) {
	return hcnshim.GetEndpointByID(endpointId)
}

// GetEndpointByName returns the endpoint specified by Name.
func GetEndpointByName(endpointName string) (*HostComputeEndpoint, error) {
	return hcnshim.GetEndpointByName(endpointName)
}

// ModifyEndpointSettings updates the port/policy of an Endpoint.
func ModifyEndpointSettings(endpointId string, request *ModifyEndpointSettingRequest) error {
	return hcnshim.ModifyEndpointSettings(endpointId, request)
}

//////////// Namespace

// NamespaceResource is associated with a namespace
type NamespaceResource = hcnshim.NamespaceResource

// HostComputeNamespace represents a namespace in HNS
type HostComputeNamespace = hcnshim.HostComputeNamespace

// ModifyNamespaceSettingRequest is the structure used to send request to modify a namespace.
// Used to Add/Remove an endpoints and containers to/from a namespace.
type ModifyNamespaceSettingRequest = hcnshim.ModifyNamespaceSettingRequest

// ListNamespaces makes a HNS call to list all available namespaces.
func ListNamespaces() ([]HostComputeNamespace, error) {
	return hcnshim.ListNamespaces()
}

// ListNamespacesQuery makes a HNS call to query the list of available namespaces.
func ListNamespacesQuery(query string) ([]HostComputeNamespace, error) {
	return hcnshim.ListNamespacesQuery(query)
}

// GetNamespaceByID returns the Namespace specified by Id.
func GetNamespaceByID(namespaceId string) (*HostComputeNamespace, error) {
	return hcnshim.GetNamespaceByID(namespaceId)
}

// GetNamespaceEndpointIds returns the endpoints of the Namespace specified by Id.
func GetNamespaceEndpointIds(namespaceId string) ([]string, error) {
	return hcnshim.GetNamespaceEndpointIds(namespaceId)
}

// GetNamespaceContainerIds returns the containers of the Namespace specified by Id.
func GetNamespaceContainerIds(namespaceId string) ([]string, error) {
	return hcnshim.GetNamespaceContainerIds(namespaceId)
}

// AddNamespaceEndpoint adds an endpoint to a Namespace.
func AddNamespaceEndpoint(namespaceId string, endpointId string) error {
	return hcnshim.AddNamespaceEndpoint(namespaceId, endpointId)
}

// RemoveNamespaceEndpoint removes an endpoint from a Namespace.
func RemoveNamespaceEndpoint(namespaceId string, endpointId string) error {
	return hcnshim.RemoveNamespaceEndpoint(namespaceId, endpointId)
}

// ModifyNamespaceSettings updates the Endpoints/Containers of a Namespace.
func ModifyNamespaceSettings(namespaceId string, request *ModifyNamespaceSettingRequest) error {
	return hcnshim.ModifyNamespaceSettings(namespaceId, request)
}

//////////// Policy

// EndpointPolicy is a collection of Policy settings for an Endpoint.
type EndpointPolicy = hcnshim.EndpointPolicy

// NetworkPolicy is a collection of Policy settings for a Network.
type NetworkPolicy = hcnshim.NetworkPolicy

// SubnetPolicy is a collection of Policy settings for a Subnet.
type SubnetPolicy = hcnshim.SubnetPolicy

// AclPolicySetting creates firewall rules on an endpoint
type AclPolicySetting = hcnshim.AclPolicySetting

//////////// LoadBalancer

// LoadBalancerPortMapping is associated with HostComputeLoadBalancer
type LoadBalancerPortMapping = hcnshim.LoadBalancerPortMapping

// HostComputeLoadBalancer represents software load balancer.
type HostComputeLoadBalancer = hcnshim.HostComputeLoadBalancer

// ListLoadBalancers makes a HNS call to list all available loadBalancers.
func ListLoadBalancers() ([]HostComputeLoadBalancer, error) {
	return hcnshim.ListLoadBalancers()
}

// ListLoadBalancersQuery makes a HNS call to query the list of available loadBalancers.
func ListLoadBalancersQuery(query string) ([]HostComputeLoadBalancer, error) {
	return hcnshim.ListLoadBalancersQuery(query)
}

// GetLoadBalancerByID returns the LoadBalancer specified by Id.
func GetLoadBalancerByID(loadBalancerId string) (*HostComputeLoadBalancer, error) {
	return hcnshim.GetLoadBalancerByID(loadBalancerId)
}

// AddHcnLoadBalancer for the specified endpoints
// Note: AddLoadBalancer already existed for V1
func AddHcnLoadBalancer(endpoints []HostComputeEndpoint, isILB bool, sourceVIP string, frontendVIPs []string, protocol uint16, internalPort uint16, externalPort uint16) (*HostComputeLoadBalancer, error) {
	return hcnshim.AddLoadBalancer(endpoints, isILB, sourceVIP, frontendVIPs, protocol, internalPort, externalPort)
}
