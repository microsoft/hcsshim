package hcsoci

import "github.com/Microsoft/hcsshim/internal/hns"

func getNamespaceEndpoints(netNS string) ([]*hns.HNSEndpoint, error) {
	ids, err := hns.GetNamespaceEndpoints(netNS)
	if err != nil {
		return nil, err
	}
	var endpoints []*hns.HNSEndpoint
	for _, id := range ids {
		endpoint, err := hns.GetHNSEndpointByID(id)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints, nil
}
