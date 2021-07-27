package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio/pkg/etwlogrus"
	"github.com/Microsoft/hcsshim/cmd/ncproxy/nodenetsvc"
	"github.com/Microsoft/hcsshim/internal/computeagent"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/ncproxy"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
)

type nodeNetSvcConn struct {
	client   nodenetsvc.NodeNetworkServiceClient
	addr     string
	grpcConn *grpc.ClientConn
}

type networkProxyStore struct {
	networkStore  *ncproxy.NetworkStore
	endpointStore *ncproxy.EndpointStore
}

var (
	// Global mapping of network namespace ID to shim compute agent ttrpc service.
	containerIDToShim = make(map[string]computeagent.ComputeAgentService)
	// Global object representing the connection to the node network service that
	// ncproxy will be talking to.
	nodeNetSvcClient *nodeNetSvcConn

	// store is the global object that is used to represent the database stores used
	// by ncproxy
	store *networkProxyStore
)

var (
	configPath    = flag.String("config", "", "Path to JSON configuration file.")
	logDir        = flag.String("log-directory", "", "Directory to write ncproxy logs to. This is just panic logs.")
	registerSvc   = flag.Bool("register-service", false, "Register ncproxy as a Windows service.")
	unregisterSvc = flag.Bool("unregister-service", false, "Unregister ncproxy as a Windows service.")
	runSvc        = flag.Bool("run-service", false, "Run ncproxy as a Windows service.")
)

// Run ncproxy
func run() error {
	flag.Parse()

	// Provider ID: cf9f01fe-87b3-568d-ecef-9f54b7c5ff70
	// Hook isn't closed explicitly, as it will exist until process exit.
	if hook, err := etwlogrus.NewHook("Microsoft.Virtualization.NCProxy"); err == nil {
		logrus.AddHook(hook)
	} else {
		logrus.Error(err)
	}

	// Register our OpenCensus logrus exporter
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
	trace.RegisterExporter(&oc.LogrusExporter{})

	// If no logging directory passed in use where ncproxy is located.
	if *logDir == "" {
		binLocation, err := os.Executable()
		if err != nil {
			return err
		}
		*logDir = filepath.Dir(binLocation)
	} else {
		// If a log dir was provided, make sure it exists.
		if _, err := os.Stat(*logDir); err != nil {
			if err := os.MkdirAll(*logDir, 0); err != nil {
				return errors.Wrap(err, "failed to make log directory")
			}
		}
	}

	// For both unregistering and registering the service we need to exit out (even on success). -register-service will register
	// ncproxy's commandline to launch with the -run-service flag set.
	if *unregisterSvc {
		if *registerSvc {
			return errors.New("-register-service and -unregister-service cannot be used together")
		}
		return unregisterService()
	}

	if *registerSvc {
		return registerService()
	}

	var serviceDone = make(chan struct{}, 1)

	// Launch as a Windows Service if necessary
	if *runSvc {
		panicLog := filepath.Join(*logDir, "ncproxy-panic.log")
		if err := initPanicFile(panicLog); err != nil {
			return err
		}
		logrus.SetOutput(ioutil.Discard)
		if err := launchService(serviceDone); err != nil {
			return err
		}
	}

	ctx := context.Background()
	conf, err := loadConfig(*configPath)
	if err != nil {
		return errors.Wrap(err, "failed getting configuration file")
	}

	if conf.GRPCAddr == "" {
		return errors.New("missing GRPC endpoint in config")
	}

	if conf.TTRPCAddr == "" {
		return errors.New("missing TTRPC endpoint in config")
	}

	// If there's a node network service in the config, assign this to our global client.
	if conf.NodeNetSvcAddr != "" {
		log.G(ctx).Infof("Connecting to NodeNetworkService at address %s", conf.NodeNetSvcAddr)

		dialCtx := ctx
		opts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithStatsHandler(&ocgrpc.ClientHandler{})}
		if conf.Timeout > 0 {
			var cancel context.CancelFunc
			dialCtx, cancel = context.WithTimeout(ctx, time.Duration(conf.Timeout)*time.Second)
			defer cancel()
			opts = append(opts, grpc.WithBlock())
		}
		client, err := grpc.DialContext(dialCtx, conf.NodeNetSvcAddr, opts...)
		if err != nil {
			return fmt.Errorf("failed to connect to NodeNetworkService at address %s", conf.NodeNetSvcAddr)
		}

		log.G(ctx).Infof("Successfully connected to NodeNetworkService at address %s", conf.NodeNetSvcAddr)

		netSvcClient := nodenetsvc.NewNodeNetworkServiceClient(client)
		nodeNetSvcClient = &nodeNetSvcConn{
			addr:     conf.NodeNetSvcAddr,
			client:   netSvcClient,
			grpcConn: client,
		}
	}

	// create the database stores
	binLocation, err := os.Executable()
	binDir := filepath.Dir(binLocation)
	db, err := bolt.Open(filepath.Join(binDir, "networkproxy.db"), 0600, nil)
	if err != nil {
		return err
	}
	defer db.Close()

	store = &networkProxyStore{
		networkStore:  metadata.NewNetworkStore(db),
		endpointStore: metadata.NewEndpointStore(db),
	}

	log.G(ctx).WithFields(logrus.Fields{
		"TTRPCAddr":      conf.TTRPCAddr,
		"NodeNetSvcAddr": conf.NodeNetSvcAddr,
		"GRPCAddr":       conf.GRPCAddr,
		"Timeout":        conf.Timeout,
	}).Info("starting ncproxy")

	serveErr := make(chan error, 1)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Create new server and then register NetworkConfigProxyServices.
	server, err := newServer(ctx, conf)
	if err != nil {
		return errors.New("failed to make new ncproxy server")
	}

	ttrpcListener, grpcListener, err := server.setup(ctx)
	if err != nil {
		return errors.New("failed to setup ncproxy server")
	}

	server.serve(ctx, ttrpcListener, grpcListener, serveErr)

	// Wait for server error or user cancellation.
	select {
	case <-sigChan:
		log.G(ctx).Info("Received interrupt. Closing")
	case err := <-serveErr:
		if err != nil {
			return errors.Wrap(err, "server failure")
		}
	case <-serviceDone:
		log.G(ctx).Info("Windows service stopped or shutdown")
	}

	// Cancel inflight requests and shutdown services
	if err := server.gracefulShutdown(ctx); err != nil {
		return errors.Wrap(err, "ncproxy failed to shutdown gracefully")
	}

	return nil
}
