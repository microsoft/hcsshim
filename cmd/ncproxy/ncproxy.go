//go:build windows

package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/containerd/containerd/protobuf"
	"github.com/containerd/ttrpc"
	typeurl "github.com/containerd/typeurl/v2"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/computeagent"
	"github.com/Microsoft/hcsshim/internal/log"
	ncproxynetworking "github.com/Microsoft/hcsshim/internal/ncproxy/networking"
	ncproxystore "github.com/Microsoft/hcsshim/internal/ncproxy/store"
	"github.com/Microsoft/hcsshim/internal/ncproxyttrpc"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/internal/uvm"
	ncproxygrpc "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v1"
	nodenetsvc "github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v1"
	"github.com/Microsoft/hcsshim/pkg/otelttrpc"
)

func init() {
	typeurl.Register(&ncproxynetworking.Endpoint{}, "ncproxy/ncproxynetworking/Endpoint")
	typeurl.Register(&ncproxynetworking.Network{}, "ncproxy/ncproxynetworking/Network")
	typeurl.Register(&hcn.HostComputeEndpoint{}, "ncproxy/hcn/HostComputeEndpoint")
	typeurl.Register(&hcn.HostComputeNetwork{}, "ncproxy/hcn/HostComputeNetwork")
}

// functions for mocking out in tests
var (
	winioDialPipe  = winio.DialPipe
	ttrpcNewClient = ttrpc.NewClient
)

// GRPC service exposed for use by a Node Network Service.
type grpcService struct {
	ncproxygrpc.UnimplementedNetworkConfigProxyServer

	// containerIDToComputeAgent is a cache that stores the mappings from
	// container ID to compute agent address is memory. This is repopulated
	// on reconnect and referenced during client calls.
	containerIDToComputeAgent *computeAgentCache

	// ncproxyNetworking is a database that stores the ncproxy networking networks
	// and endpoints persistently.
	ncpNetworkingStore *ncproxystore.NetworkingStore
}

func newGRPCService(agentCache *computeAgentCache, ncproxyNetworking *ncproxystore.NetworkingStore) *grpcService {
	return &grpcService{
		containerIDToComputeAgent: agentCache,
		ncpNetworkingStore:        ncproxyNetworking,
	}
}

var _ ncproxygrpc.NetworkConfigProxyServer = &grpcService{}

func (s *grpcService) AddNIC(ctx context.Context, req *ncproxygrpc.AddNICRequest) (_ *ncproxygrpc.AddNICResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "AddNIC", trace.WithAttributes(
		attribute.String("containerID", req.ContainerID),
		attribute.String("endpointName", req.EndpointName),
		attribute.String("nicID", req.NicID)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if req.ContainerID == "" || req.EndpointName == "" || req.NicID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	agent, err := s.containerIDToComputeAgent.get(req.ContainerID)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "No shim registered for namespace `%s`", req.ContainerID)
	}

	var anyEndpoint typeurl.Any
	if ep, err := s.ncpNetworkingStore.GetEndpointByName(ctx, req.EndpointName); err == nil {
		if ep.Settings == nil || ep.Settings.DeviceDetails == nil || ep.Settings.DeviceDetails.PCIDeviceDetails == nil {
			return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
		}
		// if there are device details, assign the device via the compute agent
		caReq := &computeagent.AssignPCIInternalRequest{
			ContainerID:          req.ContainerID,
			DeviceID:             ep.Settings.DeviceDetails.PCIDeviceDetails.DeviceID,
			VirtualFunctionIndex: ep.Settings.DeviceDetails.PCIDeviceDetails.VirtualFunctionIndex,
			NicID:                req.NicID,
		}
		if _, err := agent.AssignPCI(ctx, caReq); err != nil {
			return nil, err
		}
		anyEndpoint, err = typeurl.MarshalAny(ep)
		if err != nil {
			return nil, err
		}
	} else {
		if !errors.Is(err, ncproxystore.ErrBucketNotFound) && !errors.Is(err, ncproxystore.ErrKeyNotFound) {
			// log if there was an unexpected error before checking if this is an hcn endpoint
			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
		}
		ep, err := hcn.GetEndpointByName(req.EndpointName)
		if err != nil {
			if _, ok := err.(hcn.EndpointNotFoundError); ok { //nolint:errorlint
				return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.EndpointName)
			}
			return nil, errors.Wrapf(err, "failed to get endpoint with name `%s`", req.EndpointName)
		}

		anyEndpoint, err = typeurl.MarshalAny(ep)
		if err != nil {
			return nil, err
		}

		settings := req.EndpointSettings.GetHcnEndpoint()
		if settings != nil && settings.Policies != nil && settings.Policies.IovPolicySettings != nil {
			log.G(ctx).WithField("iov settings", settings.Policies.IovPolicySettings).Info("AddNIC iov settings")
			iovReqSettings := settings.Policies.IovPolicySettings
			if iovReqSettings.IovOffloadWeight != 0 {
				// IOV policy was set during add nic request, update the hcn endpoint
				hcnIOVSettings := &hcn.IovPolicySetting{
					IovOffloadWeight:    iovReqSettings.IovOffloadWeight,
					QueuePairsRequested: iovReqSettings.QueuePairsRequested,
					InterruptModeration: iovReqSettings.InterruptModeration,
				}
				rawJSON, err := json.Marshal(hcnIOVSettings)
				if err != nil {
					return nil, err
				}

				iovPolicy := hcn.EndpointPolicy{
					Type:     hcn.IOV,
					Settings: rawJSON,
				}
				policies := []hcn.EndpointPolicy{iovPolicy}
				if err := modifyEndpoint(ctx, ep.Id, policies, hcn.RequestTypeUpdate); err != nil {
					return nil, errors.Wrap(err, "failed to add policy to endpoint")
				}
			}
		}
	}

	caReq := &computeagent.AddNICInternalRequest{
		ContainerID: req.ContainerID,
		NicID:       req.NicID,
		Endpoint:    protobuf.FromAny(anyEndpoint),
	}
	if _, err := agent.AddNIC(ctx, caReq); err != nil {
		return nil, err
	}
	return &ncproxygrpc.AddNICResponse{}, nil
}

func (s *grpcService) ModifyNIC(ctx context.Context, req *ncproxygrpc.ModifyNICRequest) (_ *ncproxygrpc.ModifyNICResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "ModifyNIC", trace.WithAttributes(
		attribute.String("containerID", req.ContainerID),
		attribute.String("endpointName", req.EndpointName),
		attribute.String("nicID", req.NicID)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if req.ContainerID == "" || req.EndpointName == "" || req.NicID == "" || req.EndpointSettings == nil {
		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
	}

	if _, err := s.ncpNetworkingStore.GetEndpointByName(ctx, req.EndpointName); err == nil {
		return nil, status.Errorf(codes.Unimplemented, "cannot modify custom endpoints: %v", req)
	}

	ep, err := hcn.GetEndpointByName(req.EndpointName)
	if err != nil {
		if _, ok := err.(hcn.EndpointNotFoundError); ok { //nolint:errorlint
			return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.EndpointName)
		}
		return nil, errors.Wrapf(err, "failed to get endpoint with name `%s`", req.EndpointName)
	}

	anyEndpoint, err := typeurl.MarshalAny(ep)
	if err != nil {
		return nil, err
	}

	agent, err := s.containerIDToComputeAgent.get(req.ContainerID)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "No shim registered for containerID `%s`", req.ContainerID)
	}
	settings := req.EndpointSettings.GetHcnEndpoint()
	if settings.Policies == nil || settings.Policies.IovPolicySettings == nil {
		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
	}
	log.G(ctx).WithField("iov settings", settings.Policies.IovPolicySettings).Info("ModifyNIC iov settings")

	iovReqSettings := settings.Policies.IovPolicySettings
	caReq := &computeagent.ModifyNICInternalRequest{
		NicID:    req.NicID,
		Endpoint: protobuf.FromAny(anyEndpoint),
		IovPolicySettings: &computeagent.IovSettings{
			IovOffloadWeight:    iovReqSettings.IovOffloadWeight,
			QueuePairsRequested: iovReqSettings.QueuePairsRequested,
			InterruptModeration: iovReqSettings.InterruptModeration,
		},
	}

	hcnIOVSettings := &hcn.IovPolicySetting{
		IovOffloadWeight:    iovReqSettings.IovOffloadWeight,
		QueuePairsRequested: iovReqSettings.QueuePairsRequested,
		InterruptModeration: iovReqSettings.InterruptModeration,
	}
	rawJSON, err := json.Marshal(hcnIOVSettings)
	if err != nil {
		return nil, err
	}

	iovPolicy := hcn.EndpointPolicy{
		Type:     hcn.IOV,
		Settings: rawJSON,
	}
	policies := []hcn.EndpointPolicy{iovPolicy}

	// To turn off iov offload on an endpoint, we need to first call into HCS to change the
	// offload weight and then call into HNS to revoke the policy.
	//
	// To turn on iov offload, the reverse order is used.
	if iovReqSettings.IovOffloadWeight == 0 {
		if _, err := agent.ModifyNIC(ctx, caReq); err != nil {
			return nil, err
		}
		if err := modifyEndpoint(ctx, ep.Id, policies, hcn.RequestTypeUpdate); err != nil {
			return nil, errors.Wrap(err, "failed to modify network adapter")
		}
		if err := modifyEndpoint(ctx, ep.Id, policies, hcn.RequestTypeRemove); err != nil {
			return nil, errors.Wrap(err, "failed to modify network adapter")
		}
	} else {
		if err := modifyEndpoint(ctx, ep.Id, policies, hcn.RequestTypeUpdate); err != nil {
			return nil, errors.Wrap(err, "failed to modify network adapter")
		}
		if _, err := agent.ModifyNIC(ctx, caReq); err != nil {
			return nil, err
		}
	}

	return &ncproxygrpc.ModifyNICResponse{}, nil
}

func (s *grpcService) DeleteNIC(ctx context.Context, req *ncproxygrpc.DeleteNICRequest) (_ *ncproxygrpc.DeleteNICResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "DeleteNIC", trace.WithAttributes(
		attribute.String("containerID", req.ContainerID),
		attribute.String("endpointName", req.EndpointName),
		attribute.String("nicID", req.NicID)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if req.ContainerID == "" || req.EndpointName == "" || req.NicID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	var anyEndpoint typeurl.Any
	if endpt, err := s.ncpNetworkingStore.GetEndpointByName(ctx, req.EndpointName); err == nil {
		anyEndpoint, err = typeurl.MarshalAny(endpt)
		if err != nil {
			return nil, err
		}
	} else {
		if !errors.Is(err, ncproxystore.ErrBucketNotFound) && !errors.Is(err, ncproxystore.ErrKeyNotFound) {
			// log if there was an unexpected error before checking if this is an hcn endpoint
			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
		}
		ep, err := hcn.GetEndpointByName(req.EndpointName)
		if err != nil {
			if _, ok := err.(hcn.EndpointNotFoundError); ok { //nolint:errorlint
				return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.EndpointName)
			}
			return nil, errors.Wrapf(err, "failed to get endpoint with name `%s`", req.EndpointName)
		}
		anyEndpoint, err = typeurl.MarshalAny(ep)
		if err != nil {
			return nil, err
		}
	}
	agent, err := s.containerIDToComputeAgent.get(req.ContainerID)
	if err == nil {
		caReq := &computeagent.DeleteNICInternalRequest{
			ContainerID: req.ContainerID,
			NicID:       req.NicID,
			Endpoint:    protobuf.FromAny(anyEndpoint),
		}
		if _, err := agent.DeleteNIC(ctx, caReq); err != nil {
			if errors.Is(err, uvm.ErrNICNotFound) || errors.Is(err, uvm.ErrNetNSNotFound) {
				return nil, status.Errorf(codes.NotFound, "failed to remove endpoint %q from namespace %q", req.EndpointName, req.NicID)
			}
			return nil, err
		}
		return &ncproxygrpc.DeleteNICResponse{}, nil
	}
	return nil, status.Errorf(codes.FailedPrecondition, "No shim registered for namespace `%s`", req.ContainerID)
}

func (s *grpcService) CreateNetwork(ctx context.Context, req *ncproxygrpc.CreateNetworkRequest) (_ *ncproxygrpc.CreateNetworkResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "CreateNetwork")
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if req.Network == nil || req.Network.GetSettings() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	switch req.Network.GetSettings().(type) {
	case *ncproxygrpc.Network_HcnNetwork:
		networkReq := req.Network.GetHcnNetwork()
		if networkReq.Name == "" {
			return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
		}
		span.SetAttributes(
			attribute.String("networkName", networkReq.Name),
			attribute.String("type", networkReq.Mode.String()),
			attribute.String("ipamType", networkReq.IpamType.String()))

		network, err := createHCNNetwork(ctx, networkReq)
		if err != nil {
			return nil, err
		}
		return &ncproxygrpc.CreateNetworkResponse{
			ID: network.Id,
		}, nil
	case *ncproxygrpc.Network_NcproxyNetwork:
		settings := req.Network.GetNcproxyNetwork()
		if settings.Name == "" {
			return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
		}
		networkSettings := &ncproxynetworking.NetworkSettings{
			Name: settings.Name,
		}
		network := &ncproxynetworking.Network{
			NetworkName: settings.Name,
			Settings:    networkSettings,
		}
		if err := s.ncpNetworkingStore.CreateNetwork(ctx, network); err != nil {
			return nil, err
		}
		return &ncproxygrpc.CreateNetworkResponse{
			ID: settings.Name,
		}, nil
	}

	return nil, status.Errorf(codes.InvalidArgument, "invalid network settings type: %+v", req.Network.Settings)
}

func (s *grpcService) CreateEndpoint(ctx context.Context, req *ncproxygrpc.CreateEndpointRequest) (_ *ncproxygrpc.CreateEndpointResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "CreateEndpoint")
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if req.EndpointSettings == nil {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	switch req.EndpointSettings.GetSettings().(type) {
	case *ncproxygrpc.EndpointSettings_HcnEndpoint:
		reqEndpoint := req.EndpointSettings.GetHcnEndpoint()

		span.SetAttributes(
			attribute.String("macAddr", reqEndpoint.Macaddress),
			attribute.String("endpointName", reqEndpoint.Name),
			attribute.String("ipAddr", reqEndpoint.Ipaddress),
			attribute.String("networkName", reqEndpoint.NetworkName))

		if reqEndpoint.Name == "" || reqEndpoint.Ipaddress == "" || reqEndpoint.Macaddress == "" || reqEndpoint.NetworkName == "" {
			return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
		}

		network, err := hcn.GetNetworkByName(reqEndpoint.NetworkName)
		if err != nil {
			if _, ok := err.(hcn.NetworkNotFoundError); ok { //nolint:errorlint
				return nil, status.Errorf(codes.NotFound, "no network with name `%s` found", reqEndpoint.NetworkName)
			}
			return nil, errors.Wrapf(err, "failed to get network with name %q", reqEndpoint.NetworkName)
		}
		ep, err := createHCNEndpoint(ctx, network, reqEndpoint)
		if err != nil {
			return nil, err
		}
		return &ncproxygrpc.CreateEndpointResponse{
			ID: ep.Id,
		}, nil
	case *ncproxygrpc.EndpointSettings_NcproxyEndpoint:
		// get the network stored, create endpoint data and store
		reqEndpoint := req.EndpointSettings.GetNcproxyEndpoint()
		if reqEndpoint.Name == "" || reqEndpoint.Ipaddress == "" || reqEndpoint.Macaddress == "" || reqEndpoint.NetworkName == "" || reqEndpoint.DeviceDetails == nil {
			return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
		}

		network, err := s.ncpNetworkingStore.GetNetworkByName(ctx, reqEndpoint.NetworkName)
		if err != nil || network == nil {
			return nil, errors.Wrapf(err, "network %v does not exist", reqEndpoint.NetworkName)
		}
		epSettings := &ncproxynetworking.EndpointSettings{
			Name:                  reqEndpoint.Name,
			Macaddress:            reqEndpoint.Macaddress,
			IPAddress:             reqEndpoint.Ipaddress,
			IPAddressPrefixLength: reqEndpoint.IpaddressPrefixlength,
			NetworkName:           reqEndpoint.NetworkName,
			DefaultGateway:        reqEndpoint.DefaultGateway,
			DeviceDetails: &ncproxynetworking.DeviceDetails{
				PCIDeviceDetails: &ncproxynetworking.PCIDeviceDetails{
					DeviceID:             reqEndpoint.GetPciDeviceDetails().DeviceID,
					VirtualFunctionIndex: reqEndpoint.GetPciDeviceDetails().VirtualFunctionIndex,
				},
			},
		}
		ep := &ncproxynetworking.Endpoint{
			EndpointName: reqEndpoint.Name,
			Settings:     epSettings,
		}
		if err := s.ncpNetworkingStore.CreatEndpoint(ctx, ep); err != nil {
			return nil, err
		}
		return &ncproxygrpc.CreateEndpointResponse{
			ID: reqEndpoint.Name,
		}, nil
	}

	return nil, status.Errorf(codes.InvalidArgument, "invalid endpoint settings type: %+v", req.EndpointSettings.GetSettings())
}

func (s *grpcService) AddEndpoint(ctx context.Context, req *ncproxygrpc.AddEndpointRequest) (_ *ncproxygrpc.AddEndpointResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "AddEndpoint", trace.WithAttributes(
		attribute.String("endpointName", req.Name),
		attribute.String("namespaceID", req.NamespaceID)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if req.Name == "" || (!req.AttachToHost && req.NamespaceID == "") {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	if endpt, err := s.ncpNetworkingStore.GetEndpointByName(ctx, req.Name); err == nil {
		endpt.NamespaceID = req.NamespaceID
		if err := s.ncpNetworkingStore.UpdateEndpoint(ctx, endpt); err != nil {
			return nil, errors.Wrapf(err, "failed to update endpoint with name `%s`", req.Name)
		}
	} else {
		if !errors.Is(err, ncproxystore.ErrBucketNotFound) && !errors.Is(err, ncproxystore.ErrKeyNotFound) {
			// log if there was an unexpected error before checking if this is an hcn endpoint
			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
		}
		ep, err := hcn.GetEndpointByName(req.Name)
		if err != nil {
			if _, ok := err.(hcn.EndpointNotFoundError); ok { //nolint:errorlint
				return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.Name)
			}
			return nil, errors.Wrapf(err, "failed to get endpoint with name `%s`", req.Name)
		}
		if req.AttachToHost {
			if req.NamespaceID != "" {
				log.G(ctx).WithField("namespaceID", req.NamespaceID).
					Warning("Specified namespace ID will be ignored when attaching to default host namespace")
			}

			nsID, err := getHostDefaultNamespace()
			if err != nil {
				return nil, err
			}

			req.NamespaceID = nsID
			log.G(ctx).WithField("namespaceID", req.NamespaceID).Debug("Attaching endpoint to default host namespace")
			// replace current span namespaceID attribute
			span.SetAttributes(attribute.String("namespaceID", req.NamespaceID))
		}
		if err := hcn.AddNamespaceEndpoint(req.NamespaceID, ep.Id); err != nil {
			return nil, errors.Wrapf(err, "failed to add endpoint with name %q to namespace", req.Name)
		}
	}

	return &ncproxygrpc.AddEndpointResponse{}, nil
}

func (s *grpcService) DeleteEndpoint(ctx context.Context, req *ncproxygrpc.DeleteEndpointRequest) (_ *ncproxygrpc.DeleteEndpointResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "DeleteEndpoint", trace.WithAttributes(
		attribute.String("endpointName", req.Name)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	if _, err := s.ncpNetworkingStore.GetEndpointByName(ctx, req.Name); err == nil {
		if err := s.ncpNetworkingStore.DeleteEndpoint(ctx, req.Name); err != nil {
			return nil, errors.Wrapf(err, "failed to delete endpoint with name %q", req.Name)
		}
	} else {
		if !errors.Is(err, ncproxystore.ErrBucketNotFound) && !errors.Is(err, ncproxystore.ErrKeyNotFound) {
			// log if there was an unexpected error before checking if this is an hcn endpoint
			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
		}
		ep, err := hcn.GetEndpointByName(req.Name)
		if err != nil {
			if _, ok := err.(hcn.EndpointNotFoundError); ok { //nolint:errorlint
				return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.Name)
			}
			return nil, errors.Wrapf(err, "failed to get endpoint with name %q", req.Name)
		}

		if err = ep.Delete(); err != nil {
			return nil, errors.Wrapf(err, "failed to delete endpoint with name %q", req.Name)
		}
	}
	return &ncproxygrpc.DeleteEndpointResponse{}, nil
}

func (s *grpcService) DeleteNetwork(ctx context.Context, req *ncproxygrpc.DeleteNetworkRequest) (_ *ncproxygrpc.DeleteNetworkResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "DeleteNetwork", trace.WithAttributes(
		attribute.String("networkName", req.Name)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	if _, err := s.ncpNetworkingStore.GetNetworkByName(ctx, req.Name); err == nil {
		if err := s.ncpNetworkingStore.DeleteNetwork(ctx, req.Name); err != nil {
			return nil, errors.Wrapf(err, "failed to delete network with name %q", req.Name)
		}
	} else {
		if !errors.Is(err, ncproxystore.ErrBucketNotFound) && !errors.Is(err, ncproxystore.ErrKeyNotFound) {
			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
		}
		network, err := hcn.GetNetworkByName(req.Name)
		if err != nil {
			if _, ok := err.(hcn.NetworkNotFoundError); ok { //nolint:errorlint
				return nil, status.Errorf(codes.NotFound, "no network with name `%s` found", req.Name)
			}
			return nil, errors.Wrapf(err, "failed to get network with name %q", req.Name)
		}

		if err = network.Delete(); err != nil {
			return nil, errors.Wrapf(err, "failed to delete network with name %q", req.Name)
		}
	}

	return &ncproxygrpc.DeleteNetworkResponse{}, nil
}

func ncpNetworkingEndpointToEndpointResponse(ep *ncproxynetworking.Endpoint) (_ *ncproxygrpc.GetEndpointResponse, err error) {
	result := &ncproxygrpc.GetEndpointResponse{
		Namespace: ep.NamespaceID,
		ID:        ep.EndpointName,
	}
	if ep.Settings == nil {
		return result, nil
	}

	deviceDetails := &ncproxygrpc.NCProxyEndpointSettings_PciDeviceDetails{}
	if ep.Settings.DeviceDetails != nil && ep.Settings.DeviceDetails.PCIDeviceDetails != nil {
		deviceDetails.PciDeviceDetails = &ncproxygrpc.PCIDeviceDetails{
			DeviceID:             ep.Settings.DeviceDetails.PCIDeviceDetails.DeviceID,
			VirtualFunctionIndex: ep.Settings.DeviceDetails.PCIDeviceDetails.VirtualFunctionIndex,
		}
	}

	result.Endpoint = &ncproxygrpc.EndpointSettings{
		Settings: &ncproxygrpc.EndpointSettings_NcproxyEndpoint{
			NcproxyEndpoint: &ncproxygrpc.NCProxyEndpointSettings{
				Name:                  ep.EndpointName,
				Macaddress:            ep.Settings.Macaddress,
				Ipaddress:             ep.Settings.IPAddress,
				IpaddressPrefixlength: ep.Settings.IPAddressPrefixLength,
				NetworkName:           ep.Settings.NetworkName,
				DefaultGateway:        ep.Settings.DefaultGateway,
				DeviceDetails:         deviceDetails,
			},
		},
	}
	return result, nil
}

func (s *grpcService) GetEndpoint(ctx context.Context, req *ncproxygrpc.GetEndpointRequest) (_ *ncproxygrpc.GetEndpointResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "GetEndpoint", trace.WithAttributes(
		attribute.String("endpointName", req.Name)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	if ep, err := s.ncpNetworkingStore.GetEndpointByName(ctx, req.Name); err == nil {
		return ncpNetworkingEndpointToEndpointResponse(ep)
	} else if !errors.Is(err, ncproxystore.ErrBucketNotFound) && !errors.Is(err, ncproxystore.ErrKeyNotFound) {
		log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
	}

	ep, err := hcn.GetEndpointByName(req.Name)
	if err != nil {
		if _, ok := err.(hcn.EndpointNotFoundError); ok { //nolint:errorlint
			return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.Name)
		}
		return nil, errors.Wrapf(err, "failed to get endpoint with name %q", req.Name)
	}
	return hcnEndpointToEndpointResponse(ep)
}

func (s *grpcService) GetEndpoints(ctx context.Context, req *ncproxygrpc.GetEndpointsRequest) (_ *ncproxygrpc.GetEndpointsResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "GetEndpoints")
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	endpoints := []*ncproxygrpc.GetEndpointResponse{}

	rawHCNEndpoints, err := hcn.ListEndpoints()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get HNS endpoints")
	}

	rawNCProxyEndpoints, err := s.ncpNetworkingStore.ListEndpoints(ctx)
	if err != nil && !errors.Is(err, ncproxystore.ErrBucketNotFound) {
		return nil, errors.Wrap(err, "failed to get ncproxy networking endpoints")
	}

	for _, endpoint := range rawHCNEndpoints {
		e, err := hcnEndpointToEndpointResponse(&endpoint)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, e)
	}

	for _, endpoint := range rawNCProxyEndpoints {
		e, err := ncpNetworkingEndpointToEndpointResponse(endpoint)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, e)
	}

	return &ncproxygrpc.GetEndpointsResponse{
		Endpoints: endpoints,
	}, nil
}

func ncpNetworkingNetworkToNetworkResponse(network *ncproxynetworking.Network) (*ncproxygrpc.GetNetworkResponse, error) {
	return &ncproxygrpc.GetNetworkResponse{
		ID: network.NetworkName,
		Network: &ncproxygrpc.Network{
			Settings: &ncproxygrpc.Network_NcproxyNetwork{
				NcproxyNetwork: &ncproxygrpc.NCProxyNetworkSettings{
					Name: network.Settings.Name,
				},
			},
		},
	}, nil
}

func (s *grpcService) GetNetwork(ctx context.Context, req *ncproxygrpc.GetNetworkRequest) (_ *ncproxygrpc.GetNetworkResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "GetNetwork", trace.WithAttributes(
		attribute.String("networkName", req.Name)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	if network, err := s.ncpNetworkingStore.GetNetworkByName(ctx, req.Name); err == nil {
		return ncpNetworkingNetworkToNetworkResponse(network)
	} else if !errors.Is(err, ncproxystore.ErrBucketNotFound) && !errors.Is(err, ncproxystore.ErrKeyNotFound) {
		log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
	}

	network, err := hcn.GetNetworkByName(req.Name)
	if err != nil {
		if _, ok := err.(hcn.NetworkNotFoundError); ok { //nolint:errorlint
			return nil, status.Errorf(codes.NotFound, "no network with name `%s` found", req.Name)
		}
		return nil, errors.Wrapf(err, "failed to get network with name %q", req.Name)
	}

	return hcnNetworkToNetworkResponse(ctx, network)
}

func (s *grpcService) GetNetworks(ctx context.Context, req *ncproxygrpc.GetNetworksRequest) (_ *ncproxygrpc.GetNetworksResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "GetNetworks")
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	networks := []*ncproxygrpc.GetNetworkResponse{}

	rawHCNNetworks, err := hcn.ListNetworks()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get HNS networks")
	}

	rawNCProxyNetworks, err := s.ncpNetworkingStore.ListNetworks(ctx)
	if err != nil && !errors.Is(err, ncproxystore.ErrBucketNotFound) {
		return nil, errors.Wrap(err, "failed to get ncproxy networking networks")
	}

	for _, network := range rawHCNNetworks {
		n, err := hcnNetworkToNetworkResponse(ctx, &network)
		if err != nil {
			return nil, err
		}
		networks = append(networks, n)
	}

	for _, network := range rawNCProxyNetworks {
		n, err := ncpNetworkingNetworkToNetworkResponse(network)
		if err != nil {
			return nil, err
		}
		networks = append(networks, n)
	}

	return &ncproxygrpc.GetNetworksResponse{
		Networks: networks,
	}, nil
}

// TTRPC service exposed for use by the shim.
type ttrpcService struct {
	// containerIDToComputeAgent is a cache that stores the mappings from
	// container ID to compute agent address is memory. This is repopulated
	// on reconnect and referenced during client calls.
	containerIDToComputeAgent *computeAgentCache
	// agentStore refers to the database that stores the mappings from
	// containerID to compute agent address persistently. This is referenced
	// on reconnect and when registering/unregistering a compute agent.
	agentStore *ncproxystore.ComputeAgentStore
}

func newTTRPCService(ctx context.Context, agent *computeAgentCache, agentStore *ncproxystore.ComputeAgentStore) *ttrpcService {
	return &ttrpcService{
		containerIDToComputeAgent: agent,
		agentStore:                agentStore,
	}
}

func getComputeAgentClient(agentAddr string) (*computeAgentClient, error) {
	conn, err := winioDialPipe(agentAddr, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to compute agent service")
	}
	raw := ttrpcNewClient(
		conn,
		ttrpc.WithUnaryClientInterceptor(otelttrpc.ClientInterceptor()),
		ttrpc.WithOnClose(func() { conn.Close() }),
	)
	return &computeAgentClient{raw, computeagent.NewComputeAgentClient(raw)}, nil
}

func (s *ttrpcService) RegisterComputeAgent(ctx context.Context, req *ncproxyttrpc.RegisterComputeAgentRequest) (_ *ncproxyttrpc.RegisterComputeAgentResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "RegisterComputeAgent", trace.WithAttributes(
		attribute.String("containerID", req.ContainerID),
		attribute.String("agentAddress", req.AgentAddress)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	agent, err := getComputeAgentClient(req.AgentAddress)
	if err != nil {
		return nil, err
	}

	if err := s.agentStore.UpdateComputeAgent(ctx, req.ContainerID, req.AgentAddress); err != nil {
		return nil, err
	}

	// Add to client cache if connection succeeds. Don't check if there's already a map entry
	// just overwrite as the client may have changed the address of the config agent.
	if err := s.containerIDToComputeAgent.put(req.ContainerID, agent); err != nil {
		return nil, err
	}

	return &ncproxyttrpc.RegisterComputeAgentResponse{}, nil
}

func (s *ttrpcService) UnregisterComputeAgent(ctx context.Context, req *ncproxyttrpc.UnregisterComputeAgentRequest) (_ *ncproxyttrpc.UnregisterComputeAgentResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "UnregisterComputeAgent", trace.WithAttributes(
		attribute.String("containerID", req.ContainerID)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	err = s.agentStore.DeleteComputeAgent(ctx, req.ContainerID)
	if err != nil {
		log.G(ctx).WithField("key", req.ContainerID).WithError(err).Warn("failed to delete key from compute agent store")
	}

	// remove the agent from the cache and return it so we can clean up its resources as well
	agent, err := s.containerIDToComputeAgent.getAndDelete(req.ContainerID)
	if err != nil {
		return nil, err
	}
	if agent != nil {
		if err := agent.Close(); err != nil {
			return nil, err
		}
	}

	return &ncproxyttrpc.UnregisterComputeAgentResponse{}, nil
}

func (s *ttrpcService) ConfigureNetworking(ctx context.Context, req *ncproxyttrpc.ConfigureNetworkingInternalRequest) (_ *ncproxyttrpc.ConfigureNetworkingInternalResponse, err error) {
	ctx, span := otelutil.StartSpan(ctx, "ConfigureNetworking", trace.WithAttributes(
		attribute.String("containerID", req.ContainerID),
		attribute.String("agentAddress", req.RequestType.String())))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	if req.ContainerID == "" {
		return nil, status.Error(codes.InvalidArgument, "ContainerID is empty")
	}

	if nodeNetSvcClient == nil {
		return nil, status.Error(codes.FailedPrecondition, "No NodeNetworkService client registered")
	}

	switch req.RequestType {
	case ncproxyttrpc.RequestTypeInternal_Setup:
	case ncproxyttrpc.RequestTypeInternal_Teardown:
	default:
		return nil, status.Errorf(codes.InvalidArgument, "Request type %d is not known", req.RequestType)
	}

	netsvcReq := &nodenetsvc.ConfigureNetworkingRequest{
		ContainerID: req.ContainerID,
		RequestType: nodenetsvc.RequestType(req.RequestType),
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if _, err := nodeNetSvcClient.ConfigureNetworking(ctx, netsvcReq); err != nil {
		return nil, err
	}
	return &ncproxyttrpc.ConfigureNetworkingInternalResponse{}, nil
}
