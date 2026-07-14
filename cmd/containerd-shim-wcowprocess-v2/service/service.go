//go:build windows && wcowprocess

package service

import (
	"context"
	"sync"

	"github.com/Microsoft/hcsshim/internal/controller/pod"
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

// ShimName is the name of the process-isolated WCOW shim implementation.
const ShimName = "containerd-shim-wcowprocess-v2"

// Service implements the task, sandbox, and shimdiag TTRPC services for a
// single process-isolated WCOW pod.
type Service struct {
	// mu is used to synchronize access to shared state within the Service.
	mu sync.RWMutex

	// publisher is used to publish events from the shim to containerd.
	publisher shim.Publisher
	// events is a buffered channel used to queue events before they are published to containerd.
	events chan interface{}

	// sandboxID is the unique identifier for the sandbox managed by this Service instance.
	// The process-isolated WCOW shim serves a single sandbox that maps 1-1 with its pod.
	sandboxID string

	// pod is the single pod managed by this shim instance; it is nil until the sandbox is created.
	pod *pod.Controller

	// shutdown manages graceful shutdown operations and allows registration of cleanup callbacks.
	shutdown shutdown.Service
}

var _ shim.TTRPCService = (*Service)(nil)

// NewService creates a new Service with the shared state.
func NewService(ctx context.Context, eventsPublisher shim.Publisher, sd shutdown.Service) *Service {
	svc := &Service{
		publisher: eventsPublisher,
		events:    make(chan interface{}, 128),
		shutdown:  sd,
	}

	go svc.forward(ctx, eventsPublisher)

	sd.RegisterCallback(func(context.Context) error {
		close(svc.events)
		return nil
	})

	return svc
}

// RegisterTTRPC registers the Task, Sandbox, and ShimDiag TTRPC services.
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

// send enqueues an event onto the internal events channel.
func (s *Service) send(evt interface{}) {
	s.events <- evt
}

// forward publishes events from the internal channel to containerd until it is closed.
func (s *Service) forward(ctx context.Context, publisher shim.Publisher) {
	ns, _ := namespaces.Namespace(ctx)
	ctx = namespaces.WithNamespace(context.Background(), ns)
	for e := range s.events {
		if err := publisher.Publish(ctx, runtime.GetTopic(e), e); err != nil {
			log.G(ctx).WithError(err).Error("post event")
		}
	}
	_ = publisher.Close()
}
