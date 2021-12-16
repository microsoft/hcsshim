package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/computeagent"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ncproxyttrpc"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/uvm"
	ncproxygrpc "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v1"
	nodenetsvc "github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v1"
	"github.com/Microsoft/hcsshim/pkg/octtrpc"
	"github.com/containerd/ttrpc"
	"github.com/containerd/typeurl"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func init() {
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
	// containerIDToComputeAgent is a cache that stores the mappings from
	// container ID to compute agent address is memory. This is repopulated
	// on reconnect and referenced during client calls.
	containerIDToComputeAgent *computeAgentCache
}

func newGRPCService(agentCache *computeAgentCache) *grpcService {
	return &grpcService{
		containerIDToComputeAgent: agentCache,
	}
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

	ep, err := hcn.GetEndpointByName(req.EndpointName)
	if err != nil {
		if _, ok := err.(hcn.EndpointNotFoundError); ok {
			return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.EndpointName)
		}
		return nil, errors.Wrapf(err, "failed to get endpoint with name `%s`", req.EndpointName)
	}

	anyEndpoint, err := typeurl.MarshalAny(ep)
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
				return nil, errors.Wrap(err, "failed to add policy to endpointf")
			}
		}
	}

	agent, err := s.containerIDToComputeAgent.get(req.ContainerID)
	if err == nil {
		caReq := &computeagent.AddNICInternalRequest{
			ContainerID: req.ContainerID,
			NicID:       req.NicID,
			Endpoint:    anyEndpoint,
		}
		if _, err := agent.AddNIC(ctx, caReq); err != nil {
			return nil, err
		}
		return &ncproxygrpc.AddNICResponse{}, nil
	}
	return nil, status.Errorf(codes.FailedPrecondition, "No shim registered for namespace `%s`", req.ContainerID)
}

func (s *grpcService) ModifyNIC(ctx context.Context, req *ncproxygrpc.ModifyNICRequest) (_ *ncproxygrpc.ModifyNICResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "ModifyNIC")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("containerID", req.ContainerID),
		trace.StringAttribute("endpointName", req.EndpointName),
		trace.StringAttribute("nicID", req.NicID))

	if req.ContainerID == "" || req.EndpointName == "" || req.NicID == "" || req.EndpointSettings == nil {
		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
	}

	ep, err := hcn.GetEndpointByName(req.EndpointName)
	if err != nil {
		if _, ok := err.(hcn.EndpointNotFoundError); ok {
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
		Endpoint: anyEndpoint,
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

	ep, err := hcn.GetEndpointByName(req.EndpointName)
	if err != nil {
		if _, ok := err.(hcn.EndpointNotFoundError); ok {
			return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.EndpointName)
		}
		return nil, errors.Wrapf(err, "failed to get endpoint with name `%s`", req.EndpointName)
	}
	anyEndpoint, err := typeurl.MarshalAny(ep)
	if err != nil {
		return nil, err
	}
	agent, err := s.containerIDToComputeAgent.get(req.ContainerID)
	if err == nil {
		caReq := &computeagent.DeleteNICInternalRequest{
			ContainerID: req.ContainerID,
			NicID:       req.NicID,
			Endpoint:    anyEndpoint,
		}
		if _, err := agent.DeleteNIC(ctx, caReq); err != nil {
			if err == uvm.ErrNICNotFound || err == uvm.ErrNetNSNotFound {
				return nil, status.Errorf(codes.NotFound, "failed to remove endpoint %q from namespace %q", req.EndpointName, req.NicID)
			}
			return nil, err
		}
		return &ncproxygrpc.DeleteNICResponse{}, nil
	}
	return nil, status.Errorf(codes.FailedPrecondition, "No shim registered for namespace `%s`", req.ContainerID)
}

func (s *grpcService) CreateNetwork(ctx context.Context, req *ncproxygrpc.CreateNetworkRequest) (_ *ncproxygrpc.CreateNetworkResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "CreateNetwork") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	if req.Network == nil || req.Network.GetSettings() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	switch req.Network.GetSettings().(type) {
	case *ncproxygrpc.Network_HcnNetwork:
		networkReq := req.Network.GetHcnNetwork()
		if networkReq.Name == "" {
			return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
		}
		span.AddAttributes(
			trace.StringAttribute("networkName", networkReq.Name),
			trace.StringAttribute("type", networkReq.Mode.String()),
			trace.StringAttribute("ipamType", networkReq.IpamType.String()))

		network, err := createHCNNetwork(ctx, networkReq)
		if err != nil {
			return nil, err
		}
		return &ncproxygrpc.CreateNetworkResponse{
			ID: network.Id,
		}, nil
	case *ncproxygrpc.Network_NcproxyNetwork:
		return nil, status.Error(codes.Unimplemented, "ncproxy network is no implemented yet")
	}

	return nil, status.Errorf(codes.InvalidArgument, "invalid network settings type: %+v", req.Network.Settings)
}

func (s *grpcService) CreateEndpoint(ctx context.Context, req *ncproxygrpc.CreateEndpointRequest) (_ *ncproxygrpc.CreateEndpointResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "CreateEndpoint") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	if req.EndpointSettings == nil {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	switch req.EndpointSettings.GetSettings().(type) {
	case *ncproxygrpc.EndpointSettings_HcnEndpoint:
		reqEndpoint := req.EndpointSettings.GetHcnEndpoint()

		span.AddAttributes(
			trace.StringAttribute("macAddr", reqEndpoint.Macaddress),
			trace.StringAttribute("endpointName", reqEndpoint.Name),
			trace.StringAttribute("ipAddr", reqEndpoint.Ipaddress),
			trace.StringAttribute("network", reqEndpoint.NetworkName))

		if reqEndpoint.Name == "" || reqEndpoint.Ipaddress == "" || reqEndpoint.Macaddress == "" || reqEndpoint.NetworkName == "" {
			return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
		}

		network, err := hcn.GetNetworkByName(reqEndpoint.NetworkName)
		if err != nil {
			if _, ok := err.(hcn.NetworkNotFoundError); ok {
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
		return nil, status.Error(codes.Unimplemented, "ncproxy endpoint is not implemented yet")
	}

	return nil, status.Errorf(codes.InvalidArgument, "invalid endpoint settings type: %+v", req.EndpointSettings.GetSettings())
}

func (s *grpcService) AddEndpoint(ctx context.Context, req *ncproxygrpc.AddEndpointRequest) (_ *ncproxygrpc.AddEndpointResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "AddEndpoint") //nolint:ineffassign,staticcheck
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
		return nil, errors.Wrapf(err, "failed to get endpoint with name `%s`", req.Name)
	}
	if err := hcn.AddNamespaceEndpoint(req.NamespaceID, ep.Id); err != nil {
		return nil, errors.Wrapf(err, "failed to add endpoint with name %q to namespace", req.Name)
	}

	return &ncproxygrpc.AddEndpointResponse{}, nil
}

func (s *grpcService) DeleteEndpoint(ctx context.Context, req *ncproxygrpc.DeleteEndpointRequest) (_ *ncproxygrpc.DeleteEndpointResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "DeleteEndpoint") //nolint:ineffassign,staticcheck
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
	ctx, span := trace.StartSpan(ctx, "DeleteNetwork") //nolint:ineffassign,staticcheck
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
	ctx, span := trace.StartSpan(ctx, "GetEndpoint") //nolint:ineffassign,staticcheck
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
	policies, err := parseEndpointPolicies(ep.Policies)
	if err != nil {
		return nil, err
	}
	ipConfigInfo := ep.IpConfigurations
	if len(ipConfigInfo) == 0 {
		return nil, errors.Errorf("failed to find network %v ip configuration information", req.Name)
	}

	return &ncproxygrpc.GetEndpointResponse{
		Namespace: ep.HostComputeNamespace,
		ID:        ep.Id,
		Endpoint: &ncproxygrpc.EndpointSettings{
			Settings: &ncproxygrpc.EndpointSettings_HcnEndpoint{
				HcnEndpoint: &ncproxygrpc.HcnEndpointSettings{
					Name:       req.Name,
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

func (s *grpcService) GetEndpoints(ctx context.Context, req *ncproxygrpc.GetEndpointsRequest) (_ *ncproxygrpc.GetEndpointsResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "GetEndpoints") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	endpoints := []*ncproxygrpc.GetEndpointResponse{}

	rawEndpoints, err := hcn.ListEndpoints()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get HNS endpoints")
	}

	for _, endpoint := range rawEndpoints {
		endpointReq := &ncproxygrpc.GetEndpointRequest{
			Name: endpoint.Name,
		}
		e, err := s.GetEndpoint(ctx, endpointReq)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, e)
	}

	return &ncproxygrpc.GetEndpointsResponse{
		Endpoints: endpoints,
	}, nil
}

func (s *grpcService) GetNetwork(ctx context.Context, req *ncproxygrpc.GetNetworkRequest) (_ *ncproxygrpc.GetNetworkResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "GetNetwork") //nolint:ineffassign,staticcheck
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

	hcnResp, err := getHCNNetworkResponse(network)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get network information for network with name %q", req.Name)
	}

	return &ncproxygrpc.GetNetworkResponse{
		ID: network.Id,
		Network: &ncproxygrpc.Network{
			Settings: &ncproxygrpc.Network_HcnNetwork{
				HcnNetwork: hcnResp,
			},
		},
	}, nil
}

func (s *grpcService) GetNetworks(ctx context.Context, req *ncproxygrpc.GetNetworksRequest) (_ *ncproxygrpc.GetNetworksResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "GetNetworks") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	networks := []*ncproxygrpc.GetNetworkResponse{}

	rawNetworks, err := hcn.ListNetworks()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get HNS networks")
	}

	for _, network := range rawNetworks {
		networkReq := &ncproxygrpc.GetNetworkRequest{
			Name: network.Name,
		}
		n, err := s.GetNetwork(ctx, networkReq)
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
	agentStore *computeAgentStore
}

func newTTRPCService(ctx context.Context, agent *computeAgentCache, agentStore *computeAgentStore) *ttrpcService {
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
		ttrpc.WithUnaryClientInterceptor(octtrpc.ClientInterceptor()),
		ttrpc.WithOnClose(func() { conn.Close() }),
	)
	return &computeAgentClient{raw, computeagent.NewComputeAgentClient(raw)}, nil
}

func (s *ttrpcService) RegisterComputeAgent(ctx context.Context, req *ncproxyttrpc.RegisterComputeAgentRequest) (_ *ncproxyttrpc.RegisterComputeAgentResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "RegisterComputeAgent") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("containerID", req.ContainerID),
		trace.StringAttribute("agentAddress", req.AgentAddress))

	agent, err := getComputeAgentClient(req.AgentAddress)
	if err != nil {
		return nil, err
	}

	if err := s.agentStore.updateComputeAgent(ctx, req.ContainerID, req.AgentAddress); err != nil {
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
	ctx, span := trace.StartSpan(ctx, "UnregisterComputeAgent") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("containerID", req.ContainerID))

	err = s.agentStore.deleteComputeAgent(ctx, req.ContainerID)
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
	ctx, span := trace.StartSpan(ctx, "ConfigureNetworking") //nolint:ineffassign,staticcheck
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

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if _, err := nodeNetSvcClient.client.ConfigureNetworking(ctx, netsvcReq); err != nil {
		return nil, err
	}
	return &ncproxyttrpc.ConfigureNetworkingInternalResponse{}, nil
}
