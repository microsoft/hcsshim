//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/log"
	ncproxygrpc "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v1"
	nodenetsvcV0 "github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v0"
	nodenetsvc "github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v1"
)

// This is a barebones example of an implementation of the network
// config agent service that ncproxy talks to. This is solely used to test.

var configPath = flag.String("config", "", "Path to JSON configuration file.")

const (
	prefixLength uint32 = 24
	ipVersion           = "4"
)

func generateMAC() (string, error) {
	buf := make([]byte, 6)

	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}

	// set first number to 0
	buf[0] = 0
	mac := net.HardwareAddr(buf)
	macString := strings.ToUpper(mac.String())
	macString = strings.Replace(macString, ":", "-", -1)

	return macString, nil
}

func generateIPs(prefixLength string) (string, string, string) {
	buf := []byte{192, 168, 50}

	// set last to 0 for prefix
	ipPrefixBytes := append(buf, 0)
	ipPrefix := net.IP(ipPrefixBytes)
	ipPrefixString := ipPrefix.String() + "/" + prefixLength

	// set the last to 1 for gateway
	ipGatewayBytes := append(buf, 1)
	ipGateway := net.IP(ipGatewayBytes)
	ipGatewayString := ipGateway.String()

	// set last byte for IP address in range
	last := byte(rand.Intn(255-2) + 2)
	ipBytes := append(buf, last)
	ip := net.IP(ipBytes)
	ipString := ip.String()

	return ipPrefixString, ipGatewayString, ipString
}

func (s *service) configureHCNNetworkingHelper(ctx context.Context, req *nodenetsvc.ConfigureContainerNetworkingRequest) (_ *nodenetsvc.ConfigureContainerNetworkingResponse, err error) {
	prefixIP, gatewayIP, midIP := generateIPs(strconv.Itoa(int(prefixLength)))

	mode := ncproxygrpc.HostComputeNetworkSettings_NAT
	if s.conf.NetworkingSettings.HNSSettings.IOVSettings != nil {
		mode = ncproxygrpc.HostComputeNetworkSettings_Transparent
	}
	addNetworkReq := &ncproxygrpc.CreateNetworkRequest{
		Network: &ncproxygrpc.Network{
			Settings: &ncproxygrpc.Network_HcnNetwork{
				HcnNetwork: &ncproxygrpc.HostComputeNetworkSettings{
					Name:                  req.ContainerID + "_network_hcn",
					Mode:                  mode,
					SwitchName:            s.conf.NetworkingSettings.HNSSettings.SwitchName,
					IpamType:              ncproxygrpc.HostComputeNetworkSettings_Static,
					SubnetIpaddressPrefix: []string{prefixIP},
					DefaultGateway:        gatewayIP,
				},
			},
		},
	}

	networkResp, err := s.client.CreateNetwork(ctx, addNetworkReq)
	if err != nil {
		return nil, err
	}

	network, err := hcn.GetNetworkByID(networkResp.ID)
	if err != nil {
		return nil, err
	}
	s.containerToNetwork[req.ContainerID] = append(s.containerToNetwork[req.ContainerID], network.Name)

	mac, err := generateMAC()
	if err != nil {
		return nil, err
	}

	policies := &ncproxygrpc.HcnEndpointPolicies{}
	if s.conf.NetworkingSettings.HNSSettings.IOVSettings != nil {
		policies.IovPolicySettings = s.conf.NetworkingSettings.HNSSettings.IOVSettings
	}

	name := req.ContainerID + "_endpoint_hcn"
	endpointCreateReq := &ncproxygrpc.CreateEndpointRequest{
		EndpointSettings: &ncproxygrpc.EndpointSettings{
			Settings: &ncproxygrpc.EndpointSettings_HcnEndpoint{
				HcnEndpoint: &ncproxygrpc.HcnEndpointSettings{
					Name:                  name,
					Macaddress:            mac,
					Ipaddress:             midIP,
					IpaddressPrefixlength: prefixLength,
					NetworkName:           network.Name,
					Policies:              policies,
				},
			},
		},
	}

	endpt, err := s.client.CreateEndpoint(ctx, endpointCreateReq)
	if err != nil {
		return nil, err
	}

	log.G(ctx).WithField("endpt", endpt).Info("ConfigureContainerNetworking created endpoint")

	addEndpointReq := &ncproxygrpc.AddEndpointRequest{
		Name:        name,
		NamespaceID: req.NetworkNamespaceID,
	}
	_, err = s.client.AddEndpoint(ctx, addEndpointReq)
	if err != nil {
		return nil, err
	}
	s.containerToNamespace[req.ContainerID] = req.NetworkNamespaceID

	resultIPAddr := &nodenetsvc.ContainerIPAddress{
		Version:        ipVersion,
		Ip:             midIP,
		PrefixLength:   strconv.Itoa(int(prefixLength)),
		DefaultGateway: gatewayIP,
	}
	netInterface := &nodenetsvc.ContainerNetworkInterface{
		Name:               network.Name,
		MacAddress:         mac,
		NetworkNamespaceID: req.NetworkNamespaceID,
		Ipaddresses:        []*nodenetsvc.ContainerIPAddress{resultIPAddr},
	}

	return &nodenetsvc.ConfigureContainerNetworkingResponse{
		Interfaces: []*nodenetsvc.ContainerNetworkInterface{netInterface},
	}, nil
}

func (s *service) configureNCProxyNetworkingHelper(ctx context.Context, req *nodenetsvc.ConfigureContainerNetworkingRequest) (_ *nodenetsvc.ConfigureContainerNetworkingResponse, err error) {
	_, gatewayIP, midIP := generateIPs(strconv.Itoa(int(prefixLength)))
	networkName := req.ContainerID + "_network_ncproxy"
	addNetworkReq := &ncproxygrpc.CreateNetworkRequest{
		Network: &ncproxygrpc.Network{
			Settings: &ncproxygrpc.Network_NcproxyNetwork{
				NcproxyNetwork: &ncproxygrpc.NCProxyNetworkSettings{
					Name: networkName,
				},
			},
		},
	}

	_, err = s.client.CreateNetwork(ctx, addNetworkReq)
	if err != nil {
		return nil, err
	}
	s.containerToNetwork[req.ContainerID] = append(s.containerToNetwork[req.ContainerID], networkName)

	mac, err := generateMAC()
	if err != nil {
		return nil, err
	}

	name := req.ContainerID + "_endpoint_ncproxy"
	endpointCreateReq := &ncproxygrpc.CreateEndpointRequest{
		EndpointSettings: &ncproxygrpc.EndpointSettings{
			Settings: &ncproxygrpc.EndpointSettings_NcproxyEndpoint{
				NcproxyEndpoint: &ncproxygrpc.NCProxyEndpointSettings{
					Name:                  name,
					Macaddress:            mac,
					Ipaddress:             midIP,
					IpaddressPrefixlength: prefixLength,
					NetworkName:           networkName,
					DefaultGateway:        gatewayIP,
					DeviceDetails: &ncproxygrpc.NCProxyEndpointSettings_PciDeviceDetails{
						PciDeviceDetails: &ncproxygrpc.PCIDeviceDetails{
							DeviceID:             s.conf.NetworkingSettings.NCProxyNetworkingSettings.DeviceID,
							VirtualFunctionIndex: s.conf.NetworkingSettings.NCProxyNetworkingSettings.VirtualFunctionIndex,
						},
					},
				},
			},
		},
	}

	endpt, err := s.client.CreateEndpoint(ctx, endpointCreateReq)
	if err != nil {
		return nil, err
	}

	log.G(ctx).WithField("endpt", endpt).Info("ConfigureContainerNetworking created endpoint")

	addEndpointReq := &ncproxygrpc.AddEndpointRequest{
		Name:        name,
		NamespaceID: req.NetworkNamespaceID,
	}
	_, err = s.client.AddEndpoint(ctx, addEndpointReq)
	if err != nil {
		return nil, err
	}
	s.containerToNamespace[req.ContainerID] = req.NetworkNamespaceID

	resultIPAddr := &nodenetsvc.ContainerIPAddress{
		Version:        ipVersion,
		Ip:             midIP,
		PrefixLength:   strconv.Itoa(int(prefixLength)),
		DefaultGateway: gatewayIP,
	}
	netInterface := &nodenetsvc.ContainerNetworkInterface{
		Name:               networkName,
		MacAddress:         mac,
		NetworkNamespaceID: req.NetworkNamespaceID,
		Ipaddresses:        []*nodenetsvc.ContainerIPAddress{resultIPAddr},
	}

	return &nodenetsvc.ConfigureContainerNetworkingResponse{
		Interfaces: []*nodenetsvc.ContainerNetworkInterface{netInterface},
	}, nil
}

func (s *service) teardownConfigureContainerNetworking(ctx context.Context, req *nodenetsvc.ConfigureContainerNetworkingRequest) (_ *nodenetsvc.ConfigureContainerNetworkingResponse, err error) {
	eReq := &ncproxygrpc.GetEndpointsRequest{}
	resp, err := s.client.GetEndpoints(ctx, eReq)
	if err != nil {
		return nil, err
	}

	for _, endpoint := range resp.Endpoints {
		if endpoint == nil {
			log.G(ctx).Warn("failed to find endpoint to delete")
			continue
		}
		if endpoint.Endpoint == nil || endpoint.Endpoint.Settings == nil {
			log.G(ctx).WithField("name", endpoint.ID).Warn("failed to get endpoint settings")
			continue
		}
		if endpoint.Namespace == req.NetworkNamespaceID {
			endpointName := ""
			switch ep := endpoint.Endpoint.GetSettings().(type) {
			case *ncproxygrpc.EndpointSettings_NcproxyEndpoint:
				endpointName = ep.NcproxyEndpoint.Name
			case *ncproxygrpc.EndpointSettings_HcnEndpoint:
				endpointName = ep.HcnEndpoint.Name
			default:
				log.G(ctx).WithField("name", endpoint.ID).Warn("invalid endpoint settings type")
				continue
			}
			deleteEndptReq := &ncproxygrpc.DeleteEndpointRequest{
				Name: endpointName,
			}
			if _, err := s.client.DeleteEndpoint(ctx, deleteEndptReq); err != nil {
				log.G(ctx).WithField("name", endpointName).Warn("failed to delete endpoint")
			}
		}
	}

	if networks, ok := s.containerToNetwork[req.ContainerID]; ok {
		for _, networkName := range networks {
			deleteReq := &ncproxygrpc.DeleteNetworkRequest{
				Name: networkName,
			}
			if _, err := s.client.DeleteNetwork(ctx, deleteReq); err != nil {
				log.G(ctx).WithField("name", networkName).Warn("failed to delete network")
			}
		}
		delete(s.containerToNetwork, req.ContainerID)
	}

	return &nodenetsvc.ConfigureContainerNetworkingResponse{}, nil
}

func (s *service) ConfigureContainerNetworking(ctx context.Context, req *nodenetsvc.ConfigureContainerNetworkingRequest) (_ *nodenetsvc.ConfigureContainerNetworkingResponse, err error) {
	// for testing purposes, make endpoints here
	log.G(ctx).WithField("req", req).Info("ConfigureContainerNetworking request")

	if req.RequestType == nodenetsvc.RequestType_Setup {
		interfaces := []*nodenetsvc.ContainerNetworkInterface{}
		if s.conf.NetworkingSettings != nil && s.conf.NetworkingSettings.HNSSettings != nil {
			result, err := s.configureHCNNetworkingHelper(ctx, req)
			if err != nil {
				return nil, err
			}
			interfaces = append(interfaces, result.Interfaces...)
		}
		if s.conf.NetworkingSettings != nil && s.conf.NetworkingSettings.NCProxyNetworkingSettings != nil {
			result, err := s.configureNCProxyNetworkingHelper(ctx, req)
			if err != nil {
				return nil, err
			}
			interfaces = append(interfaces, result.Interfaces...)
		}
		return &nodenetsvc.ConfigureContainerNetworkingResponse{
			Interfaces: interfaces,
		}, nil
	} else if req.RequestType == nodenetsvc.RequestType_Teardown {
		return s.teardownConfigureContainerNetworking(ctx, req)
	}
	return nil, fmt.Errorf("invalid request type %v", req.RequestType)
}

func (s *service) addHelper(ctx context.Context, req *nodenetsvc.ConfigureNetworkingRequest, containerNamespaceID string) (_ *nodenetsvc.ConfigureNetworkingResponse, err error) {
	eReq := &ncproxygrpc.GetEndpointsRequest{}
	resp, err := s.client.GetEndpoints(ctx, eReq)
	if err != nil {
		return nil, err
	}
	log.G(ctx).WithField("endpts", resp.Endpoints).Info("ConfigureNetworking addrequest")

	for _, endpoint := range resp.Endpoints {
		if endpoint == nil {
			log.G(ctx).Warn("failed to find endpoint")
			continue
		}
		if endpoint.Endpoint == nil || endpoint.Endpoint.Settings == nil {
			log.G(ctx).WithField("name", endpoint.ID).Warn("failed to get endpoint settings")
			continue
		}
		if endpoint.Namespace == containerNamespaceID {
			// add endpoints that are in the namespace as NICs
			nicID, err := guid.NewV4()
			if err != nil {
				return nil, fmt.Errorf("failed to create nic GUID: %s", err)
			}
			endpointName := ""
			switch ep := endpoint.Endpoint.GetSettings().(type) {
			case *ncproxygrpc.EndpointSettings_NcproxyEndpoint:
				endpointName = ep.NcproxyEndpoint.Name
			case *ncproxygrpc.EndpointSettings_HcnEndpoint:
				endpointName = ep.HcnEndpoint.Name
			default:
				log.G(ctx).WithField("name", endpoint.ID).Warn("invalid endpoint settings type")
				continue
			}
			nsReq := &ncproxygrpc.AddNICRequest{
				ContainerID:  req.ContainerID,
				NicID:        nicID.String(),
				EndpointName: endpointName,
			}
			if _, err := s.client.AddNIC(ctx, nsReq); err != nil {
				return nil, err
			}
			s.endpointToNicID[endpointName] = nicID.String()
		}
	}

	defer func() {
		if err != nil {
			_, _ = s.teardownHelper(ctx, req, containerNamespaceID)
		}
	}()

	return &nodenetsvc.ConfigureNetworkingResponse{}, nil
}

func (s *service) teardownHelper(ctx context.Context, req *nodenetsvc.ConfigureNetworkingRequest, containerNamespaceID string) (*nodenetsvc.ConfigureNetworkingResponse, error) {
	eReq := &ncproxygrpc.GetEndpointsRequest{}
	resp, err := s.client.GetEndpoints(ctx, eReq)
	if err != nil {
		return nil, err
	}

	for _, endpoint := range resp.Endpoints {
		if endpoint == nil {
			log.G(ctx).Warn("failed to find endpoint to delete")
			continue
		}
		if endpoint.Endpoint == nil || endpoint.Endpoint.Settings == nil {
			log.G(ctx).WithField("name", endpoint.ID).Warn("failed to get endpoint settings")
			continue
		}

		if endpoint.Namespace == containerNamespaceID {
			endpointName := ""
			switch ep := endpoint.Endpoint.GetSettings().(type) {
			case *ncproxygrpc.EndpointSettings_NcproxyEndpoint:
				endpointName = ep.NcproxyEndpoint.Name
			case *ncproxygrpc.EndpointSettings_HcnEndpoint:
				endpointName = ep.HcnEndpoint.Name
			default:
				log.G(ctx).WithField("name", endpoint.ID).Warn("invalid endpoint settings type")
				continue
			}
			nicID, ok := s.endpointToNicID[endpointName]
			if !ok {
				log.G(ctx).WithField("name", endpointName).Warn("endpoint was not assigned a NIC ID previously")
				continue
			}
			// remove endpoints that are in the namespace as NICs
			nsReq := &ncproxygrpc.DeleteNICRequest{
				ContainerID:  req.ContainerID,
				NicID:        nicID,
				EndpointName: endpointName,
			}
			if _, err := s.client.DeleteNIC(ctx, nsReq); err != nil {
				log.G(ctx).WithField("name", endpointName).Warn("failed to delete endpoint nic")
			}
			delete(s.endpointToNicID, endpointName)
		}
	}
	return &nodenetsvc.ConfigureNetworkingResponse{}, nil
}

func (s *service) ConfigureNetworking(ctx context.Context, req *nodenetsvc.ConfigureNetworkingRequest) (*nodenetsvc.ConfigureNetworkingResponse, error) {
	log.G(ctx).WithField("req", req).Info("ConfigureNetworking request")

	containerNamespaceID, ok := s.containerToNamespace[req.ContainerID]
	if !ok {
		return nil, fmt.Errorf("no namespace was previously created for containerID %s", req.ContainerID)
	}

	if req.RequestType == nodenetsvc.RequestType_Setup {
		return s.addHelper(ctx, req, containerNamespaceID)
	}
	return s.teardownHelper(ctx, req, containerNamespaceID)
}

//nolint:stylecheck
func (s *service) GetHostLocalIpAddress(ctx context.Context, req *nodenetsvc.GetHostLocalIpAddressRequest) (*nodenetsvc.GetHostLocalIpAddressResponse, error) {
	return &nodenetsvc.GetHostLocalIpAddressResponse{IpAddr: ""}, nil
}

func (s *service) PingNodeNetworkService(ctx context.Context, req *nodenetsvc.PingNodeNetworkServiceRequest) (*nodenetsvc.PingNodeNetworkServiceResponse, error) {
	return &nodenetsvc.PingNodeNetworkServiceResponse{}, nil
}

func main() {
	var err error
	ctx := context.Background()

	flag.Parse()
	conf, err := readConfig(*configPath)
	if err != nil {
		log.G(ctx).WithError(err).Fatalf("failed to read network agent's config file at %s", *configPath)
	}
	log.G(ctx).WithFields(logrus.Fields{
		"config path": *configPath,
		"conf":        conf,
	}).Info("network agent configuration")

	sigChan := make(chan os.Signal, 1)
	serveErr := make(chan error, 1)
	defer close(serveErr)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	grpcClient, err := grpc.Dial(
		conf.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.G(ctx).WithError(err).Fatalf("failed to connect to ncproxy at %s", conf.GRPCAddr)
	}
	defer grpcClient.Close()

	log.G(ctx).WithField("addr", conf.GRPCAddr).Info("connected to ncproxy")
	ncproxyClient := ncproxygrpc.NewNetworkConfigProxyClient(grpcClient)
	service := &service{
		conf:                 conf,
		client:               ncproxyClient,
		containerToNamespace: make(map[string]string),
		endpointToNicID:      make(map[string]string),
		containerToNetwork:   make(map[string][]string),
	}
	v0Service := &v0ServiceWrapper{
		s: service,
	}
	server := grpc.NewServer()
	nodenetsvc.RegisterNodeNetworkServiceServer(server, service)
	nodenetsvcV0.RegisterNodeNetworkServiceServer(server, v0Service)

	grpcListener, err := net.Listen("tcp", conf.NodeNetSvcAddr)
	if err != nil {
		log.G(ctx).WithError(err).Fatalf("failed to listen on %s", grpcListener.Addr().String())
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

	log.G(ctx).WithField("addr", conf.NodeNetSvcAddr).Info("serving network service agent")

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
