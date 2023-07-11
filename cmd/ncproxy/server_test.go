//go:build windows

package main

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	computeagentMock "github.com/Microsoft/hcsshim/internal/computeagent/mock"
	ncproxystore "github.com/Microsoft/hcsshim/internal/ncproxy/store"
	"github.com/Microsoft/hcsshim/internal/ncproxyttrpc"
	nodenetsvcV0 "github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v0"
	nodenetsvcMockV0 "github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v0/mock"
	nodenetsvc "github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v1"
	nodenetsvcMock "github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v1/mock"
	"github.com/containerd/ttrpc"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRegisterComputeAgent(t *testing.T) {
	ctx := context.Background()

	// setup test database
	tempDir := t.TempDir()

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// create test TTRPC service
	store := ncproxystore.NewComputeAgentStore(db)
	agentCache := newComputeAgentCache()
	tService := newTTRPCService(ctx, agentCache, store)

	// setup mocked calls
	winioDialPipe = func(path string, timeout *time.Duration) (net.Conn, error) {
		rPipe, _ := net.Pipe()
		return rPipe, nil
	}
	ttrpcNewClient = func(conn net.Conn, opts ...ttrpc.ClientOpts) *ttrpc.Client {
		return &ttrpc.Client{}
	}

	containerID := t.Name() + "-containerID"
	req := &ncproxyttrpc.RegisterComputeAgentRequest{
		AgentAddress: t.Name() + "-agent-address",
		ContainerID:  containerID,
	}
	if _, err := tService.RegisterComputeAgent(ctx, req); err != nil {
		t.Fatalf("expected to get no error, instead got %v", err)
	}

	// validate that the entry was added to the agent
	actual, err := agentCache.get(containerID)
	if err != nil {
		t.Fatalf("failed to get the agent entry %v", err)
	}
	if actual == nil {
		t.Fatal("compute agent client was not put into agent cache")
	}
}

func TestConfigureNetworking_V1(t *testing.T) {
	ctx := context.Background()

	// setup test database
	tempDir := t.TempDir()

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// create test TTRPC service
	store := ncproxystore.NewComputeAgentStore(db)
	agentCache := newComputeAgentCache()
	tService := newTTRPCService(ctx, agentCache, store)

	// setup mocked client and mocked calls for nodenetsvc
	nodeNetCtrl := gomock.NewController(t)
	defer nodeNetCtrl.Finish()
	mockedClient := nodenetsvcMock.NewMockNodeNetworkServiceClient(nodeNetCtrl)
	mockedClientV0 := nodenetsvcMockV0.NewMockNodeNetworkServiceClient(nodeNetCtrl)
	nodeNetSvcClient = &nodeNetSvcConn{
		addr:     "",
		client:   mockedClient,
		v0Client: mockedClientV0,
	}

	// allow calls to v1 mock api
	mockedClient.EXPECT().ConfigureNetworking(gomock.Any(), gomock.Any()).Return(&nodenetsvc.ConfigureNetworkingResponse{}, nil).AnyTimes()

	type config struct {
		name          string
		containerID   string
		requestType   ncproxyttrpc.RequestTypeInternal
		errorExpected bool
	}
	containerID := t.Name() + "-containerID"
	tests := []config{
		{
			name:          "Configure Networking setup returns no error",
			containerID:   containerID,
			requestType:   ncproxyttrpc.RequestTypeInternal_Setup,
			errorExpected: false,
		},
		{
			name:          "Configure Networking teardown returns no error",
			containerID:   containerID,
			requestType:   ncproxyttrpc.RequestTypeInternal_Teardown,
			errorExpected: false,
		},
		{
			name:          "Configure Networking setup returns error when container ID is empty",
			containerID:   "",
			requestType:   ncproxyttrpc.RequestTypeInternal_Setup,
			errorExpected: true,
		},
		{
			name:          "Configure Networking setup returns error when request type is not supported",
			containerID:   containerID,
			requestType:   3, // unsupported request type
			errorExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			req := &ncproxyttrpc.ConfigureNetworkingInternalRequest{
				ContainerID: test.containerID,
				RequestType: test.requestType,
			}
			_, err := tService.ConfigureNetworking(ctx, req)
			if test.errorExpected && err == nil {
				t.Fatalf("expected ConfigureNetworking to return an error")
			}
			if !test.errorExpected && err != nil {
				t.Fatalf("expected ConfigureNetworking to return no error, instead got %v", err)
			}
		})
	}
}

func TestConfigureNetworking_V0(t *testing.T) {
	ctx := context.Background()

	// setup test database
	tempDir := t.TempDir()

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// create test TTRPC service
	store := ncproxystore.NewComputeAgentStore(db)
	agentCache := newComputeAgentCache()
	tService := newTTRPCService(ctx, agentCache, store)

	// setup mocked client and mocked calls for nodenetsvc
	nodeNetCtrl := gomock.NewController(t)
	defer nodeNetCtrl.Finish()
	mockedClient := nodenetsvcMock.NewMockNodeNetworkServiceClient(nodeNetCtrl)
	mockedClientV0 := nodenetsvcMockV0.NewMockNodeNetworkServiceClient(nodeNetCtrl)
	nodeNetSvcClient = &nodeNetSvcConn{
		addr:     "",
		client:   mockedClient,
		v0Client: mockedClientV0,
	}

	// v1 api calls should return "Unimplemented" so that we will try the v0 code path
	// allow succcessful calls to v0 api
	mockedClientV0.EXPECT().ConfigureNetworking(gomock.Any(), gomock.Any()).Return(&nodenetsvcV0.ConfigureNetworkingResponse{}, nil).AnyTimes()
	mockedClient.EXPECT().ConfigureNetworking(gomock.Any(), gomock.Any()).Return(nil, status.Error(codes.Unimplemented, "mock the v1 api not implemented")).AnyTimes()

	type config struct {
		name          string
		containerID   string
		requestType   ncproxyttrpc.RequestTypeInternal
		errorExpected bool
	}
	containerID := t.Name() + "-containerID"
	tests := []config{
		{
			name:          "Configure Networking setup returns no error",
			containerID:   containerID,
			requestType:   ncproxyttrpc.RequestTypeInternal_Setup,
			errorExpected: false,
		},
		{
			name:          "Configure Networking teardown returns no error",
			containerID:   containerID,
			requestType:   ncproxyttrpc.RequestTypeInternal_Teardown,
			errorExpected: false,
		},
		{
			name:          "Configure Networking setup returns error when container ID is empty",
			containerID:   "",
			requestType:   ncproxyttrpc.RequestTypeInternal_Setup,
			errorExpected: true,
		},
		{
			name:          "Configure Networking setup returns error when request type is not supported",
			containerID:   containerID,
			requestType:   3, // unsupported request type
			errorExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			req := &ncproxyttrpc.ConfigureNetworkingInternalRequest{
				ContainerID: test.containerID,
				RequestType: test.requestType,
			}
			_, err := tService.ConfigureNetworking(ctx, req)
			if test.errorExpected && err == nil {
				t.Fatalf("expected ConfigureNetworking to return an error")
			}
			if !test.errorExpected && err != nil {
				t.Fatalf("expected ConfigureNetworking to return no error, instead got %v", err)
			}
		})
	}
}

func TestReconnectComputeAgents_Success(t *testing.T) {
	ctx := context.Background()

	// setup test database
	tempDir := t.TempDir()

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// create test TTRPC service
	store := ncproxystore.NewComputeAgentStore(db)
	agentCache := newComputeAgentCache()

	// setup mocked calls
	winioDialPipe = func(path string, timeout *time.Duration) (net.Conn, error) {
		rPipe, _ := net.Pipe()
		return rPipe, nil
	}
	ttrpcNewClient = func(conn net.Conn, opts ...ttrpc.ClientOpts) *ttrpc.Client {
		return &ttrpc.Client{}
	}

	// add test entry in database
	containerID := "fake-container-id"
	address := "123412341234"

	if err := store.UpdateComputeAgent(ctx, containerID, address); err != nil {
		t.Fatal(err)
	}

	reconnectComputeAgents(ctx, store, agentCache)

	// validate that the agent cache has the entry now
	actualClient, err := agentCache.get(containerID)
	if err != nil {
		t.Fatal(err)
	}
	if actualClient == nil {
		t.Fatal("no entry added on reconnect to agent client cache")
	}
}

func TestReconnectComputeAgents_Failure(t *testing.T) {
	ctx := context.Background()

	// setup test database
	tempDir := t.TempDir()

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// create test TTRPC service
	store := ncproxystore.NewComputeAgentStore(db)
	agentCache := newComputeAgentCache()

	// setup mocked calls
	winioDialPipe = func(path string, timeout *time.Duration) (net.Conn, error) {
		// this will cause the reconnect compute agents call to run into an error
		// trying to reconnect to the fake container address
		return nil, errors.New("fake error")
	}
	ttrpcNewClient = func(conn net.Conn, opts ...ttrpc.ClientOpts) *ttrpc.Client {
		return &ttrpc.Client{}
	}

	// add test entry in database
	containerID := "fake-container-id"
	address := "123412341234"

	if err := store.UpdateComputeAgent(ctx, containerID, address); err != nil {
		t.Fatal(err)
	}

	reconnectComputeAgents(ctx, store, agentCache)

	// validate that the agent cache does NOT have an entry
	actualClient, err := agentCache.get(containerID)
	if err != nil {
		t.Fatal(err)
	}
	if actualClient != nil {
		t.Fatalf("expected no entry on failure, instead found %v", actualClient)
	}

	// validate that the agent store no longer has an entry for this container
	value, err := store.GetComputeAgent(ctx, containerID)
	if err == nil {
		t.Fatalf("expected an error, instead found value %s", value)
	}
}

func TestDisconnectComputeAgents(t *testing.T) {
	ctx := context.Background()
	containerID := "fake-container-id"

	agentCache := newComputeAgentCache()

	// create mocked compute agent service
	computeAgentCtrl := gomock.NewController(t)
	defer computeAgentCtrl.Finish()
	mockedService := computeagentMock.NewMockComputeAgentService(computeAgentCtrl)
	mockedAgentClient := &computeAgentClient{nil, mockedService}

	// put mocked compute agent in agent cache for test
	if err := agentCache.put(containerID, mockedAgentClient); err != nil {
		t.Fatal(err)
	}

	if err := disconnectComputeAgents(ctx, agentCache); err != nil {
		t.Fatal(err)
	}

	// validate there is no longer an entry for the compute agent client
	actual, err := agentCache.get(containerID)
	if err == nil {
		t.Fatalf("expected to find the cache empty, instead found %v", actual)
	}
}
