//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/log"
	ncproxygrpc "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v1"
	"github.com/pkg/errors"
)

func hcnEndpointToEndpointResponse(ep *hcn.HostComputeEndpoint) (_ *ncproxygrpc.GetEndpointResponse, err error) {
	hcnEndpointResp := &ncproxygrpc.HcnEndpointSettings{
		Name:        ep.Name,
		Macaddress:  ep.MacAddress,
		NetworkName: ep.HostComputeNetwork,
		DnsSetting: &ncproxygrpc.DnsSetting{
			ServerIpAddrs: ep.Dns.ServerList,
			Domain:        ep.Dns.Domain,
			Search:        ep.Dns.Search,
		},
	}

	policies, err := parseEndpointPolicies(ep.Policies)
	if err != nil {
		return nil, err
	}
	hcnEndpointResp.Policies = policies

	ipConfigInfos := ep.IpConfigurations
	// there may be one ipv4 and/or one ipv6 configuration for an endpoint
	if len(ipConfigInfos) == 0 || len(ipConfigInfos) > 2 {
		return nil, errors.Errorf("invalid number (%v) of ip configuration information for endpoint %v", len(ipConfigInfos), ep.Name)
	}
	for _, ipConfig := range ipConfigInfos {
		ip := net.ParseIP(ipConfig.IpAddress)
		if ip == nil {
			return nil, errors.Errorf("failed to parse IP address %v", ipConfig.IpAddress)
		}
		if ip.To4() != nil {
			// this is an IPv4 address
			hcnEndpointResp.Ipaddress = ipConfig.IpAddress
			hcnEndpointResp.IpaddressPrefixlength = uint32(ipConfig.PrefixLength)
		} else {
			// this is an IPv6 address
			hcnEndpointResp.Ipv6Address = ipConfig.IpAddress
			hcnEndpointResp.Ipv6AddressPrefixlength = uint32(ipConfig.PrefixLength)
		}
	}

	return &ncproxygrpc.GetEndpointResponse{
		Namespace: ep.HostComputeNamespace,
		ID:        ep.Id,
		Endpoint: &ncproxygrpc.EndpointSettings{
			Settings: &ncproxygrpc.EndpointSettings_HcnEndpoint{
				HcnEndpoint: hcnEndpointResp,
			},
		},
	}, nil
}

func modifyEndpoint(ctx context.Context, id string, policies []hcn.EndpointPolicy, requestType hcn.RequestType) error {
	endpointRequest := hcn.PolicyEndpointRequest{
		Policies: policies,
	}

	settingsJSON, err := json.Marshal(endpointRequest)
	if err != nil {
		return err
	}

	requestMessage := &hcn.ModifyEndpointSettingRequest{
		ResourceType: hcn.EndpointResourceTypePolicy,
		RequestType:  requestType,
		Settings:     settingsJSON,
	}

	return hcn.ModifyEndpointSettings(id, requestMessage)
}

func parseEndpointPolicies(policies []hcn.EndpointPolicy) (*ncproxygrpc.HcnEndpointPolicies, error) {
	results := &ncproxygrpc.HcnEndpointPolicies{}
	for _, policy := range policies {
		switch policy.Type {
		case hcn.PortMapping:
			portMapSettings := &hcn.PortnameEndpointPolicySetting{}
			if err := json.Unmarshal(policy.Settings, portMapSettings); err != nil {
				return nil, err
			}
			results.PortnamePolicySetting = &ncproxygrpc.PortNameEndpointPolicySetting{
				PortName: portMapSettings.Name,
			}
		case hcn.IOV:
			iovSettings := &hcn.IovPolicySetting{}
			if err := json.Unmarshal(policy.Settings, iovSettings); err != nil {
				return nil, err
			}
			results.IovPolicySettings = &ncproxygrpc.IovEndpointPolicySetting{
				IovOffloadWeight:    iovSettings.IovOffloadWeight,
				QueuePairsRequested: iovSettings.QueuePairsRequested,
				InterruptModeration: iovSettings.InterruptModeration,
			}
		}
	}
	return results, nil
}

func constructEndpointPolicies(req *ncproxygrpc.HcnEndpointPolicies) ([]hcn.EndpointPolicy, error) {
	policies := []hcn.EndpointPolicy{}
	if req.IovPolicySettings != nil {
		iovSettings := hcn.IovPolicySetting{
			IovOffloadWeight:    req.IovPolicySettings.IovOffloadWeight,
			QueuePairsRequested: req.IovPolicySettings.QueuePairsRequested,
			InterruptModeration: req.IovPolicySettings.InterruptModeration,
		}
		iovJSON, err := json.Marshal(iovSettings)
		if err != nil {
			return []hcn.EndpointPolicy{}, errors.Wrap(err, "failed to marshal IovPolicySettings")
		}
		policy := hcn.EndpointPolicy{
			Type:     hcn.IOV,
			Settings: iovJSON,
		}
		policies = append(policies, policy)
	}

	if req.PortnamePolicySetting != nil {
		portPolicy := hcn.PortnameEndpointPolicySetting{
			Name: req.PortnamePolicySetting.PortName,
		}
		portPolicyJSON, err := json.Marshal(portPolicy)
		if err != nil {
			return []hcn.EndpointPolicy{}, errors.Wrap(err, "failed to marshal portname")
		}
		policy := hcn.EndpointPolicy{
			Type:     hcn.PortName,
			Settings: portPolicyJSON,
		}
		policies = append(policies, policy)
	}

	return policies, nil
}

func createHCNNetwork(ctx context.Context, req *ncproxygrpc.HostComputeNetworkSettings) (*hcn.HostComputeNetwork, error) {
	// Check if the network already exists, and if so return error.
	_, err := hcn.GetNetworkByName(req.Name)
	if err == nil {
		return nil, errors.Errorf("network with name %q already exists", req.Name)
	}

	policies := []hcn.NetworkPolicy{}
	if req.SwitchName != "" {
		// Get the layer ID from the external switch. HNS will create a transparent network for
		// any external switch that is created not through HNS so this is what we're
		// searching for here. If the network exists, the vSwitch with this name exists.
		extSwitch, err := hcn.GetNetworkByName(req.SwitchName)
		if err != nil {
			if _, ok := err.(hcn.NetworkNotFoundError); ok {
				return nil, errors.Errorf("no network/switch with name `%s` found", req.SwitchName)
			}
			return nil, errors.Wrapf(err, "failed to get network/switch with name %q", req.SwitchName)
		}

		// Get layer ID and use this as the basis for what to layer the new network over.
		if extSwitch.Health.Extra.LayeredOn == "" {
			return nil, errors.Errorf("no layer ID found for network %q found", extSwitch.Id)
		}

		layerPolicy := hcn.LayerConstraintNetworkPolicySetting{LayerId: extSwitch.Health.Extra.LayeredOn}
		data, err := json.Marshal(layerPolicy)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal layer policy")
		}

		netPolicy := hcn.NetworkPolicy{
			Type:     hcn.LayerConstraint,
			Settings: data,
		}
		policies = append(policies, netPolicy)
	}

	subnets := make([]hcn.Subnet, 0, len(req.SubnetIpaddressPrefix)+len(req.SubnetIpaddressPrefixIpv6))
	for _, addrPrefix := range req.SubnetIpaddressPrefix {
		subnet := hcn.Subnet{
			IpAddressPrefix: addrPrefix,
			Routes: []hcn.Route{
				{
					NextHop:           req.DefaultGateway,
					DestinationPrefix: "0.0.0.0/0",
				},
			},
		}
		subnets = append(subnets, subnet)
	}

	if len(req.SubnetIpaddressPrefixIpv6) != 0 {
		if err := hcn.IPv6DualStackSupported(); err != nil {
			// a request was made for an IPv6 address on a system that doesn't support IPv6
			return nil, fmt.Errorf("IPv6 address requested but not supported: %v", err)
		}
	}

	for _, ipv6AddrPrefix := range req.SubnetIpaddressPrefixIpv6 {
		subnet := hcn.Subnet{
			IpAddressPrefix: ipv6AddrPrefix,
			Routes: []hcn.Route{
				{
					NextHop:           req.DefaultGatewayIpv6,
					DestinationPrefix: "::/0",
				},
			},
		}
		subnets = append(subnets, subnet)
	}

	ipam := hcn.Ipam{
		Type:    req.IpamType.String(),
		Subnets: subnets,
	}

	network := &hcn.HostComputeNetwork{
		Name:     req.Name,
		Type:     hcn.NetworkType(req.Mode.String()),
		Ipams:    []hcn.Ipam{ipam},
		Policies: policies,
		SchemaVersion: hcn.SchemaVersion{
			Major: 2,
			Minor: 2,
		},
	}

	network, err = network.Create()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create HNS network %q", req.Name)
	}

	return network, nil
}

func hcnNetworkToNetworkResponse(ctx context.Context, network *hcn.HostComputeNetwork) (*ncproxygrpc.GetNetworkResponse, error) {
	var (
		ipamType                  int32
		defaultGateway            string
		defaultGatewayIPv6        string
		switchName                string
		subnetIPAddressPrefixes   []string
		subnetIPv6AddressPrefixes []string
	)

	for _, ipam := range network.Ipams {
		// all ipams should have the same type so just keep the last one
		ipamType = ncproxygrpc.HostComputeNetworkSettings_IpamType_value[ipam.Type]
		for _, subnet := range ipam.Subnets {
			// split prefix off string so we can check if this is ipv4 or ipv6
			ipParts := strings.Split(subnet.IpAddressPrefix, "/")
			ipPrefix := net.ParseIP(ipParts[0])
			if ipPrefix == nil {
				return nil, fmt.Errorf("failed to parse IP address %v", ipPrefix)
			}
			if ipPrefix.To4() != nil {
				// this is an IPv4 address
				subnetIPAddressPrefixes = append(subnetIPAddressPrefixes, subnet.IpAddressPrefix)
				if len(subnet.Routes) != 0 {
					defaultGateway = subnet.Routes[0].NextHop
				}
			} else {
				// this is an IPv6 address
				subnetIPv6AddressPrefixes = append(subnetIPv6AddressPrefixes, subnet.IpAddressPrefix)
				if len(subnet.Routes) != 0 {
					defaultGatewayIPv6 = subnet.Routes[0].NextHop
				}
			}
		}
	}

	mode := ncproxygrpc.HostComputeNetworkSettings_NetworkMode_value[string(network.Type)]

	if network.Health.Extra.SwitchGuid != "" {
		extSwitch, err := hcn.GetNetworkByID(network.Health.Extra.SwitchGuid)
		if err != nil {
			return nil, err
		}
		switchName = extSwitch.Name
	}

	settings := &ncproxygrpc.HostComputeNetworkSettings{
		Name:                      network.Name,
		Mode:                      ncproxygrpc.HostComputeNetworkSettings_NetworkMode(mode),
		SwitchName:                switchName,
		IpamType:                  ncproxygrpc.HostComputeNetworkSettings_IpamType(ipamType),
		SubnetIpaddressPrefix:     subnetIPAddressPrefixes,
		DefaultGateway:            defaultGateway,
		SubnetIpaddressPrefixIpv6: subnetIPv6AddressPrefixes,
		DefaultGatewayIpv6:        defaultGatewayIPv6,
	}

	var startMac, endMac string
	switch n := len(network.MacPool.Ranges); {
	case n < 1:
		return nil, fmt.Errorf("network %s(%s) MAC pool is empty", network.Name, network.Id)
	case n > 1:
		log.G(ctx).WithField("networkName", network.Name).Warn("network has multiple MAC pools, only returning the first")
		fallthrough
	default:
		startMac = network.MacPool.Ranges[0].StartMacAddress
		endMac = network.MacPool.Ranges[0].EndMacAddress
	}

	return &ncproxygrpc.GetNetworkResponse{
		ID: network.Id,
		Network: &ncproxygrpc.Network{
			Settings: &ncproxygrpc.Network_HcnNetwork{
				HcnNetwork: settings,
			},
		},
		MacRange: &ncproxygrpc.MacRange{
			StartMacAddress: startMac,
			EndMacAddress:   endMac,
		},
	}, nil
}

func createHCNEndpoint(ctx context.Context, network *hcn.HostComputeNetwork, req *ncproxygrpc.HcnEndpointSettings) (*hcn.HostComputeEndpoint, error) {
	// Construct ip config.
	ipConfigs := []hcn.IpConfig{}
	if req.Ipaddress != "" && req.IpaddressPrefixlength != 0 {
		ipv4Config := hcn.IpConfig{
			IpAddress:    req.Ipaddress,
			PrefixLength: uint8(req.IpaddressPrefixlength),
		}
		ipConfigs = append(ipConfigs, ipv4Config)
	}

	if req.Ipv6Address != "" && req.Ipv6AddressPrefixlength != 0 {
		if err := hcn.IPv6DualStackSupported(); err != nil {
			// a request was made for an IPv6 address on a system that doesn't support IPv6
			return nil, fmt.Errorf("IPv6 address requested but not supported: %v", err)
		}
		ipv6Config := hcn.IpConfig{
			IpAddress:    req.Ipv6Address,
			PrefixLength: uint8(req.Ipv6AddressPrefixlength),
		}
		ipConfigs = append(ipConfigs, ipv6Config)
	}

	var err error
	policies := []hcn.EndpointPolicy{}
	if req.Policies != nil {
		policies, err = constructEndpointPolicies(req.Policies)
		if err != nil {
			return nil, errors.Wrap(err, "failed to construct endpoint policies")
		}
	}

	endpoint := &hcn.HostComputeEndpoint{
		Name:               req.Name,
		HostComputeNetwork: network.Id,
		MacAddress:         req.Macaddress,
		IpConfigurations:   ipConfigs,
		Policies:           policies,
		SchemaVersion: hcn.SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	if req.DnsSetting != nil {
		endpoint.Dns = hcn.Dns{
			ServerList: req.DnsSetting.ServerIpAddrs,
			Domain:     req.DnsSetting.Domain,
			Search:     req.DnsSetting.Search,
		}
	}
	endpoint, err = endpoint.Create()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HNS endpoint")
	}

	return endpoint, nil
}

// getHostDefaultNamespace returns the first namespace found that has type [hcn.NamespaceTypeHostDefault],
// or an error if none is found.
func getHostDefaultNamespace() (string, error) {
	namespaces, err := hcn.ListNamespaces()
	if err != nil {
		return "", errors.Wrapf(err, "failed list namespaces")
	}

	for _, ns := range namespaces {
		if ns.Type == hcn.NamespaceTypeHostDefault {
			return ns.Id, nil
		}
	}
	return "", errors.New("unable to find default host namespace to attach to")
}
