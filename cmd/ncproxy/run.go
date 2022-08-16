//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio/pkg/etw"
	"github.com/Microsoft/go-winio/pkg/etwlogrus"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/computeagent"
	"github.com/Microsoft/hcsshim/internal/debug"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	nodenetsvc "github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v1"
	"github.com/containerd/ttrpc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
)

type nodeNetSvcConn struct {
	client   nodenetsvc.NodeNetworkServiceClient
	addr     string
	grpcConn *grpc.ClientConn
}

type computeAgentClient struct {
	raw *ttrpc.Client
	computeagent.ComputeAgentService
}

func (c *computeAgentClient) Close() error {
	if c.raw == nil {
		return nil
	}
	return c.raw.Close()
}

var (
	// Global object representing the connection to the node network service that
	// ncproxy will be talking to.
	nodeNetSvcClient *nodeNetSvcConn
)

func etwCallback(sourceID guid.GUID, state etw.ProviderState, level etw.Level, matchAnyKeyword uint64, matchAllKeyword uint64, filterData uintptr) {
	if state == etw.ProviderStateCaptureState {
		stacks := debug.DumpStacks()
		logrus.WithField("stack", stacks).Info("ncproxy goroutine stack dump")
	}
}

func app() *cli.App {
	app := cli.NewApp()
	app.Name = "ncproxy"
	app.Usage = "Network configuration proxy"
	app.Description = `
ncproxy is a network daemon designed to facilitate container network setup on a machine. It's
designed to communicate with several agents and simply acts as the proxy between the 'compute agent'
and 'node network' services.`
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config,c",
			Usage: "Path to the configuration file",
		},
		cli.StringFlag{
			Name:  "log-directory",
			Usage: "Directory to write ncproxy logs to. This is just panic logs.",
		},
		cli.StringFlag{
			Name:  "database-path",
			Usage: "Path to database file storing information on container to compute agent mapping.",
		},
		cli.BoolFlag{
			Name:  "register-service",
			Usage: "Register ncproxy as a Windows service.",
		},
		cli.BoolFlag{
			Name:  "unregister-service",
			Usage: "Unregister ncproxy as a Windows service.",
		},
		cli.BoolFlag{
			Name:   "run-service",
			Hidden: true,
			Usage:  "Run ncproxy as a Windows service.",
		},
	}
	app.Commands = []cli.Command{
		configCommand,
	}
	app.Action = func(ctx *cli.Context) error {
		return run(ctx)
	}
	return app
}

// Run ncproxy
func run(clicontext *cli.Context) error {
	var (
		configPath    = clicontext.GlobalString("config")
		logDir        = clicontext.GlobalString("log-directory")
		dbPath        = clicontext.GlobalString("database-path")
		registerSvc   = clicontext.GlobalBool("register-service")
		unregisterSvc = clicontext.GlobalBool("unregister-service")
		runSvc        = clicontext.GlobalBool("run-service")
	)

	// Provider ID: cf9f01fe-87b3-568d-ecef-9f54b7c5ff70
	// Hook isn't closed explicitly, as it will exist until process exit.
	if provider, err := etw.NewProvider("Microsoft.Virtualization.NCProxy", etwCallback); err == nil {
		if hook, err := etwlogrus.NewHookFromProvider(provider); err == nil {
			logrus.AddHook(hook)
		} else {
			logrus.Error(err)
		}
	} else {
		logrus.Error(err)
	}

	// Register our OpenCensus logrus exporter
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
	trace.RegisterExporter(&oc.LogrusExporter{})

	// If no logging directory passed in use where ncproxy is located.
	if logDir == "" {
		binLocation, err := os.Executable()
		if err != nil {
			return err
		}
		logDir = filepath.Dir(binLocation)
	} else {
		// If a log dir was provided, make sure it exists.
		if _, err := os.Stat(logDir); err != nil {
			if err := os.MkdirAll(logDir, 0); err != nil {
				return errors.Wrap(err, "failed to make log directory")
			}
		}
	}

	// For both unregistering and registering the service we need to exit out (even on success). -register-service will register
	// ncproxy's commandline to launch with the -run-service flag set.
	if unregisterSvc {
		if registerSvc {
			return errors.New("-register-service and -unregister-service cannot be used together")
		}
		return unregisterService()
	}

	if registerSvc {
		return registerService()
	}

	var serviceDone = make(chan struct{}, 1)

	// Launch as a Windows Service if necessary
	if runSvc {
		panicLog := filepath.Join(logDir, "ncproxy-panic.log")
		if err := initPanicFile(panicLog); err != nil {
			return err
		}
		logrus.SetOutput(io.Discard)
		if err := launchService(serviceDone); err != nil {
			return err
		}
	}

	ctx := context.Background()
	conf, err := loadConfig(configPath)
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

	// setup ncproxy databases
	if dbPath == "" {
		// default location for ncproxy database
		binLocation, err := os.Executable()
		if err != nil {
			return err
		}
		dbPath = filepath.Dir(binLocation) + "networkproxy.db"
	} else {
		// If a db path was provided, make sure parent directories exist
		dir := filepath.Dir(dbPath)
		if _, err := os.Stat(dir); err != nil {
			if err := os.MkdirAll(dir, 0); err != nil {
				return errors.Wrap(err, "failed to make database directory")
			}
		}
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
	server, err := newServer(ctx, conf, dbPath)
	if err != nil {
		return errors.New("failed to make new ncproxy server")
	}
	defer server.cleanupResources(ctx)

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
	server.gracefulShutdown(ctx)

	return nil
}
