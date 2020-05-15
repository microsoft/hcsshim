package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Microsoft/hcsshim/cmd/ncproxy/nodenetsvc"
	"github.com/Microsoft/hcsshim/internal/computeagent"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type nodeNetSvcConn struct {
	client   nodenetsvc.NodeNetworkServiceClient
	addr     string
	grpcConn *grpc.ClientConn
}

var (
	configPath = flag.String("config", "", "Path to JSON configuration file.")
	// Global mapping of network namespace ID to shim compute agent ttrpc service.
	containerIDToShim = make(map[string]computeagent.ComputeAgentService)
	// Global object representing the connection to the node network service that
	// ncproxy will be talking to.
	nodeNetSvcClient *nodeNetSvcConn
)

func main() {
	flag.Parse()
	ctx := context.Background()
	conf, err := loadConfig(*configPath)
	if err != nil {
		log.G(ctx).WithError(err).Fatal("failed getting configuration file")
	}

	if conf.GRPCAddr == "" {
		log.G(ctx).Fatal("missing GRPC endpoint in config")
	}

	if conf.TTRPCAddr == "" {
		log.G(ctx).Fatal("missing TTRPC endpoint in config")
	}

	// If there's a node network service in the config, assign this to our global client.
	if conf.NodeNetSvcAddr != "" {
		log.G(ctx).Debugf("connecting to NodeNetworkService at address %s", conf.NodeNetSvcAddr)

		opts := []grpc.DialOption{grpc.WithInsecure()}
		if conf.Timeout > 0 {
			opts = append(opts, grpc.WithBlock(), grpc.WithTimeout(time.Duration(conf.Timeout)*time.Second))
		}
		client, err := grpc.Dial(conf.NodeNetSvcAddr, opts...)
		if err != nil {
			log.G(ctx).Fatalf("failed to connect to NodeNetworkService at address %s", conf.NodeNetSvcAddr)
		}

		log.G(ctx).Debugf("successfully connected to NodeNetworkService at address %s", conf.NodeNetSvcAddr)

		netSvcClient := nodenetsvc.NewNodeNetworkServiceClient(client)
		nodeNetSvcClient = &nodeNetSvcConn{
			addr:     conf.NodeNetSvcAddr,
			client:   netSvcClient,
			grpcConn: client,
		}
	}

	log.G(ctx).WithFields(logrus.Fields{
		"TTRPCAddr":      conf.TTRPCAddr,
		"NodeNetSvcAddr": conf.NodeNetSvcAddr,
		"GRPCAddr":       conf.GRPCAddr,
		"Timeout":        conf.Timeout,
	}).Info("starting ncproxy")

	sigChan := make(chan os.Signal, 1)
	serveErr := make(chan error, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	defer signal.Stop(sigChan)

	// Create new server and then register NetworkConfigProxyServices.
	server, err := newServer(ctx, conf)
	if err != nil {
		log.G(ctx).WithError(err).Fatal("failed to make new ncproxy server")
	}

	ttrpcListener, grpcListener, err := server.setup(ctx)
	if err != nil {
		log.G(ctx).WithError(err).Fatal("failed to setup ncproxy server")
	}

	server.serve(ctx, ttrpcListener, grpcListener, serveErr)

	// Wait for server error or user cancellation.
	select {
	case <-sigChan:
		log.G(ctx).Info("received interrupt. Closing")
	case err := <-serveErr:
		if err != nil {
			log.G(ctx).WithError(err).Fatal("service failure")
		}
	}

	// Cancel inflight requests and shutdown services
	if err := server.gracefulShutdown(ctx); err != nil {
		log.G(ctx).WithError(err).Fatal("ncproxy failed to shutdown gracefully")
	}
}
