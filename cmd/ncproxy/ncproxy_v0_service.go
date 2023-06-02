package main

import (
	"context"
	"errors"
	"strconv"

	ncproxygrpcv0 "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v0"
	ncproxygrpc "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	errUnsupportedNetworkType  = errors.New("unsupported network type")
	errUnsupportedEndpointType = errors.New("unsupported endpoint type")
)

type v0ServiceWrapper struct {
	ncproxygrpcv0.UnimplementedNetworkConfigProxyServer

	s *grpcService
}

func newV0ServiceWrapper(s *grpcService) *v0ServiceWrapper {
	return &v0ServiceWrapper{s: s}
}

var _ ncproxygrpcv0.NetworkConfigProxyServer = &v0ServiceWrapper{}

func (w *v0ServiceWrapper) AddNIC(ctx context.Context, req *ncproxygrpcv0.AddNICRequest) (_ *ncproxygrpcv0.AddNICResponse, err error) {
	v1Req := &ncproxygrpc.AddNICRequest{
		ContainerID:  req.ContainerID,
		NicID:        req.NicID,
		EndpointName: req.EndpointName,
	}
	_, err = w.s.AddNIC(ctx, v1Req)
	if err != nil {
		return nil, err
	}
	return &ncproxygrpcv0.AddNICResponse{}, nil
}

func (w *v0ServiceWrapper) ModifyNIC(ctx context.Context, req *ncproxygrpcv0.ModifyNICRequest) (_ *ncproxygrpcv0.ModifyNICResponse, err error) {
	v1Req := &ncproxygrpc.ModifyNICRequest{
		ContainerID:  req.ContainerID,
		NicID:        req.NicID,
		EndpointName: req.EndpointName,
	}
	if req.IovPolicySettings != nil {
		v1Req.EndpointSettings = &ncproxygrpc.EndpointSettings{
			Settings: &ncproxygrpc.EndpointSettings_HcnEndpoint{
				HcnEndpoint: &ncproxygrpc.HcnEndpointSettings{
					Policies: &ncproxygrpc.HcnEndpointPolicies{
						IovPolicySettings: &ncproxygrpc.IovEndpointPolicySetting{
							IovOffloadWeight:    req.IovPolicySettings.IovOffloadWeight,
							QueuePairsRequested: req.IovPolicySettings.QueuePairsRequested,
							InterruptModeration: req.IovPolicySettings.InterruptModeration,
						},
					},
				},
			},
		}
	}
	_, err = w.s.ModifyNIC(ctx, v1Req)
	if err != nil {
		return nil, err
	}
	return &ncproxygrpcv0.ModifyNICResponse{}, nil
}

func (w *v0ServiceWrapper) DeleteNIC(ctx context.Context, req *ncproxygrpcv0.DeleteNICRequest) (_ *ncproxygrpcv0.DeleteNICResponse, err error) {
	v1Req := &ncproxygrpc.DeleteNICRequest{
		ContainerID:  req.ContainerID,
		NicID:        req.NicID,
		EndpointName: req.EndpointName,
	}
	_, err = w.s.DeleteNIC(ctx, v1Req)
	if err != nil {
		return nil, err
	}
	return &ncproxygrpcv0.DeleteNICResponse{}, nil
}

func (w *v0ServiceWrapper) CreateNetwork(ctx context.Context, req *ncproxygrpcv0.CreateNetworkRequest) (_ *ncproxygrpcv0.CreateNetworkResponse, err error) {
	v1Req := &ncproxygrpc.CreateNetworkRequest{
		Network: &ncproxygrpc.Network{
			Settings: &ncproxygrpc.Network_HcnNetwork{
				HcnNetwork: &ncproxygrpc.HostComputeNetworkSettings{
					Name:                  req.Name,
					Mode:                  ncproxygrpc.HostComputeNetworkSettings_NetworkMode(req.Mode),
					SwitchName:            req.SwitchName,
					IpamType:              ncproxygrpc.HostComputeNetworkSettings_IpamType(req.IpamType),
					SubnetIpaddressPrefix: req.SubnetIpaddressPrefix,
					DefaultGateway:        req.DefaultGateway,
				},
			},
		},
	}
	resp, err := w.s.CreateNetwork(ctx, v1Req)
	if err != nil {
		return nil, err
	}
	return &ncproxygrpcv0.CreateNetworkResponse{
		ID: resp.ID,
	}, nil
}

func (w *v0ServiceWrapper) CreateEndpoint(ctx context.Context, req *ncproxygrpcv0.CreateEndpointRequest) (_ *ncproxygrpcv0.CreateEndpointResponse, err error) {
	var v1DnsSettings *ncproxygrpc.DnsSetting
	if req.DnsSetting != nil {
		v1DnsSettings = &ncproxygrpc.DnsSetting{
			ServerIpAddrs: req.DnsSetting.ServerIpAddrs,
			Domain:        req.DnsSetting.Domain,
			Search:        req.DnsSetting.Search,
		}
	}
	var v1PortnamePolicySetting *ncproxygrpc.PortNameEndpointPolicySetting
	if req.PortnamePolicySetting != nil {
		v1PortnamePolicySetting = &ncproxygrpc.PortNameEndpointPolicySetting{
			PortName: req.PortnamePolicySetting.PortName,
		}
	}

	var v1IovPolicySettings *ncproxygrpc.IovEndpointPolicySetting
	if req.IovPolicySettings != nil {
		v1IovPolicySettings = &ncproxygrpc.IovEndpointPolicySetting{
			IovOffloadWeight:    req.IovPolicySettings.IovOffloadWeight,
			QueuePairsRequested: req.IovPolicySettings.QueuePairsRequested,
			InterruptModeration: req.IovPolicySettings.InterruptModeration,
		}
	}
	prefixLen, err := strconv.ParseUint(req.IpaddressPrefixlength, 10, 32)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "received invalid ip address prefix length %+v: %v", req, err)
	}
	v1Req := &ncproxygrpc.CreateEndpointRequest{
		EndpointSettings: &ncproxygrpc.EndpointSettings{
			Settings: &ncproxygrpc.EndpointSettings_HcnEndpoint{
				HcnEndpoint: &ncproxygrpc.HcnEndpointSettings{
					Name:                  req.Name,
					Macaddress:            req.Macaddress,
					Ipaddress:             req.Ipaddress,
					IpaddressPrefixlength: uint32(prefixLen),
					NetworkName:           req.NetworkName,
					DnsSetting:            v1DnsSettings,
					Policies: &ncproxygrpc.HcnEndpointPolicies{
						PortnamePolicySetting: v1PortnamePolicySetting,
						IovPolicySettings:     v1IovPolicySettings,
					},
				},
			},
		},
	}
	resp, err := w.s.CreateEndpoint(ctx, v1Req)
	if err != nil {
		return nil, err
	}
	return &ncproxygrpcv0.CreateEndpointResponse{
		ID: resp.ID,
	}, nil
}

func (w *v0ServiceWrapper) AddEndpoint(ctx context.Context, req *ncproxygrpcv0.AddEndpointRequest) (_ *ncproxygrpcv0.AddEndpointResponse, err error) {
	v1Req := &ncproxygrpc.AddEndpointRequest{
		Name:        req.Name,
		NamespaceID: req.NamespaceID,
	}
	_, err = w.s.AddEndpoint(ctx, v1Req)
	if err != nil {
		return nil, err
	}
	return &ncproxygrpcv0.AddEndpointResponse{}, nil
}

func (w *v0ServiceWrapper) DeleteEndpoint(ctx context.Context, req *ncproxygrpcv0.DeleteEndpointRequest) (_ *ncproxygrpcv0.DeleteEndpointResponse, err error) {
	v1Req := &ncproxygrpc.DeleteEndpointRequest{
		Name: req.Name,
	}
	_, err = w.s.DeleteEndpoint(ctx, v1Req)
	if err != nil {
		return nil, err
	}
	return &ncproxygrpcv0.DeleteEndpointResponse{}, nil
}

func (w *v0ServiceWrapper) DeleteNetwork(ctx context.Context, req *ncproxygrpcv0.DeleteNetworkRequest) (_ *ncproxygrpcv0.DeleteNetworkResponse, err error) {
	v1Req := &ncproxygrpc.DeleteNetworkRequest{
		Name: req.Name,
	}
	_, err = w.s.DeleteNetwork(ctx, v1Req)
	if err != nil {
		return nil, err
	}
	return &ncproxygrpcv0.DeleteNetworkResponse{}, nil
}

func (w *v0ServiceWrapper) GetEndpoint(ctx context.Context, req *ncproxygrpcv0.GetEndpointRequest) (_ *ncproxygrpcv0.GetEndpointResponse, err error) {
	v1Req := &ncproxygrpc.GetEndpointRequest{
		Name: req.Name,
	}
	resp, err := w.s.GetEndpoint(ctx, v1Req)
	if err != nil {
		return nil, err
	}
	v0Resp, err := v1EndpointToV0EndpointResp(resp)
	if err != nil {
		if err == errUnsupportedEndpointType {
			return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.Name)
		}
		return nil, err
	}

	return v0Resp, nil
}

func v1EndpointToV0EndpointResp(v1EndpointResp *ncproxygrpc.GetEndpointResponse) (_ *ncproxygrpcv0.GetEndpointResponse, err error) {
	if v1EndpointResp.Endpoint != nil {
		if v1EndpointResp.Endpoint.GetHcnEndpoint() != nil {
			v0Resp := &ncproxygrpcv0.GetEndpointResponse{
				ID:        v1EndpointResp.ID,
				Namespace: v1EndpointResp.Namespace,
			}
			v1EndpointSettings := v1EndpointResp.Endpoint.GetHcnEndpoint()
			v0Resp.Name = v1EndpointSettings.Name
			v0Resp.Network = v1EndpointSettings.NetworkName

			if v1EndpointSettings.DnsSetting != nil {
				v0Resp.DnsSetting = &ncproxygrpcv0.DnsSetting{
					ServerIpAddrs: v1EndpointSettings.DnsSetting.ServerIpAddrs,
					Domain:        v1EndpointSettings.DnsSetting.Domain,
					Search:        v1EndpointSettings.DnsSetting.Search,
				}
			}
			return v0Resp, nil
		}
	}
	return nil, errUnsupportedEndpointType
}

func (w *v0ServiceWrapper) GetEndpoints(ctx context.Context, req *ncproxygrpcv0.GetEndpointsRequest) (_ *ncproxygrpcv0.GetEndpointsResponse, err error) {
	resp, err := w.s.GetEndpoints(ctx, &ncproxygrpc.GetEndpointsRequest{})
	if err != nil {
		return nil, err
	}
	v0Endpoints := make([]*ncproxygrpcv0.GetEndpointResponse, len(resp.Endpoints))
	for i, e := range resp.Endpoints {
		v0Endpoint, err := v1EndpointToV0EndpointResp(e)
		if err != nil {
			if err == errUnsupportedEndpointType {
				// ignore unsupported endpoints
				continue
			}
			return nil, err
		}
		v0Endpoints[i] = v0Endpoint
	}
	return &ncproxygrpcv0.GetEndpointsResponse{
		Endpoints: v0Endpoints,
	}, nil
}

func (w *v0ServiceWrapper) GetNetwork(ctx context.Context, req *ncproxygrpcv0.GetNetworkRequest) (_ *ncproxygrpcv0.GetNetworkResponse, err error) {
	v1Req := &ncproxygrpc.GetNetworkRequest{
		Name: req.Name,
	}
	resp, err := w.s.GetNetwork(ctx, v1Req)
	if err != nil {
		return nil, err
	}

	v0Resp, err := v1NetworkToV0NetworkResp(resp)
	if err != nil {
		if err == errUnsupportedNetworkType {
			return nil, status.Errorf(codes.NotFound, "no network with name `%s` found", req.Name)
		}
		return nil, err
	}

	return v0Resp, nil
}

func v1NetworkToV0NetworkResp(v1NetworkResp *ncproxygrpc.GetNetworkResponse) (_ *ncproxygrpcv0.GetNetworkResponse, err error) {
	if v1NetworkResp.Network != nil {
		if v1NetworkResp.Network.GetHcnNetwork() != nil {
			v0Resp := &ncproxygrpcv0.GetNetworkResponse{
				ID:   v1NetworkResp.ID,
				Name: v1NetworkResp.Network.GetHcnNetwork().Name,
			}
			return v0Resp, nil
		}
	}
	return nil, errUnsupportedNetworkType
}

func (w *v0ServiceWrapper) GetNetworks(ctx context.Context, req *ncproxygrpcv0.GetNetworksRequest) (_ *ncproxygrpcv0.GetNetworksResponse, err error) {
	resp, err := w.s.GetNetworks(ctx, &ncproxygrpc.GetNetworksRequest{})
	if err != nil {
		return nil, err
	}
	v0Networks := make([]*ncproxygrpcv0.GetNetworkResponse, len(resp.Networks))
	for i, n := range resp.Networks {
		v0Network, err := v1NetworkToV0NetworkResp(n)
		if err != nil {
			if err == errUnsupportedNetworkType {
				// ignore unsupported networks
				continue
			}
			return nil, err
		}
		v0Networks[i] = v0Network
	}
	return &ncproxygrpcv0.GetNetworksResponse{
		Networks: v0Networks,
	}, nil
}
