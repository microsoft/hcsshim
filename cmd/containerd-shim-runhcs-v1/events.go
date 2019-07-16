package main

import (
	"bytes"
	"context"
	"os/exec"
	"sync"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/containerd/typeurl"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

type publisher func(ctx context.Context, topic string, event interface{}) error

var _ = (publisher)(publishEvent)

var publishLock sync.Mutex

func publishEvent(ctx context.Context, topic string, event interface{}) (err error) {
	ctx, span := trace.StartSpan(ctx, "publishEvent")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("topic", topic))

	log.G(ctx).WithFields(logrus.Fields{
		"topic": topic,
		"event": event,
	}).Debug("Publishing event")

	publishLock.Lock()
	defer publishLock.Unlock()

	encoded, err := typeurl.MarshalAny(event)
	if err != nil {
		return errors.Wrap(err, "encode failed")
	}
	data, err := encoded.Marshal()
	if err != nil {
		return errors.Wrap(err, "marshal failed")
	}
	cmd := exec.Command(containerdBinaryFlag, "--address", addressFlag, "publish", "--topic", topic, "--namespace", namespaceFlag)
	cmd.Stdin = bytes.NewReader(data)
	err = cmd.Run()
	if err != nil {
		return errors.Wrap(err, "publish failed")
	}

	return nil
}
