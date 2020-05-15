package main

import (
	context "context"
	"net"
	"strings"

	"github.com/Microsoft/hcsshim/internal/computeagent"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/containerd/ttrpc"

	"github.com/Microsoft/hcsshim/internal/hns"
	"github.com/Microsoft/hcsshim/internal/log"
)

// This file holds the implementation of the Compute Agent service that is exposed for
// external network configuration. The compute agent TTRPC service has only one method,
// AddNIC, that will add a NIC to a Utility VM and return the response of this operation.

// %s is Pod ID. <ID>@vm
const addrFmt = "\\\\.\\pipe\\computeagent-%s"

// computeAgent implements the ComputeAgent ttrpc service for adding NICs to a Utility
// VM.
type computeAgent struct {
	// Hosting UVM that NICs will be added to.
	uvm *uvm.UtilityVM
}

// AddNIC will add a NIC to the computeagent services hosting UVM.
func (ca *computeAgent) AddNIC(ctx context.Context, req *computeagent.AddNICRequest) (*computeagent.AddNICResponse, error) {
	log.G(ctx).Debug("AddNIC request received")
	endpoint, err := hns.GetHNSEndpointByID(req.EndpointID)
	if err != nil {
		return nil, err
	}
	if err := ca.uvm.AddEndpointToNSWithID(ctx, req.NamespaceID, req.NicID, endpoint); err != nil {
		return nil, err
	}
	return &computeagent.AddNICResponse{}, nil
}

func serveComputeAgent(ctx context.Context, server *ttrpc.Server, l net.Listener) {
	log.G(ctx).WithField("address", l.Addr().String()).Info("serving compute agent")

	go func() {
		defer l.Close()
		if err := server.Serve(ctx, l); err != nil &&
			!strings.Contains(err.Error(), "use of closed network connection") {
			log.G(ctx).WithError(err).Fatal("compute-agent: serve failure")
		}
	}()
}
