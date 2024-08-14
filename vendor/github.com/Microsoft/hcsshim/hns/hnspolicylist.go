//go:build windows

package hns

import (
	"github.com/Microsoft/hcsshim/hns/internal"
)

// RoutePolicy is a structure defining schema for Route based Policy
type RoutePolicy = internal.RoutePolicy

// ELBPolicy is a structure defining schema for ELB LoadBalancing based Policy
type ELBPolicy = internal.ELBPolicy

// LBPolicy is a structure defining schema for LoadBalancing based Policy
type LBPolicy = internal.LBPolicy

// PolicyList is a structure defining schema for Policy list request
type PolicyList = internal.PolicyList

// HNSPolicyListRequest makes a call into HNS to update/query a single network
func HNSPolicyListRequest(method, path, request string) (*PolicyList, error) {
	return internal.HNSPolicyListRequest(method, path, request)
}

// HNSListPolicyListRequest gets all the policy list
func HNSListPolicyListRequest() ([]PolicyList, error) {
	return internal.HNSListPolicyListRequest()
}

// PolicyListRequest makes a HNS call to modify/query a network policy list
func PolicyListRequest(method, path, request string) (*PolicyList, error) {
	return internal.PolicyListRequest(method, path, request)
}

// GetPolicyListByID get the policy list by ID
func GetPolicyListByID(policyListID string) (*PolicyList, error) {
	return internal.GetPolicyListByID(policyListID)
}

// AddLoadBalancer policy list for the specified endpoints
func AddLoadBalancer(endpoints []HNSEndpoint, isILB bool, sourceVIP, vip string, protocol uint16, internalPort uint16, externalPort uint16) (*PolicyList, error) {
	return internal.AddLoadBalancer(endpoints, isILB, sourceVIP, vip, protocol, internalPort, externalPort)
}

// AddRoute adds route policy list for the specified endpoints
func AddRoute(endpoints []HNSEndpoint, destinationPrefix string, nextHop string, encapEnabled bool) (*PolicyList, error) {
	return internal.AddRoute(endpoints, destinationPrefix, nextHop, encapEnabled)
}
