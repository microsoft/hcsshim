//go:build windows

package main

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/log"
	nodenetsvcV0 "github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v0"
	nodenetsvc "github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v1"
)

func (s *v0ServiceWrapper) ConfigureContainerNetworking(ctx context.Context, req *nodenetsvcV0.ConfigureContainerNetworkingRequest) (_ *nodenetsvcV0.ConfigureContainerNetworkingResponse, err error) {
	v1Req := &nodenetsvc.ConfigureContainerNetworkingRequest{
		RequestType:        nodenetsvc.RequestType(req.RequestType),
		ContainerID:        req.ContainerID,
		NetworkNamespaceID: req.NetworkNamespaceID,
	}
	v1Resp, err := s.s.ConfigureContainerNetworking(ctx, v1Req)
	if err != nil {
		return nil, err
	}

	v0Interfaces := make([]*nodenetsvcV0.ContainerNetworkInterface, len(v1Resp.Interfaces))
	for i, v1Interface := range v1Resp.Interfaces {
		v0Interface := &nodenetsvcV0.ContainerNetworkInterface{
			Name:               v1Interface.Name,
			MacAddress:         v1Interface.MacAddress,
			NetworkNamespaceID: v1Interface.NetworkNamespaceID,
		}

		if v1Interface.Ipaddresses != nil {
			ipAddrs := make([]*nodenetsvcV0.ContainerIPAddress, len(v1Interface.Ipaddresses))
			for i, v1IP := range v1Interface.Ipaddresses {
				v0IP := &nodenetsvcV0.ContainerIPAddress{
					Version:        v1IP.Version,
					Ip:             v1IP.Ip,
					PrefixLength:   v1IP.PrefixLength,
					DefaultGateway: v1IP.DefaultGateway,
				}
				ipAddrs[i] = v0IP
			}
			v0Interface.Ipaddresses = ipAddrs
		}
		v0Interfaces[i] = v0Interface
	}
	return &nodenetsvcV0.ConfigureContainerNetworkingResponse{
		Interfaces: v0Interfaces,
	}, nil
}

func (s *v0ServiceWrapper) ConfigureNetworking(ctx context.Context, req *nodenetsvcV0.ConfigureNetworkingRequest) (*nodenetsvcV0.ConfigureNetworkingResponse, error) {
	log.G(ctx).WithField("req", req).Info("ConfigureNetworking request")
	v1Req := &nodenetsvc.ConfigureNetworkingRequest{
		ContainerID: req.ContainerID,
		RequestType: nodenetsvc.RequestType(req.RequestType),
	}
	_, err := s.s.ConfigureNetworking(ctx, v1Req)
	if err != nil {
		return nil, err
	}
	return &nodenetsvcV0.ConfigureNetworkingResponse{}, nil
}

//nolint:stylecheck
func (s *v0ServiceWrapper) GetHostLocalIpAddress(ctx context.Context, req *nodenetsvcV0.GetHostLocalIpAddressRequest) (*nodenetsvcV0.GetHostLocalIpAddressResponse, error) {
	return &nodenetsvcV0.GetHostLocalIpAddressResponse{IpAddr: ""}, nil
}

func (s *v0ServiceWrapper) PingNodeNetworkService(ctx context.Context, req *nodenetsvcV0.PingNodeNetworkServiceRequest) (*nodenetsvcV0.PingNodeNetworkServiceResponse, error) {
	return &nodenetsvcV0.PingNodeNetworkServiceResponse{}, nil
}
