package main

import (
	"context"
	"net"
	"strings"
	"sync"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/log"
	ncproxystore "github.com/Microsoft/hcsshim/internal/ncproxy/store"
	"github.com/Microsoft/hcsshim/internal/ncproxyttrpc"
	ncproxygrpc "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v1"
	"github.com/Microsoft/hcsshim/pkg/octtrpc"
	"github.com/containerd/ttrpc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
)

type server struct {
	ttrpc *ttrpc.Server
	grpc  *grpc.Server
	conf  *config

	// store shared data on server for cleaning up later
	// database for containerID to compute agent address
	agentStore *ncproxystore.ComputeAgentStore
	// cache of container IDs to compute agent clients
	cache *computeAgentCache

	// database store for ncproxynetworking networks and endpoints
	// database for network name to ncproxy networking network
	ncproxyNetworking *ncproxystore.NetworkingStore
}

func newServer(ctx context.Context, conf *config, dbPath string) (*server, error) {
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, err
	}
	agentStore := ncproxystore.NewComputeAgentStore(db)
	agentCache := newComputeAgentCache()
	reconnectComputeAgents(ctx, agentStore, agentCache)

	ttrpcServer, err := ttrpc.NewServer(ttrpc.WithUnaryServerInterceptor(octtrpc.ServerInterceptor()))
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to create ttrpc server")
		return nil, err
	}
	return &server{
		grpc:              grpc.NewServer(grpc.StatsHandler(&ocgrpc.ServerHandler{})),
		ttrpc:             ttrpcServer,
		conf:              conf,
		agentStore:        agentStore,
		cache:             agentCache,
		ncproxyNetworking: ncproxystore.NewNetworkingStore(db),
	}, nil
}

func (s *server) setup(ctx context.Context) (net.Listener, net.Listener, error) {
	gService := newGRPCService(s.cache, s.ncproxyNetworking)
	ncproxygrpc.RegisterNetworkConfigProxyServer(s.grpc, gService)

	tService := newTTRPCService(ctx, s.cache, s.agentStore)
	ncproxyttrpc.RegisterNetworkConfigProxyService(s.ttrpc, tService)

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

// best effort graceful shutdown of the grpc and ttrpc servers
func (s *server) gracefulShutdown(ctx context.Context) {
	s.grpc.GracefulStop()
	if err := s.ttrpc.Shutdown(ctx); err != nil {
		log.G(ctx).WithError(err).Error("failed to gracefully shutdown ttrpc server")
	}
}

// best effort cleanup resources belonging to the server
func (s *server) cleanupResources(ctx context.Context) {
	if err := disconnectComputeAgents(ctx, s.cache); err != nil {
		log.G(ctx).WithError(err).Error("failed to disconnect connections in compute agent cache")
	}
	if err := s.agentStore.Close(); err != nil {
		log.G(ctx).WithError(err).Error("failed to close ncproxy compute agent database")
	}
	if err := s.ncproxyNetworking.Close(); err != nil {
		log.G(ctx).WithError(err).Error("failed to close ncproxy networking database")
	}
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

// reconnectComputeAgents handles reconnecting to existing compute agents on ncproxy
// restart.
//
// Ncproxy maintains a cache of active compute agents in order reestablish connections
// if the service is restarted. The cache is persisted in a bolt database. The schema
// can be found in `buckets.go`.
//
// On restart ncproxy will attempt to create new compute agent connections from the
// database of active compute agent addresses and add them to its compute agent client
// cache. Reconnect *MUST* be called before the server is allowed to start serving anything
// so that we can ensure that the cache is ready. Reconnections are performed in parallel
// to improve service startup performance.
//
// There are a few failure modes for reconnect:
//
// 1. If a compute agent entry is stale, connecting to the compute agent client will fail
//    and we will remove the entry from the database.
//
// 2. If an active compute agent exists but we fail to connect to it, we will again remove
//    the entry from the database. In this case, it is the node network service's
//    responsibility to cleanup host network resources that are no longer being used.
//
// Other failure modes are possible but not expected. In all failure cases we log the failures
// but allow the service start to proceed. We chose this approach vs just failing service
// start to avoid blocking service for all containers that had successful reconnection and to
// avoid blocking the creation of new containers until retry or mitigation.
func reconnectComputeAgents(ctx context.Context, agentStore *ncproxystore.ComputeAgentStore, agentCache *computeAgentCache) {
	computeAgentMap, err := agentStore.GetComputeAgents(ctx)
	if err != nil && errors.Is(err, ncproxystore.ErrBucketNotFound) {
		// no entries in the database yet, return early
		log.G(ctx).WithError(err).Debug("no entries in database")
		return
	} else if err != nil {
		log.G(ctx).WithError(err).Error("failed to get compute agent information")
	}

	var wg sync.WaitGroup
	for cid, addr := range computeAgentMap {
		wg.Add(1)
		go func(agentAddress, containerID string) {
			defer wg.Done()
			service, err := getComputeAgentClient(agentAddress)
			if err != nil {
				// can't connect to compute agent, remove entry in database
				log.G(ctx).WithField("agentAddress", agentAddress).WithError(err).Error("failed to create new compute agent client")
				dErr := agentStore.DeleteComputeAgent(ctx, containerID)
				if dErr != nil {
					log.G(ctx).WithField("key", containerID).WithError(dErr).Warn("failed to delete key from compute agent store")
				}
				return
			}
			log.G(ctx).WithField("containerID", containerID).Info("reconnected to container's compute agent")

			// connection succeeded, add entry in cache map for later
			// since the servers have not started running, we know that the cache cannot be empty
			// which would only happen on a call to `disconnectComputeAgents`, ignore error
			_ = agentCache.put(containerID, service)
		}(addr, cid)
	}

	wg.Wait()
}

// disconnectComputeAgents clears the cache of compute agent clients and cleans up
// their resources.
func disconnectComputeAgents(ctx context.Context, containerIDToComputeAgent *computeAgentCache) error {
	agents, err := containerIDToComputeAgent.getAllAndClear()
	if err != nil {
		return errors.Wrapf(err, "failed to get all cached compute agent clients")
	}
	for _, agent := range agents {
		if err := agent.Close(); err != nil {
			log.G(ctx).WithError(err).Error("failed to close compute agent connection")
		}
	}
	return nil
}
