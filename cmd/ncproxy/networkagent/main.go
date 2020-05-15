package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio/pkg/guid"

	"github.com/Microsoft/hcsshim/cmd/ncproxy/configagent"
	"github.com/Microsoft/hcsshim/cmd/ncproxy/ncproxygrpc"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// This is a barebones example of an implementation of the network
// config agent service that ncproxy talks to. This is solely used to test and
// will be removed.

const (
	netID       = "ContainerPlat-nat"
	listenAddr  = "127.0.0.1:9201"
	ncProxyAddr = "127.0.0.1:9200"
)

type service struct {
	client ncproxygrpc.NetworkConfigProxyClient
}

func (s *service) ConnectNamespaceToNetwork(ctx context.Context, req *configagent.ConnectNamespaceToNetworkRequest) (*configagent.ConnectNamespaceToNetworkResponse, error) {
	// Change NetworkID to NetworkName if this is the preferred method? Easier to
	// debug etc.
	log.G(ctx).WithFields(logrus.Fields{
		"namespace": req.NamespaceID,
		"networkID": req.NetworkID,
	}).Info("ConnectNamespaceToNetwork request")

	endpoints, err := hcsoci.GetNamespaceEndpoints(ctx, req.NamespaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace endpoints: %s", err)
	}
	added := false
	for _, endpoint := range endpoints {
		if endpoint.VirtualNetworkName == netID {
			nicID, err := guid.NewV4()
			if err != nil {
				return nil, fmt.Errorf("failed to create nic GUID: %s", err)
			}
			nsReq := &ncproxygrpc.AddNICRequest{
				NamespaceID: req.NamespaceID,
				NicID:       nicID.String(),
				EndpointID:  endpoint.Id,
			}
			if _, err := s.client.AddNIC(ctx, nsReq); err != nil {
				return nil, err
			}
			added = true
		}
	}
	if !added {
		return nil, errors.New("no endpoints found to add")
	}
	return &configagent.ConnectNamespaceToNetworkResponse{}, nil
}

func main() {
	ctx := context.Background()

	sigChan := make(chan os.Signal, 1)
	serveErr := make(chan error, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	defer signal.Stop(sigChan)

	grpcClient, err := grpc.Dial(
		ncProxyAddr,
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to connect to ncproxy")
		os.Exit(1)
	}
	defer grpcClient.Close()

	log.G(ctx).WithField("addr", ncProxyAddr).Info("connected to ncproxy")
	ncproxyClient := ncproxygrpc.NewNetworkConfigProxyClient(grpcClient)
	service := &service{ncproxyClient}
	server := grpc.NewServer()
	configagent.RegisterNetworkConfigAgentServer(server, service)

	grpcListener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.G(ctx).WithError(err).Errorf("failed to listen on %s", grpcListener.Addr().String())
		os.Exit(1)
	}

	go func() {
		defer grpcListener.Close()
		if err := server.Serve(grpcListener); err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				serveErr <- nil
			}
			serveErr <- err
		}
	}()

	log.G(ctx).WithField("addr", listenAddr).Info("serving config agent")

	go func() {
		req := &ncproxygrpc.RegisterNetworkConfigAgentRequest{
			AgentAddress: listenAddr,
			NetworkID:    netID,
		}
		_, err := ncproxyClient.RegisterNetworkConfigAgent(ctx, req)
		if err != nil {
			log.G(ctx).WithError(err).Warn("failed to register config agent")
		} else {
			log.G(ctx).Info("registered config agent")
		}
	}()

	// Wait for server error or user cancellation.
	select {
	case <-sigChan:
		log.G(ctx).Info("Received interrupt. Closing")
	case err := <-serveErr:
		if err != nil {
			log.G(ctx).WithError(err).Fatal("grpc service failure")
		}
	}

	// Cancel inflight requests and shutdown service
	server.GracefulStop()
}
