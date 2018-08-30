package hcsshimtest

import (
	"encoding/json"

	"github.com/Microsoft/hcsshim"
)

func cleanup() {
	// Delete test network (if exists)
	testNetwork, err := hcsshim.GetNetworkByName(NatTestNetworkName)
	if err != nil {
		return
	}
	if testNetwork != nil {
		_, err := testNetwork.Delete()
		if err != nil {
			return
		}
	}
}

func HcnCreateTestNetwork() (*hcsshim.HostComputeNetwork, error) {
	cleanup()
	network := &hcsshim.HostComputeNetwork{
		Type: "NAT",
		Name: NatTestNetworkName,
		MacPool: hcsshim.HcnMacPool{
			Ranges: []hcsshim.MacRange{
				hcsshim.MacRange{
					StartMacAddress: "00-15-5D-52-C0-00",
					EndMacAddress:   "00-15-5D-52-CF-FF",
				},
			},
		},
		Ipams: []hcsshim.Ipam{
			hcsshim.Ipam{
				Type: "Static",
				Subnets: []hcsshim.HcnSubnet{
					hcsshim.HcnSubnet{
						IpAddressPrefix: "192.168.100.0/24",
						Routes: []hcsshim.HcnRoute{
							hcsshim.HcnRoute{
								NextHop:           "192.168.100.1",
								DestinationPrefix: "0.0.0.0",
							},
						},
					},
				},
			},
		},
		SchemaVersion: hcsshim.SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	return network.Create()
}

func HcnCreateTestEndpoint(network *hcsshim.HostComputeNetwork) (*hcsshim.HostComputeEndpoint, error) {
	Endpoint := &hcsshim.HostComputeEndpoint{
		Name: NatTestEndpointName,
		SchemaVersion: hcsshim.SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	return network.CreateEndpoint(Endpoint)
}

func HcnCreateTestEndpointWithNamespace(network *hcsshim.HostComputeNetwork, namespace *hcsshim.HostComputeNamespace) (*hcsshim.HostComputeEndpoint, error) {
	Endpoint := &hcsshim.HostComputeEndpoint{
		Name:                 NatTestEndpointName,
		HostComputeNamespace: namespace.Id,
		SchemaVersion: hcsshim.SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	return network.CreateEndpoint(Endpoint)
}

func HcnCreateTestNamespace() (*hcsshim.HostComputeNamespace, error) {
	namespace := &hcsshim.HostComputeNamespace{
		Type:        "HostDefault",
		NamespaceId: 5,
		SchemaVersion: hcsshim.SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	return namespace.Create()
}

func HcnCreateAclsAllowIn() (*hcsshim.EndpointPolicy, error) {
	in := hcsshim.AclPolicySetting{
		Protocols:       "6,17",
		Action:          "Allow",
		Direction:       "In",
		LocalAddresses:  "192.168.100.0/24,10.0.0.21",
		RemoteAddresses: "192.168.100.0/24,10.0.0.21",
		LocalPorts:      "80,8080",
		RemotePorts:     "80,8080",
		RuleType:        "Switch",
		Priority:        200,
	}

	rawJSON, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	endpointPolicy := hcsshim.EndpointPolicy{
		Type:     "ACL",
		Settings: rawJSON,
	}

	return &endpointPolicy, nil
}

func HcnCreateTestLoadBalancer(endpoint *hcsshim.HostComputeEndpoint) (*hcsshim.HostComputeLoadBalancer, error) {
	loadBalancer := &hcsshim.HostComputeLoadBalancer{
		HostComputeEndpoints: []string{endpoint.Id},
		SourceVIP:            "10.0.0.1",
		PortMappings: []hcsshim.LoadBalancerPortMapping{
			hcsshim.LoadBalancerPortMapping{
				Protocol:     6, // TCP
				InternalPort: 8080,
				ExternalPort: 8090,
			},
		},
		FrontendVIPs: []string{"1.1.1.2", "1.1.1.3"},
		SchemaVersion: hcsshim.SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	return loadBalancer.Create()
}
