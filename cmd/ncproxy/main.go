package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/Microsoft/hcsshim/cmd/ncproxy/configagent"
	"github.com/Microsoft/hcsshim/internal/computeagent"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/sirupsen/logrus"
)

var (
	configPath = flag.String("config", "", "Path to JSON configuration file.")
	// Global mapping of network namespace ID to shim compute agent ttrpc service.
	namespaceToShim = make(map[string]computeagent.ComputeAgentService)
	// Mapping of network name to config agent clients
	networkToConfigAgent = make(map[string]configagent.NetworkConfigAgentClient)
	// NcProxy configuration
	conf = new(config)
)

func main() {
	flag.Parse()
	ctx := context.Background()
	conf, err := loadConfig(*configPath)
	if err != nil {
		log.G(ctx).WithError(err).Error("failed getting configuration file")
		os.Exit(1)
	}

	if conf.GRPCAddr == "" {
		log.G(ctx).Error("missing GRPC endpoint in config")
		os.Exit(1)
	}
	if conf.TTRPCAddr == "" {
		log.G(ctx).Error("missing TTRPC endpoint in config")
		os.Exit(1)
	}
	if conf.Timeout == 0 {
		conf.Timeout = 5
	}

	// Construct config agent client connections from config file information.
	if err := configToClients(ctx, conf); err != nil {
		log.G(ctx).WithError(err).Error("failed to dial clients")
		os.Exit(1)
	}

	log.G(ctx).Info("Starting ncproxy")

	sigChan := make(chan os.Signal, 1)
	serveErr := make(chan error, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	defer signal.Stop(sigChan)

	// Create new server and then register NetworkConfigProxyServices.
	server, err := newServer(ctx)
	if err != nil {
		os.Exit(1)
	}

	ttrpcListener, grpcListener, err := server.setup(ctx)
	if err != nil {
		os.Exit(1)
	}

	go func() {
		log.G(ctx).WithFields(logrus.Fields{
			"address": conf.TTRPCAddr,
		}).Info("Serving TTRPC service")

		serveErr <- trapClosedConnErr(server.ttrpc.Serve(ctx, ttrpcListener))
	}()

	go func() {
		log.G(ctx).WithFields(logrus.Fields{
			"address": conf.GRPCAddr,
		}).Info("Serving GRPC service")

		defer grpcListener.Close()
		serveErr <- trapClosedConnErr(server.grpc.Serve(grpcListener))
	}()

	// Wait for server error or user cancellation.
	select {
	case <-sigChan:
		log.G(ctx).Info("Received interrupt. Closing")
	case err := <-serveErr:
		if err != nil {
			log.G(ctx).WithError(err).Fatal("service failure")
		}
	}

	// Cancel inflight requests and shutdown services
	if err := server.gracefulShutdown(ctx); err != nil {
		log.G(ctx).WithError(err).Warn("ncproxy failed to shutdown gracefully")
	}
}
