//go:build windows

package service

import (
	"context"
	"sync"

	"github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/shim"
	"github.com/Microsoft/hcsshim/internal/shimdiag"

	sandboxsvc "github.com/containerd/containerd/api/runtime/sandbox/v1"
	tasksvc "github.com/containerd/containerd/api/runtime/task/v3"
	"github.com/containerd/containerd/v2/core/runtime"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/shutdown"
	"github.com/containerd/ttrpc"
)

// Service is the shared Service struct that implements all TTRPC Service interfaces.
// All Service methods (sandbox, task, and shimdiag) operate on this shared struct.
type Service struct {
	// mu is used to synchronize access to shared state within the Service.
	mu sync.Mutex

	// publisher is used to publish events from the shim to containerd.
	publisher shim.Publisher
	// events is a buffered channel used to queue events before they are published to containerd.
	events chan interface{}

	// sandboxID is the unique identifier for the sandbox managed by this Service instance.
	// For LCOW shim, sandboxID corresponds 1-1 with the UtilityVM managed by the shim.
	sandboxID string

	// sandboxOptions contains parsed, shim-level configuration for the sandbox
	// such as architecture and confidential-compute settings.
	sandboxOptions *lcow.SandboxOptions

	// vmController is responsible for managing the lifecycle of the underlying utility VM and its associated resources.
	vmController vm.Controller

	// shutdown manages graceful shutdown operations and allows registration of cleanup callbacks.
	shutdown shutdown.Service
}

var _ shim.TTRPCService = (*Service)(nil)

// NewService creates a new instance of the Service with the shared state.
func NewService(ctx context.Context, eventsPublisher shim.Publisher, sd shutdown.Service) *Service {
	svc := &Service{
		publisher:    eventsPublisher,
		events:       make(chan interface{}, 128), // Buffered channel for events
		vmController: vm.NewController(),
		shutdown:     sd,
	}

	go svc.forward(ctx, eventsPublisher)

	// Register a shutdown callback to close the events channel,
	// which signals the forward goroutine to exit.
	sd.RegisterCallback(func(context.Context) error {
		close(svc.events)
		return nil
	})

	// Perform best-effort VM cleanup on shutdown.
	sd.RegisterCallback(func(ctx context.Context) error {
		_ = svc.vmController.TerminateVM(ctx)
		return nil
	})

	return svc
}

// RegisterTTRPC registers the Task, Sandbox, and ShimDiag TTRPC services on
// the provided server so that containerd can call into the shim over TTRPC.
func (s *Service) RegisterTTRPC(server *ttrpc.Server) error {
	tasksvc.RegisterTTRPCTaskService(server, s)
	sandboxsvc.RegisterTTRPCSandboxService(server, s)
	shimdiag.RegisterShimDiagService(server, s)
	return nil
}

// SandboxID returns the unique identifier for the sandbox managed by this Service.
func (s *Service) SandboxID() string {
	return s.sandboxID
}

// send enqueues an event onto the internal events channel so that it can be
// forwarded to containerd asynchronously by the forward goroutine.
//
// TODO: wire up send() for task events once task lifecycle methods are implemented.
//
//nolint:unused
func (s *Service) send(evt interface{}) {
	s.events <- evt
}

// forward runs in a dedicated goroutine and publishes events from the internal
// events channel to containerd using the provided Publisher.  It exits when the
// events channel is closed (which happens during graceful shutdown).
func (s *Service) forward(ctx context.Context, publisher shim.Publisher) {
	ns, _ := namespaces.Namespace(ctx)
	ctx = namespaces.WithNamespace(context.Background(), ns)
	for e := range s.events {
		err := publisher.Publish(ctx, runtime.GetTopic(e), e)
		if err != nil {
			log.G(ctx).WithError(err).Error("post event")
		}
	}
	_ = publisher.Close()
}
