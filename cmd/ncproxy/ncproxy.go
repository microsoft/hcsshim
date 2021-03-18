package main

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/cmd/ncproxy/ncproxygrpc"
	"github.com/Microsoft/hcsshim/cmd/ncproxy/nodenetsvc"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/computeagent"
	"github.com/Microsoft/hcsshim/internal/ncproxyttrpc"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/pkg/octtrpc"
	"github.com/containerd/ttrpc"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPC service exposed for use by a Node Network Service. Holds a mutex for
// updating global client.
type grpcService struct {
}

var _ ncproxygrpc.NetworkConfigProxyServer = &grpcService{}

func (s *grpcService) AddNIC(ctx context.Context, req *ncproxygrpc.AddNICRequest) (_ *ncproxygrpc.AddNICResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "AddNIC")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("containerID", req.ContainerID),
		trace.StringAttribute("endpointName", req.EndpointName),
		trace.StringAttribute("nicID", req.NicID))

	if req.ContainerID == "" || req.EndpointName == "" || req.NicID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}
	if client, ok := containerIDToShim[req.ContainerID]; ok {
		caReq := &computeagent.AddNICInternalRequest{
			ContainerID:  req.ContainerID,
			NicID:        req.NicID,
			EndpointName: req.EndpointName,
		}
		if _, err := client.AddNIC(ctx, caReq); err != nil {
			return nil, err
		}
		return &ncproxygrpc.AddNICResponse{}, nil
	}
	return nil, status.Errorf(codes.FailedPrecondition, "No shim registered for namespace `%s`", req.ContainerID)
}

func (s *grpcService) DeleteNIC(ctx context.Context, req *ncproxygrpc.DeleteNICRequest) (_ *ncproxygrpc.DeleteNICResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "DeleteNIC")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("containerID", req.ContainerID),
		trace.StringAttribute("endpointName", req.EndpointName),
		trace.StringAttribute("nicID", req.NicID))

	if req.ContainerID == "" || req.EndpointName == "" || req.NicID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}
	if client, ok := containerIDToShim[req.ContainerID]; ok {
		caReq := &computeagent.DeleteNICInternalRequest{
			ContainerID:  req.ContainerID,
			NicID:        req.NicID,
			EndpointName: req.EndpointName,
		}
		if _, err := client.DeleteNIC(ctx, caReq); err != nil {
			if err == uvm.ErrNICNotFound || err == uvm.ErrNetNSNotFound {
				return nil, status.Errorf(codes.NotFound, "failed to remove endpoint %q from namespace %q", req.EndpointName, req.NicID)
			}
			return nil, err
		}
		return &ncproxygrpc.DeleteNICResponse{}, nil
	}
	return nil, status.Errorf(codes.FailedPrecondition, "No shim registered for namespace `%s`", req.ContainerID)
}

//
// HNS Methods
//
func (s *grpcService) CreateNetwork(ctx context.Context, req *ncproxygrpc.CreateNetworkRequest) (_ *ncproxygrpc.CreateNetworkResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "CreateNetwork")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("networkName", req.Name),
		trace.StringAttribute("type", req.Mode.String()),
		trace.StringAttribute("ipamType", req.IpamType.String()))

	if req.Name == "" || req.Mode.String() == "" || req.IpamType.String() == "" || req.SwitchName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	// Check if the network already exists, and if so return error.
	_, err = hcn.GetNetworkByName(req.Name)
	if err == nil {
		return nil, status.Errorf(codes.FailedPrecondition, "network with name %q already exists", req.Name)
	}

	// Get the layer ID from the external switch. HNS will create a transparent network for
	// any external switch that is created not through HNS so this is what we're
	// searching for here. If the network exists, the vSwitch with this name exists.
	extSwitch, err := hcn.GetNetworkByName(req.SwitchName)
	if err != nil {
		if _, ok := err.(hcn.NetworkNotFoundError); ok {
			return nil, status.Errorf(codes.NotFound, "no network/switch with name `%s` found", req.SwitchName)
		}
		return nil, errors.Wrapf(err, "failed to get network/switch with name %q", req.SwitchName)
	}

	// Get layer ID and use this as the basis for what to layer the new network over.
	if extSwitch.Health.Extra.LayeredOn == "" {
		return nil, status.Errorf(codes.NotFound, "no layer ID found for network %q found", extSwitch.Id)
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

	subnets := make([]hcn.Subnet, len(req.SubnetIpadressPrefix))
	for i, addrPrefix := range req.SubnetIpadressPrefix {
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
		Policies: []hcn.NetworkPolicy{netPolicy},
		SchemaVersion: hcn.SchemaVersion{
			Major: 2,
			Minor: 2,
		},
	}

	network, err = network.Create()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create HNS network %q", req.Name)
	}

	return &ncproxygrpc.CreateNetworkResponse{
		ID: network.Id,
	}, nil
}

func (s *grpcService) CreateEndpoint(ctx context.Context, req *ncproxygrpc.CreateEndpointRequest) (_ *ncproxygrpc.CreateEndpointResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "CreateEndpoint")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("macAddr", req.Macaddress),
		trace.StringAttribute("endpointName", req.Name),
		trace.StringAttribute("ipAddr", req.Ipaddress),
		trace.StringAttribute("networkName", req.NetworkName))

	if req.Name == "" || req.Ipaddress == "" || req.Macaddress == "" || req.NetworkName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	network, err := hcn.GetNetworkByName(req.NetworkName)
	if err != nil {
		if _, ok := err.(hcn.NetworkNotFoundError); ok {
			return nil, status.Errorf(codes.NotFound, "no network with name `%s` found", req.NetworkName)
		}
		return nil, errors.Wrapf(err, "failed to get network with name %q", req.NetworkName)
	}

	prefixLen, err := strconv.ParseUint(req.IpaddressPrefixlength, 10, 8)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert ip address prefix length to uint")
	}

	// Construct ip config.
	ipConfig := hcn.IpConfig{
		IpAddress:    req.Ipaddress,
		PrefixLength: uint8(prefixLen),
	}

	// Construct the portname policy we'll be setting on the endpoint.
	var portPolicy hcn.PortnameEndpointPolicySetting
	if req.PortnamePolicySetting != nil {
		portPolicy = hcn.PortnameEndpointPolicySetting{
			Name: req.PortnamePolicySetting.PortName,
		}
	}
	portPolicyJSON, err := json.Marshal(portPolicy)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal portname")
	}

	// Construct endpoint policy
	epPolicy := hcn.EndpointPolicy{
		Type:     hcn.EndpointPolicyType(req.PolicyType.String()),
		Settings: portPolicyJSON,
	}

	endpoint := &hcn.HostComputeEndpoint{
		Name:               req.Name,
		HostComputeNetwork: network.Id,
		MacAddress:         req.Macaddress,
		IpConfigurations:   []hcn.IpConfig{ipConfig},
		Policies:           []hcn.EndpointPolicy{epPolicy},
		SchemaVersion: hcn.SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	endpoint, err = endpoint.Create()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HNS endpoint")
	}

	return &ncproxygrpc.CreateEndpointResponse{
		ID: endpoint.Id,
	}, nil
}

func (s *grpcService) AddEndpoint(ctx context.Context, req *ncproxygrpc.AddEndpointRequest) (_ *ncproxygrpc.AddEndpointResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "AddEndpoint")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("endpointName", req.Name),
		trace.StringAttribute("namespaceID", req.NamespaceID))

	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	ep, err := hcn.GetEndpointByName(req.Name)
	if err != nil {
		if _, ok := err.(hcn.EndpointNotFoundError); ok {
			return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.Name)
		}
		return nil, errors.Wrapf(err, "failed to get endpoint with name %q", req.Name)
	}

	if err := hcn.AddNamespaceEndpoint(req.NamespaceID, ep.Id); err != nil {
		return nil, errors.Wrapf(err, "failed to add endpoint with name %q to namespace", req.Name)
	}
	return &ncproxygrpc.AddEndpointResponse{}, nil
}

func (s *grpcService) DeleteEndpoint(ctx context.Context, req *ncproxygrpc.DeleteEndpointRequest) (_ *ncproxygrpc.DeleteEndpointResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "DeleteEndpoint")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("endpointName", req.Name))

	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	ep, err := hcn.GetEndpointByName(req.Name)
	if err != nil {
		if _, ok := err.(hcn.EndpointNotFoundError); ok {
			return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.Name)
		}
		return nil, errors.Wrapf(err, "failed to get endpoint with name %q", req.Name)
	}

	if err = ep.Delete(); err != nil {
		return nil, errors.Wrapf(err, "failed to delete endpoint with name %q", req.Name)
	}
	return &ncproxygrpc.DeleteEndpointResponse{}, nil
}

func (s *grpcService) DeleteNetwork(ctx context.Context, req *ncproxygrpc.DeleteNetworkRequest) (_ *ncproxygrpc.DeleteNetworkResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "DeleteNetwork")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("networkName", req.Name))

	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	network, err := hcn.GetNetworkByName(req.Name)
	if err != nil {
		if _, ok := err.(hcn.NetworkNotFoundError); ok {
			return nil, status.Errorf(codes.NotFound, "no network with name `%s` found", req.Name)
		}
		return nil, errors.Wrapf(err, "failed to get network with name %q", req.Name)
	}

	if err = network.Delete(); err != nil {
		return nil, errors.Wrapf(err, "failed to delete network with name %q", req.Name)
	}
	return &ncproxygrpc.DeleteNetworkResponse{}, nil
}

func (s *grpcService) GetEndpoint(ctx context.Context, req *ncproxygrpc.GetEndpointRequest) (_ *ncproxygrpc.GetEndpointResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "GetEndpoint")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("endpointName", req.Name))

	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	ep, err := hcn.GetEndpointByName(req.Name)
	if err != nil {
		if _, ok := err.(hcn.EndpointNotFoundError); ok {
			return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.Name)
		}
		return nil, errors.Wrapf(err, "failed to get endpoint with name %q", req.Name)
	}

	return &ncproxygrpc.GetEndpointResponse{
		ID:        ep.Id,
		Name:      ep.Name,
		Network:   ep.HostComputeNetwork,
		Namespace: ep.HostComputeNamespace,
	}, nil
}

func (s *grpcService) GetEndpoints(ctx context.Context, req *ncproxygrpc.GetEndpointsRequest) (_ *ncproxygrpc.GetEndpointsResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "GetEndpoints")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	rawEndpoints, err := hcn.ListEndpoints()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get HNS endpoints")
	}

	endpoints := make([]*ncproxygrpc.GetEndpointResponse, len(rawEndpoints))
	for i, endpoint := range rawEndpoints {
		resp := &ncproxygrpc.GetEndpointResponse{
			ID:        endpoint.Id,
			Name:      endpoint.Name,
			Network:   endpoint.HostComputeNetwork,
			Namespace: endpoint.HostComputeNamespace,
		}
		endpoints[i] = resp
	}
	return &ncproxygrpc.GetEndpointsResponse{
		Endpoints: endpoints,
	}, nil
}

func (s *grpcService) GetNetwork(ctx context.Context, req *ncproxygrpc.GetNetworkRequest) (_ *ncproxygrpc.GetNetworkResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "GetNetwork")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("networkName", req.Name))

	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	network, err := hcn.GetNetworkByName(req.Name)
	if err != nil {
		if _, ok := err.(hcn.NetworkNotFoundError); ok {
			return nil, status.Errorf(codes.NotFound, "no network with name `%s` found", req.Name)
		}
		return nil, errors.Wrapf(err, "failed to get network with name %q", req.Name)
	}

	return &ncproxygrpc.GetNetworkResponse{
		ID:   network.Id,
		Name: network.Name,
	}, nil
}

func (s *grpcService) GetNetworks(ctx context.Context, req *ncproxygrpc.GetNetworksRequest) (_ *ncproxygrpc.GetNetworksResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "GetNetworks")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	rawNetworks, err := hcn.ListNetworks()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get HNS networks")
	}

	networks := make([]*ncproxygrpc.GetNetworkResponse, len(rawNetworks))
	for i, network := range rawNetworks {
		resp := &ncproxygrpc.GetNetworkResponse{
			ID:   network.Id,
			Name: network.Name,
		}
		networks[i] = resp
	}

	return &ncproxygrpc.GetNetworksResponse{
		Networks: networks,
	}, nil
}

// TTRPC service exposed for use by the shim. Holds a mutex for updating map of
// client connections.
type ttrpcService struct {
	m sync.Mutex
}

func (s *ttrpcService) RegisterComputeAgent(ctx context.Context, req *ncproxyttrpc.RegisterComputeAgentRequest) (_ *ncproxyttrpc.RegisterComputeAgentResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "RegisterComputeAgent")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("containerID", req.ContainerID),
		trace.StringAttribute("agentAddress", req.AgentAddress))

	conn, err := winio.DialPipe(req.AgentAddress, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to compute agent service")
	}
	client := ttrpc.NewClient(
		conn,
		ttrpc.WithUnaryClientInterceptor(octtrpc.ClientInterceptor()),
		ttrpc.WithOnClose(func() { conn.Close() }),
	)
	// Add to global client map if connection succeeds. Don't check if there's already a map entry
	// just overwrite as the client may have changed the address of the config agent.
	s.m.Lock()
	defer s.m.Unlock()
	containerIDToShim[req.ContainerID] = computeagent.NewComputeAgentClient(client)
	return &ncproxyttrpc.RegisterComputeAgentResponse{}, nil
}

func (s *ttrpcService) ConfigureNetworking(ctx context.Context, req *ncproxyttrpc.ConfigureNetworkingInternalRequest) (_ *ncproxyttrpc.ConfigureNetworkingInternalResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "ConfigureNetworking")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("containerID", req.ContainerID),
		trace.StringAttribute("agentAddress", req.RequestType.String()))

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

	if _, err := nodeNetSvcClient.client.ConfigureNetworking(ctx, netsvcReq); err != nil {
		return nil, err
	}
	return &ncproxyttrpc.ConfigureNetworkingInternalResponse{}, nil
}
