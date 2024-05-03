//go:build windows

package uvm

import (
	"context"
	"strings"

	"github.com/Microsoft/go-winio"
	"github.com/containerd/ttrpc"
	typeurl "github.com/containerd/typeurl/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/computeagent"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/hns"
	"github.com/Microsoft/hcsshim/internal/log"
	ncproxynetworking "github.com/Microsoft/hcsshim/internal/ncproxy/networking"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/otelttrpc"
)

func init() {
	typeurl.Register(&ncproxynetworking.Endpoint{}, "ncproxy/ncproxynetworking/Endpoint")
	typeurl.Register(&ncproxynetworking.Network{}, "ncproxy/ncproxynetworking/Network")
	typeurl.Register(&hcn.HostComputeEndpoint{}, "ncproxy/hcn/HostComputeEndpoint")
	typeurl.Register(&hcn.HostComputeNetwork{}, "ncproxy/hcn/HostComputeNetwork")
}

// This file holds the implementation of the Compute Agent service that is exposed for
// external network configuration.

const ComputeAgentAddrFmt = "\\\\.\\pipe\\computeagent-%s"

// create an interface here so we can mock out calls to the UtilityVM in our tests
type agentComputeSystem interface {
	AddEndpointToNSWithID(context.Context, string, string, *hns.HNSEndpoint) error
	UpdateNIC(context.Context, string, *hcsschema.NetworkAdapter) error
	RemoveEndpointFromNS(context.Context, string, *hns.HNSEndpoint) error
	AssignDevice(context.Context, string, uint16, string) (*VPCIDevice, error)
	RemoveDevice(context.Context, string, uint16) error
	AddNICInGuest(context.Context, *guestresource.LCOWNetworkAdapter) error
	RemoveNICInGuest(context.Context, *guestresource.LCOWNetworkAdapter) error
}

var _ agentComputeSystem = &UtilityVM{}

// mock hcn function for tests
var hnsGetHNSEndpointByName = hns.GetHNSEndpointByName

// computeAgent implements the ComputeAgent ttrpc service for adding and deleting NICs to a
// Utility VM.
type computeAgent struct {
	uvm agentComputeSystem
}

var _ computeagent.ComputeAgentService = &computeAgent{}

func (ca *computeAgent) AssignPCI(ctx context.Context, req *computeagent.AssignPCIInternalRequest) (*computeagent.AssignPCIInternalResponse, error) {
	log.G(ctx).WithFields(logrus.Fields{
		"containerID":          req.ContainerID,
		"deviceID":             req.DeviceID,
		"virtualFunctionIndex": req.VirtualFunctionIndex,
	}).Info("AssignPCI request")

	if req.DeviceID == "" {
		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
	}

	dev, err := ca.uvm.AssignDevice(ctx, req.DeviceID, uint16(req.VirtualFunctionIndex), req.NicID)
	if err != nil {
		return nil, err
	}
	return &computeagent.AssignPCIInternalResponse{ID: dev.VMBusGUID}, nil
}

func (ca *computeAgent) RemovePCI(ctx context.Context, req *computeagent.RemovePCIInternalRequest) (*computeagent.RemovePCIInternalResponse, error) {
	log.G(ctx).WithFields(logrus.Fields{
		"containerID": req.ContainerID,
		"deviceID":    req.DeviceID,
	}).Info("RemovePCI request")

	if req.DeviceID == "" {
		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
	}
	if err := ca.uvm.RemoveDevice(ctx, req.DeviceID, uint16(req.VirtualFunctionIndex)); err != nil {
		return nil, err
	}
	return &computeagent.RemovePCIInternalResponse{}, nil
}

// AddNIC will add a NIC to the computeagent services hosting UVM.
func (ca *computeAgent) AddNIC(ctx context.Context, req *computeagent.AddNICInternalRequest) (*computeagent.AddNICInternalResponse, error) {
	log.G(ctx).WithFields(logrus.Fields{
		"containerID": req.ContainerID,
		"endpoint":    req.Endpoint,
		"nicID":       req.NicID,
	}).Info("AddNIC request")

	if req.NicID == "" || req.Endpoint == nil {
		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
	}

	endpoint, err := typeurl.UnmarshalAny(req.Endpoint)
	if err != nil {
		return nil, err
	}

	switch endpt := endpoint.(type) {
	case *ncproxynetworking.Endpoint:
		cfg := &guestresource.LCOWNetworkAdapter{
			NamespaceID:    endpt.NamespaceID,
			ID:             req.NicID,
			IPAddress:      endpt.Settings.IPAddress,
			PrefixLength:   uint8(endpt.Settings.IPAddressPrefixLength),
			GatewayAddress: endpt.Settings.DefaultGateway,
			VPCIAssigned:   true,
		}
		if err := ca.uvm.AddNICInGuest(ctx, cfg); err != nil {
			return nil, err
		}
	case *hcn.HostComputeEndpoint:
		hnsEndpoint, err := hnsGetHNSEndpointByName(endpt.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get endpoint with name %q", endpt.Name)
		}
		if err := ca.uvm.AddEndpointToNSWithID(ctx, hnsEndpoint.Namespace.ID, req.NicID, hnsEndpoint); err != nil {
			return nil, err
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "invalid request endpoint type")
	}

	return &computeagent.AddNICInternalResponse{}, nil
}

// ModifyNIC will modify a NIC from the computeagent services hosting UVM.
func (ca *computeAgent) ModifyNIC(ctx context.Context, req *computeagent.ModifyNICInternalRequest) (*computeagent.ModifyNICInternalResponse, error) {
	log.G(ctx).WithFields(logrus.Fields{
		"nicID":    req.NicID,
		"endpoint": req.Endpoint,
	}).Info("ModifyNIC request")

	if req.NicID == "" || req.Endpoint == nil || req.IovPolicySettings == nil {
		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
	}

	endpoint, err := typeurl.UnmarshalAny(req.Endpoint)
	if err != nil {
		return nil, err
	}

	switch endpt := endpoint.(type) {
	case *ncproxynetworking.Endpoint:
		return nil, errors.New("modifying ncproxy networking endpoints is not supported")
	case *hcn.HostComputeEndpoint:
		hnsEndpoint, err := hnsGetHNSEndpointByName(endpt.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get endpoint with name `%s`", endpt.Name)
		}

		moderationValue := hcsschema.InterruptModerationValue(req.IovPolicySettings.InterruptModeration)
		moderationName := hcsschema.InterruptModerationValueToName[moderationValue]

		iovSettings := &hcsschema.IovSettings{
			OffloadWeight:       &req.IovPolicySettings.IovOffloadWeight,
			QueuePairsRequested: &req.IovPolicySettings.QueuePairsRequested,
			InterruptModeration: &moderationName,
		}

		nic := &hcsschema.NetworkAdapter{
			EndpointId:  hnsEndpoint.Id,
			MacAddress:  hnsEndpoint.MacAddress,
			IovSettings: iovSettings,
		}

		if err := ca.uvm.UpdateNIC(ctx, req.NicID, nic); err != nil {
			return nil, errors.Wrap(err, "failed to update UVM's network adapter")
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "invalid request endpoint type")
	}

	return &computeagent.ModifyNICInternalResponse{}, nil
}

// DeleteNIC will delete a NIC from the computeagent services hosting UVM.
func (ca *computeAgent) DeleteNIC(ctx context.Context, req *computeagent.DeleteNICInternalRequest) (*computeagent.DeleteNICInternalResponse, error) {
	log.G(ctx).WithFields(logrus.Fields{
		"containerID": req.ContainerID,
		"nicID":       req.NicID,
		"endpoint":    req.Endpoint,
	}).Info("DeleteNIC request")

	if req.NicID == "" || req.Endpoint == nil {
		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
	}

	endpoint, err := typeurl.UnmarshalAny(req.Endpoint)
	if err != nil {
		return nil, err
	}

	switch endpt := endpoint.(type) {
	case *ncproxynetworking.Endpoint:
		cfg := &guestresource.LCOWNetworkAdapter{
			ID: req.NicID,
		}
		if err := ca.uvm.RemoveNICInGuest(ctx, cfg); err != nil {
			return nil, err
		}
	case *hcn.HostComputeEndpoint:
		hnsEndpoint, err := hnsGetHNSEndpointByName(endpt.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get endpoint with name %q", endpt.Name)
		}
		if err := ca.uvm.RemoveEndpointFromNS(ctx, hnsEndpoint.Namespace.ID, hnsEndpoint); err != nil {
			return nil, err
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "invalid request endpoint type")
	}

	return &computeagent.DeleteNICInternalResponse{}, nil
}

func setupAndServe(ctx context.Context, caAddr string, vm *UtilityVM) error {
	// Setup compute agent service
	l, err := winio.ListenPipe(caAddr, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to listen on %s", caAddr)
	}
	s, err := ttrpc.NewServer(ttrpc.WithUnaryServerInterceptor(otelttrpc.ServerInterceptor()))
	if err != nil {
		return err
	}
	computeagent.RegisterComputeAgentService(s, &computeAgent{vm})

	log.G(ctx).WithField("address", l.Addr().String()).Info("serving compute agent")
	go func() {
		defer l.Close()
		if err := trapClosedConnErr(s.Serve(ctx, l)); err != nil {
			log.G(ctx).WithError(err).Fatal("compute agent: serve failure")
		}
	}()

	return nil
}

func trapClosedConnErr(err error) error {
	if err == nil || strings.Contains(err.Error(), "use of closed network connection") {
		return nil
	}
	return err
}
