//go:build windows

package guestresource

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Microsoft/hcsshim/hcn"

	"github.com/samber/lo"
)

// BuildLCOWNetworkAdapter converts an HCN endpoint into the [LCOWNetworkAdapter]
// payload that the GCS expects.
func BuildLCOWNetworkAdapter(nicID string, endpoint *hcn.HostComputeEndpoint, policyBasedRouting bool) (*LCOWNetworkAdapter, error) {
	req := &LCOWNetworkAdapter{
		NamespaceID: endpoint.HostComputeNamespace,
		ID:          nicID,
		MacAddress:  endpoint.MacAddress,
		IPConfigs:   make([]LCOWIPConfig, 0, len(endpoint.IpConfigurations)),
		Routes:      make([]LCOWRoute, 0, len(endpoint.Routes)),
	}

	for _, i := range endpoint.IpConfigurations {
		ipConfig := LCOWIPConfig{
			IPAddress:    i.IpAddress,
			PrefixLength: i.PrefixLength,
		}
		req.IPConfigs = append(req.IPConfigs, ipConfig)
	}

	for _, r := range endpoint.Routes {
		newRoute := LCOWRoute{
			DestinationPrefix: r.DestinationPrefix,
			NextHop:           r.NextHop,
			Metric:            r.Metric,
		}
		req.Routes = append(req.Routes, newRoute)
	}

	// !NOTE:
	// the `DNSSuffix` field is explicitly used as the search list for host-name lookup in
	// the guest's `resolv.conf`, and not as the DNS suffix.
	// The name is a legacy hold over.

	// use DNS domain as the first (default) search value, if it is provided
	searches := endpoint.Dns.Search
	if endpoint.Dns.Domain != "" {
		searches = append([]string{endpoint.Dns.Domain}, searches...)
	}

	// canonicalize the DNS config
	canon := func(s string, _ int) string {
		// zone identifiers in IPv6 addresses really, really shouldn't be case sensitive, but ... *shrug*
		return strings.ToLower(s)
	}
	servers := lo.Map(endpoint.Dns.ServerList, canon)
	searches = lo.Map(searches, canon)

	req.DNSSuffix = strings.Join(searches, ",")
	req.DNSServerList = strings.Join(servers, ",")

	for _, p := range endpoint.Policies {
		if p.Type == hcn.EncapOverhead {
			var settings hcn.EncapOverheadEndpointPolicySetting
			if err := json.Unmarshal(p.Settings, &settings); err != nil {
				return nil, fmt.Errorf("unmarshal encap overhead policy setting: %w", err)
			}
			req.EncapOverhead = settings.Overhead
		}
	}

	req.PolicyBasedRouting = policyBasedRouting

	return req, nil
}
