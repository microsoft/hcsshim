package main

import (
	"context"
	"encoding/json"

	"github.com/Microsoft/hcsshim/hcn"
	ncproxygrpc "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v1"
	"github.com/pkg/errors"
)

func hcnEndpointToEndpointResponse(ep *hcn.HostComputeEndpoint) (_ *ncproxygrpc.GetEndpointResponse, err error) {
	policies, err := parseEndpointPolicies(ep.Policies)
	if err != nil {
		return nil, err
	}
	ipConfigInfo := ep.IpConfigurations
	if len(ipConfigInfo) == 0 {
		return nil, errors.Errorf("failed to find network %v ip configuration information", ep.Name)
	}

	return &ncproxygrpc.GetEndpointResponse{
		Namespace: ep.HostComputeNamespace,
		ID:        ep.Id,
		Endpoint: &ncproxygrpc.EndpointSettings{
			Settings: &ncproxygrpc.EndpointSettings_HcnEndpoint{
				HcnEndpoint: &ncproxygrpc.HcnEndpointSettings{
					Name:       ep.Name,
					Macaddress: ep.MacAddress,
					// only use the first ip config returned since we only expect there to be one
					Ipaddress:             ep.IpConfigurations[0].IpAddress,
					IpaddressPrefixlength: uint32(ep.IpConfigurations[0].PrefixLength),
					NetworkName:           ep.HostComputeNetwork,
					Policies:              policies,
					DnsSetting: &ncproxygrpc.DnsSetting{
						ServerIpAddrs: ep.Dns.ServerList,
						Domain:        ep.Dns.Domain,
						Search:        ep.Dns.Search,
					},
				},
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

	subnets := make([]hcn.Subnet, len(req.SubnetIpaddressPrefix))
	for i, addrPrefix := range req.SubnetIpaddressPrefix {
		subnet := hcn.Subnet{
			IpAddressPrefix: addrPrefix,
			Routes: []hcn.Route{
				{
					NextHop:           req.DefaultGateway,
					DestinationPrefix: "0.0.0.0/0",
				},
			},
		}
		subnets[i] = subnet
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

func hcnNetworkToNetworkResponse(network *hcn.HostComputeNetwork) (*ncproxygrpc.GetNetworkResponse, error) {
	var (
		ipamType                int32
		defaultGateway          string
		switchName              string
		subnetIPAddressPrefixes []string
	)

	for _, ipam := range network.Ipams {
		for _, subnet := range ipam.Subnets {
			subnetIPAddressPrefixes = append(subnetIPAddressPrefixes, subnet.IpAddressPrefix)
		}
	}

	if len(network.Ipams) > 0 {
		// only use the first ipam type returned since we expect that to the the same type for all subnets added
		ipamType = ncproxygrpc.HostComputeNetworkSettings_IpamType_value[network.Ipams[0].Type]
		if len(network.Ipams[0].Subnets) > 0 && len(network.Ipams[0].Subnets[0].Routes) > 0 {
			// only use the first route as we expect all routes to use the default gateway as the next
			// see createHCNNetwork.
			defaultGateway = network.Ipams[0].Subnets[0].Routes[0].NextHop
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
		Name:                  network.Name,
		Mode:                  ncproxygrpc.HostComputeNetworkSettings_NetworkMode(mode),
		SwitchName:            switchName,
		IpamType:              ncproxygrpc.HostComputeNetworkSettings_IpamType(ipamType),
		SubnetIpaddressPrefix: subnetIPAddressPrefixes,
		DefaultGateway:        defaultGateway,
	}

	return &ncproxygrpc.GetNetworkResponse{
		ID: network.Id,
		Network: &ncproxygrpc.Network{
			Settings: &ncproxygrpc.Network_HcnNetwork{
				HcnNetwork: settings,
			},
		},
	}, nil
}

func createHCNEndpoint(ctx context.Context, network *hcn.HostComputeNetwork, req *ncproxygrpc.HcnEndpointSettings) (*hcn.HostComputeEndpoint, error) {
	// Construct ip config.
	ipConfig := hcn.IpConfig{
		IpAddress:    req.Ipaddress,
		PrefixLength: uint8(req.IpaddressPrefixlength),
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
		IpConfigurations:   []hcn.IpConfig{ipConfig},
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
