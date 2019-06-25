package main

import (
	"bytes"
	"context"
	"os/exec"
	"sync"

	"github.com/containerd/typeurl"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

type publisher func(ctx context.Context, topic string, event interface{})

var _ = (publisher)(publishEvent)

var publishLock sync.Mutex

func publishEvent(ctx context.Context, topic string, event interface{}) {
	_, span := trace.StartSpan(ctx, "publishEvent")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("topic", topic))

	publishLock.Lock()
	defer publishLock.Unlock()

	encoded, err := typeurl.MarshalAny(event)
	if err != nil {
		logrus.WithError(err).Error("publishEvent - Failed to encode event")
		return
	}
	data, err := encoded.Marshal()
	if err != nil {
		logrus.WithError(err).Error("publishEvent - Failed to marshal event")
		return
	}
	cmd := exec.Command(containerdBinaryFlag, "--address", addressFlag, "publish", "--topic", topic, "--namespace", namespaceFlag)
	cmd.Stdin = bytes.NewReader(data)
	err = cmd.Run()
	if err != nil {
		logrus.WithError(err).Error("publishEvent - Failed to publish event")
	}
}
