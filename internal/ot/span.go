package ot

import (
	"context"
	"github.com/Microsoft/hcsshim/internal/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	TracerName = "containerd-shim-runhcs-v1"
)

var DefaultSampler = sdktrace.AlwaysSample()

// SetSpanStatus sets `span.SetStatus` to the proper status depending on `err`. If
// `err` is `nil` assumes `trace.StatusCodeOk`.
func SetSpanStatus(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
}

// StartSpan wraps otel.Start(), but, if the span is sampling,
// adds a log entry to the context that points to the newly created span.
func StartSpan(ctx context.Context, name string, o ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := otel.Tracer(TracerName)
	ctx, s := tracer.Start(ctx, name, o...)
	return update(ctx, s)
}

// StartSpanWithRemoteParent wraps "go.opentelemetry.io/otel/trace".ContextWithRemoteSpanContext
//
// See StartSpan for more information.
func StartSpanWithRemoteParent(ctx context.Context, name string, parent trace.SpanContext, o ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := otel.Tracer(TracerName)
	ctx = trace.ContextWithRemoteSpanContext(ctx, parent)
	ctx, s := tracer.Start(ctx, name, o...)
	return update(ctx, s)
}

func update(ctx context.Context, s trace.Span) (context.Context, trace.Span) {
	if s.IsRecording() {
		ctx = log.UpdateContext(ctx)
	}

	return ctx, s
}

var WithServerSpanKind = trace.WithSpanKind(trace.SpanKindServer)
var WithClientSpanKind = trace.WithSpanKind(trace.SpanKindClient)

func spanKindToString(sk trace.SpanKind) string {
	switch sk {
	case trace.SpanKindClient:
		return "client"
	case trace.SpanKindServer:
		return "server"
	default:
		return ""
	}
}
