package otelutil

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/Microsoft/hcsshim/internal/log"
)

var DefaultSampler = sdktrace.AlwaysSample()

// SetSpanStatus sets `span.SetStatus` to the proper status depending on `err`. If
// `err` is `nil` assumes `codes.Ok`.
func SetSpanStatus(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Code(toStatusCode(err)), err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
}

// StartSpan wraps "go.opentelemetry.io/otel/trace".StartSpan, but, if the span is sampling,
// adds a log entry to the context that points to the newly created span.
func StartSpan(ctx context.Context, name string, o ...trace.SpanStartOption) (context.Context, trace.Span) {
	ctx, s := otel.Tracer("").Start(ctx, name, o...)
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
