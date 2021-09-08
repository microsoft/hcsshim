package main

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/containerd/containerd/namespaces"
	shim "github.com/containerd/containerd/runtime/v2/shim"
	"go.opencensus.io/trace"
)

type publisher interface {
	publishEvent(ctx context.Context, topic string, event interface{}) (err error)
}

type eventPublisher struct {
	remotePublisher *shim.RemoteEventsPublisher
}

var _ = (publisher)(&eventPublisher{})

func newEventPublisher(address string) (*eventPublisher, error) {
	p, err := shim.NewPublisher(address)
	if err != nil {
		return nil, err
	}
	return &eventPublisher{
		remotePublisher: p,
	}, nil
}

func (e *eventPublisher) close() error {
	return e.remotePublisher.Close()
}

func (e *eventPublisher) publishEvent(ctx context.Context, topic string, event interface{}) (err error) {
	ctx, span := trace.StartSpan(ctx, "publishEvent")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("topic", topic),
		trace.StringAttribute("event", fmt.Sprintf("%+v", event)))

	if e == nil {
		return nil
	}

	return e.remotePublisher.Publish(namespaces.WithNamespace(ctx, namespaceFlag), topic, event)
}
