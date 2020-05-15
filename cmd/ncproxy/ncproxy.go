package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/cmd/ncproxy/configagent"
	"github.com/Microsoft/hcsshim/cmd/ncproxy/ncproxygrpc"
	"github.com/Microsoft/hcsshim/internal/computeagent"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ncproxyttrpc"
	"github.com/containerd/ttrpc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// GRPC service exposed for use by a network configuration agent. Holds a mutex for
// updating map of client connections.
type grpcService struct {
	m sync.Mutex
}

func (s *grpcService) AddNIC(ctx context.Context, req *ncproxygrpc.AddNICRequest) (*ncproxygrpc.AddNICResponse, error) {
	log.G(ctx).WithFields(logrus.Fields{
		"namespace":  req.NamespaceID,
		"endpointID": req.EndpointID,
		"nicID":      req.NicID,
	}).Info("AddNIC request")

	if client, ok := namespaceToShim[req.NamespaceID]; ok {
		caReq := &computeagent.AddNICRequest{
			NamespaceID: req.NamespaceID,
			NicID:       req.NicID,
			EndpointID:  req.EndpointID,
		}
		if _, err := client.AddNIC(ctx, caReq); err != nil {
			return nil, err
		}
		return &ncproxygrpc.AddNICResponse{}, nil
	}
	return nil, fmt.Errorf("no shim registered for namespace %s", req.NamespaceID)
}

func (s *grpcService) RegisterNetworkConfigAgent(ctx context.Context, req *ncproxygrpc.RegisterNetworkConfigAgentRequest) (*ncproxygrpc.RegisterNetworkConfigAgentResponse, error) {
	log.G(ctx).WithFields(logrus.Fields{
		"networkID":    req.NetworkID,
		"agentAddress": req.AgentAddress,
	}).Info("RegisterNetworkConfigAgent request")

	client, err := grpc.Dial(
		req.AgentAddress,
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(time.Duration(conf.Timeout)*time.Second),
	)
	if err != nil {
		return nil, err
	}
	// Add to global client map if connection succeeds. Don't check if there's already a map entry
	// just overwrite as the client may have changed the address of the config agent.
	s.m.Lock()
	defer s.m.Unlock()
	networkToConfigAgent[req.NetworkID] = configagent.NewNetworkConfigAgentClient(client)
	return &ncproxygrpc.RegisterNetworkConfigAgentResponse{}, nil
}

// TTRPC service exposed for use by the shim. Holds a mutex for updating map of
// client connections.
type ttrpcService struct {
	m sync.Mutex
}

func (s *ttrpcService) RegisterComputeAgent(ctx context.Context, req *ncproxyttrpc.RegisterComputeAgentRequest) (*ncproxyttrpc.RegisterComputeAgentResponse, error) {
	log.G(ctx).WithFields(logrus.Fields{
		"namespace":    req.NamespaceID,
		"agentAddress": req.AgentAddress,
	}).Info("RegisterComputeAgent request")

	conn, err := winio.DialPipe(req.AgentAddress, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to compute agent service")
	}
	cl := ttrpc.NewClient(conn, ttrpc.WithOnClose(func() { conn.Close() }))
	// Add to global client map if connection succeeds. Don't check if there's already a map entry
	// just overwrite as the client may have changed the address of the config agent.
	s.m.Lock()
	defer s.m.Unlock()
	namespaceToShim[req.NamespaceID] = computeagent.NewComputeAgentClient(cl)
	return &ncproxyttrpc.RegisterComputeAgentResponse{}, nil
}

func (s *ttrpcService) ConfigureNamespace(ctx context.Context, req *ncproxyttrpc.ConfigureNamespaceRequest) (*ncproxyttrpc.ConfigureNamespaceResponse, error) {
	log.G(ctx).WithFields(logrus.Fields{
		"namespace": req.NamespaceID,
	}).Info("ConfigureNamespace request")
	// Grab endpoints from namespace and check what network it belongs to. If the
	// network is registered call ConnectNamespaceToNetwork for the endpoint.
	endpoints, err := hcsoci.GetNamespaceEndpoints(ctx, req.NamespaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace endpoints: %s", err)
	}
	// If there are multiple endpoints in the same network we need to only call
	// ConnectNameSpaceToNetwork once, so we keep a map of seen networks.
	seen := make(map[string]struct{})
	for _, endpoint := range endpoints {
		if client, ok := networkToConfigAgent[endpoint.VirtualNetworkName]; ok {
			if _, ok := seen[endpoint.VirtualNetworkName]; ok {
				continue
			}
			seen[endpoint.VirtualNetworkName] = struct{}{}
			nsReq := &configagent.ConnectNamespaceToNetworkRequest{
				NamespaceID: req.NamespaceID,
				NetworkID:   endpoint.VirtualNetworkName,
			}
			if _, err := client.ConnectNamespaceToNetwork(ctx, nsReq); err != nil {
				return nil, err
			}
		}
	}
	return &ncproxyttrpc.ConfigureNamespaceResponse{}, nil
}
