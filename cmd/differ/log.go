package main

//
// helper functions for logging and tracing
//

import (
	"context"
	"encoding/base64"
	"io/ioutil"
	"os"

	"github.com/Microsoft/go-winio/pkg/etw"
	"github.com/Microsoft/go-winio/pkg/etwlogrus"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
	"go.opencensus.io/trace"
	"go.opencensus.io/trace/propagation"
)

func setupLogging() error {
	logrus.SetOutput(ioutil.Discard)
	logrus.AddHook(log.NewHook())
	if lvl, err := logrus.ParseLevel(os.Getenv(logLevelEnvVar)); err == nil {
		logrus.SetLevel(lvl)
	}

	f := func(guid.GUID, etw.ProviderState, etw.Level, uint64, uint64, uintptr) {}
	prov := "Containerd"
	if p, ok := os.LookupEnv(logETWProviderEnvVar); ok {
		prov = p
	}
	provider, err := etw.NewProvider(prov, f)
	if err != nil {
		return err
	}

	hook, err := etwlogrus.NewHookFromProvider(provider)
	if err != nil {
		return err
	}
	logrus.AddHook(hook)

	trace.ApplyConfig(trace.Config{DefaultSampler: oc.DefaultSampler})
	trace.RegisterExporter(&oc.LogrusExporter{})
	return nil
}

func startSpan(c *cli.Context, n string, o ...trace.StartOption) (s *trace.Span) {
	if sc, ok := spanContextFromEnv(); ok {
		c.Context, s = oc.StartSpanWithRemoteParent(c.Context, n, sc, o...)
	} else {
		c.Context, s = oc.StartSpan(c.Context, n, o...)
	}
	return s
}

func spanContextFromEnv() (sc trace.SpanContext, ok bool) {
	s, ok := os.LookupEnv(spanContextEnvVar)
	if !ok {
		return sc, ok
	}
	// b := make([]byte, base64.StdEncoding.DecodedLen(len(s)))
	// if _, err := base64.StdEncoding.Decode(b, []byte(s)); err != nil {
	// 	return sc, false
	// }
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return sc, false
	}
	return propagation.FromBinary(b)
}

func spanContextToString(ctx context.Context) (string, bool) {
	span := trace.FromContext(ctx)
	if span == nil {
		return "", false
	}
	b := propagation.Binary(span.SpanContext())
	return base64.StdEncoding.EncodeToString(b), true
}
