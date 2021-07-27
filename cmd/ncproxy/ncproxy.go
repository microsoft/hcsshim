package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/cmd/ncproxy/ncproxygrpc"
	"github.com/Microsoft/hcsshim/cmd/ncproxy/nodenetsvc"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/computeagent"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ncproxy"
	"github.com/Microsoft/hcsshim/internal/ncproxyttrpc"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/pkg/octtrpc"
	"github.com/containerd/ttrpc"
	"github.com/docker/docker/errdefs"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPC service exposed for use by a Node Network Service.
type grpcService struct{}

var _ ncproxygrpc.NetworkConfigProxyServer = &grpcService{}

func (s *grpcService) AssignPCI(ctx context.Context, req *ncproxygrpc.AssignPCIRequest) (_ *ncproxygrpc.AssignPCIResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "AssignPCI")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("containerID", req.ContainerID))

	if req.ContainerID == "" || req.DeviceID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	if client, ok := containerIDToShim[req.ContainerID]; ok {
		caReq := &computeagent.AssignPCIInternalRequest{
			ContainerID:          req.ContainerID,
			DeviceID:             req.DeviceID,
			VirtualFunctionIndex: req.VirtualFunctionIndex,
		}
		resp, err := client.AssignPCI(ctx, caReq)
		if err != nil {
			return nil, err
		}
		return &ncproxygrpc.AssignPCIResponse{ID: resp.ID}, nil
	}
	return nil, status.Errorf(codes.FailedPrecondition, "No shim registered for containerID `%s`", req.ContainerID)
}

func (s *grpcService) RemovePCI(ctx context.Context, req *ncproxygrpc.RemovePCIRequest) (_ *ncproxygrpc.RemovePCIResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "RemovePCI")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("containerID", req.ContainerID))

	if req.ContainerID == "" || req.DeviceID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	if client, ok := containerIDToShim[req.ContainerID]; ok {
		caReq := &computeagent.RemovePCIInternalRequest{
			ContainerID:          req.ContainerID,
			DeviceID:             req.DeviceID,
			VirtualFunctionIndex: req.VirtualFunctionIndex,
		}
		if _, err := client.RemovePCI(ctx, caReq); err != nil {
			return nil, err
		}
		return &ncproxygrpc.RemovePCIResponse{}, nil
	}
	return nil, status.Errorf(codes.FailedPrecondition, "No shim registered for containerID `%s`", req.ContainerID)
}

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

func createModifyNICInternalRequest(iovSettings *ncproxygrpc.IovEndpointPolicySetting) *computeagent.IovSettings {
	return &computeagent.IovSettings{
		IovOffloadWeight:    iovSettings.IovOffloadWeight,
		QueuePairsRequested: iovSettings.QueuePairsRequested,
		InterruptModeration: iovSettings.InterruptModeration,
	}
}

func constructHCNIovPolicySetting(iovSettings *ncproxygrpc.IovEndpointPolicySetting) *hcn.IovPolicySetting {
	return &hcn.IovPolicySetting{
		IovOffloadWeight:    iovSettings.IovOffloadWeight,
		QueuePairsRequested: iovSettings.QueuePairsRequested,
		InterruptModeration: iovSettings.InterruptModeration,
	}
}

func (s *grpcService) ModifyNIC(ctx context.Context, req *ncproxygrpc.ModifyNICRequest) (_ *ncproxygrpc.ModifyNICResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "ModifyNIC")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("containerID", req.ContainerID),
		trace.StringAttribute("endpointName", req.EndpointName),
		trace.StringAttribute("nicID", req.NicID))

	log.G(ctx).WithField("settings", req.EndpointSettings).Info("ModifyNIC iov settings")

	if req.ContainerID == "" || req.EndpointName == "" || req.NicID == "" || req.EndpointSettings == nil || req.EndpointSettings.IovPolicySettings == nil {
		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
	}

	if client, ok := containerIDToShim[req.ContainerID]; ok {
		caReq := &computeagent.ModifyNICInternalRequest{
			NicID:             req.NicID,
			EndpointName:      req.EndpointName,
			IovPolicySettings: createModifyNICInternalRequest(req.EndpointSettings.IovPolicySettings),
		}

		hcnIOVSettings := constructHCNIovPolicySetting(req.EndpointSettings.IovPolicySettings)
		rawJSON, err := json.Marshal(hcnIOVSettings)
		if err != nil {
			return nil, err
		}

		iovPolicy := hcn.EndpointPolicy{
			Type:     hcn.IOV,
			Settings: rawJSON,
		}
		policies := []hcn.EndpointPolicy{iovPolicy}

		ep, err := hcn.GetEndpointByName(req.EndpointName)
		if err != nil {
			if _, ok := err.(hcn.EndpointNotFoundError); ok {
				return nil, status.Errorf(codes.NotFound, "no endpoint with name `%s` found", req.EndpointName)
			}
			return nil, errors.Wrapf(err, "failed to get endpoint with name `%s`", req.EndpointName)
		}

		// To turn off iov offload on an endpoint, we need to first call into HCS to change the
		// offload weight and then call into HNS to revoke the policy.
		//
		// To turn on iov offload, the reverse order is used.
		if req.EndpointSettings.IovPolicySettings.IovOffloadWeight == 0 {
			if _, err := client.ModifyNIC(ctx, caReq); err != nil {
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
			if _, err := client.ModifyNIC(ctx, caReq); err != nil {
				return nil, err
			}
		}

		return &ncproxygrpc.ModifyNICResponse{}, nil
	}
	return nil, status.Errorf(codes.FailedPrecondition, "No shim registered for containerID `%s`", req.ContainerID)
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

func (s *grpcService) CreateNetwork(ctx context.Context, req *ncproxygrpc.CreateNetworkRequest) (resp *ncproxygrpc.CreateNetworkResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "CreateNetwork") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	if req.Network == nil || req.Network.GetSettings() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "must provide network settings in request: %+v", req)
	}

	var network ncproxy.Network
	// TODO katiewasnothere: use type instead of this if/else
	if req.Network.GetHcnSettings() != nil {
		network, err = ncproxy.CreateHcnNetwork(req.Network.GetHcnSettings())
		if err != nil {
			return nil, err
		}
	} else if req.Network.GetCustomNetworkSettings() != nil {
		network, err = ncproxy.CreateCustomNetwork(req.Network.GetCustomNetworkSettings())
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.Errorf("invalid network settings %v", req.Network.GetSettings())
	}

	if err := store.networkStore.Update(ctx, network.ID(), network); err != nil {
		return nil, err
	}
	return &ncproxygrpc.CreateNetworkResponse{ID: network.ID()}, nil
}

func (s *grpcService) CreateEndpoint(ctx context.Context, req *ncproxygrpc.CreateEndpointRequest) (resp *ncproxygrpc.CreateEndpointResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "CreateEndpoint") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	if req.EndpointSettings == nil || req.EndpointSettings.GetSettings() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	var endpoint ncproxy.Endpoint

	if req.EndpointSettings.GetHcnEndpoint() != nil {
		endpoint, err = ncproxy.CreateHcnEndpoint(req.EndpointSettings.GetHcnEndpoint())
		if err != nil {
			return nil, err
		}
	} else if req.EndpointSettings.GetCustomEndpoint() != nil {
		endpoint, err = ncproxy.CreateCustomEndpoint(req.EndpointSettings.GetCustomEndpoint())
		if err != nil {
			return nil, err
		}
	}

	// store the endpoint
	err = store.endpointStore.Update(ctx, endpoint.Name(), endpoint)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to add endpoint with name %q", req.Settings.Name)
	}

	return resp, nil
}

func (s *grpcService) AddEndpoint(ctx context.Context, req *ncproxygrpc.AddEndpointRequest) (_ *ncproxygrpc.AddEndpointResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "AddEndpoint") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("endpointName", req.Name))

	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "received empty field in request: %+v", req)
	}

	// get the endpoint from store
	endpt, err := store.endpointStore.Get(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	if err := endpt.Add(ctx, req.NamespaceID); err != nil {
		return nil, err
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

	// get the endpoint from store
	endpt, err := store.endpointStore.Get(ctx, req.Name)
	if err != nil {
		// todo katiewasnothere: best effort
		return nil, err
	}

	if err := endpt.Delete(ctx); err != nil {
		// todo katiewasnothere: best effort

		return nil, err
	}

	// TODO katiewasnothere: remove from database

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

	network, err := store.networkStore.Get(ctx, req.Name)
	if err != nil {
		// todo katiewasnothere: best effort
		return nil, err
	}

	if err := network.Delete(ctx); err != nil {
		// todo katiewasnothere: best effort

		return nil, err
	}

	// todo katiewasnothere: remove from db

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

	endpt, err := store.endpointStore.Get(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	return &ncproxygrpc.GetEndpointResponse{
		ID:               endpt.ID(),
		Name:             endpt.Name(),
		EndpointSettings: endpt.Settings(),
	}, nil
}

func (s *grpcService) GetEndpoints(ctx context.Context, req *ncproxygrpc.GetEndpointsRequest) (_ *ncproxygrpc.GetEndpointsResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "GetEndpoints") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	rawEndpoints, err := store.endpointStore.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	endpoints := make([]*ncproxygrpc.GetEndpointResponse, len(rawEndpoints))
	for i, endpoint := range rawEndpoints {
		resp := &ncproxygrpc.GetEndpointResponse{
			ID:               endpoint.ID(),
			Name:             endpoint.Name(),
			EndpointSettings: endpoint.Settings(),
		}
		endpoints[i] = resp
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

	network, err := store.networkStore.Get(ctx, req.Name)
	if err != nil {
		if _, ok := err.(errdefs.ErrNotFound); ok {
			return nil, status.Errorf(codes.NotFound, "no network with name `%s` found", req.Name)
		}
		return nil, errors.Wrapf(err, "failed to get network with name %q", req.Name)
	}

	return &ncproxygrpc.GetNetworkResponse{
		ID:   network.ID(),
		Name: network.Name(),
	}, nil
}

// TODO: list all networks  katiewasnothere
func (s *grpcService) GetNetworks(ctx context.Context, req *ncproxygrpc.GetNetworksRequest) (_ *ncproxygrpc.GetNetworksResponse, err error) {
	ctx, span := trace.StartSpan(ctx, "GetNetworks") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	rawNetworks, err := store.networkStore.GetAll(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get ncproxy networks")
	}

	networks := make([]*ncproxygrpc.GetNetworkResponse, len(rawNetworks))
	for i, network := range rawNetworks {
		resp := &ncproxygrpc.GetNetworkResponse{
			ID:   network.ID(),
			Name: network.Name(),
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
	ctx, span := trace.StartSpan(ctx, "RegisterComputeAgent") //nolint:ineffassign,staticcheck
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
