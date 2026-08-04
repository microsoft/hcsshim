//go:build windows

package main

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/ot"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/shim"
	"go.opentelemetry.io/otel/attribute"
)

type publisher interface {
	publishEvent(ctx context.Context, topic string, event interface{}) (err error)
}

type eventPublisher struct {
	namespace       string
	remotePublisher *shim.RemoteEventsPublisher
}

var _ publisher = &eventPublisher{}

func newEventPublisher(address, namespace string) (*eventPublisher, error) {
	p, err := shim.NewPublisher(address)
	if err != nil {
		return nil, err
	}
	return &eventPublisher{
		namespace:       namespace,
		remotePublisher: p,
	}, nil
}

func (e *eventPublisher) close() error {
	return e.remotePublisher.Close()
}

func (e *eventPublisher) publishEvent(ctx context.Context, topic string, event interface{}) (err error) {
	ctx, span := ot.StartSpan(ctx, "publishEvent")
	defer span.End()
	defer func() { ot.SetSpanStatus(span, err) }()
	span.SetAttributes(
		attribute.String("topic", topic),
		attribute.String("event", fmt.Sprintf("%+v", event)))

	if e == nil {
		return nil
	}

	return e.remotePublisher.Publish(namespaces.WithNamespace(ctx, e.namespace), topic, event)
}
