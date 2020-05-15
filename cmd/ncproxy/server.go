package main

import (
	"context"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/cmd/ncproxy/ncproxygrpc"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ncproxyttrpc"
	"github.com/Microsoft/hcsshim/pkg/octtrpc"
	"github.com/containerd/ttrpc"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type server struct {
	ttrpc *ttrpc.Server
	grpc  *grpc.Server
	conf  *config
}

func newServer(ctx context.Context, conf *config) (*server, error) {
	ttrpcServer, err := ttrpc.NewServer(ttrpc.WithUnaryServerInterceptor(octtrpc.ServerInterceptor()))
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to create ttrpc server")
		return nil, err
	}
	return &server{
		grpc:  grpc.NewServer(),
		ttrpc: ttrpcServer,
		conf:  conf,
	}, nil
}

func (s *server) setup(ctx context.Context) (net.Listener, net.Listener, error) {
	ncproxygrpc.RegisterNetworkConfigProxyServer(s.grpc, &grpcService{})
	ncproxyttrpc.RegisterNetworkConfigProxyService(s.ttrpc, &ttrpcService{})

	ttrpcListener, err := winio.ListenPipe(s.conf.TTRPCAddr, nil)
	if err != nil {
		log.G(ctx).WithError(err).Errorf("failed to listen on %s", s.conf.TTRPCAddr)
		return nil, nil, err
	}

	grpcListener, err := net.Listen("tcp", s.conf.GRPCAddr)
	if err != nil {
		log.G(ctx).WithError(err).Errorf("failed to listen on %s", s.conf.GRPCAddr)
		return nil, nil, err
	}
	return ttrpcListener, grpcListener, nil
}

func (s *server) gracefulShutdown(ctx context.Context) error {
	s.grpc.GracefulStop()
	return s.ttrpc.Shutdown(ctx)
}

func trapClosedConnErr(err error) error {
	if err == nil || strings.Contains(err.Error(), "use of closed network connection") {
		return nil
	}
	return err
}

func (s *server) serve(ctx context.Context, ttrpcListener net.Listener, grpcListener net.Listener, serveErr chan error) {
	go func() {
		log.G(ctx).WithFields(logrus.Fields{
			"address": s.conf.TTRPCAddr,
		}).Info("Serving ncproxy TTRPC service")

		// No need to defer close the listener as ttrpc.Serve does this internally.
		serveErr <- trapClosedConnErr(s.ttrpc.Serve(ctx, ttrpcListener))
	}()

	go func() {
		log.G(ctx).WithFields(logrus.Fields{
			"address": s.conf.GRPCAddr,
		}).Info("Serving ncproxy GRPC service")

		defer grpcListener.Close()
		serveErr <- trapClosedConnErr(s.grpc.Serve(grpcListener))
	}()
}
