package oc

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/log"
	"go.opencensus.io/trace"
)

var DefaultSampler = trace.AlwaysSample()

// SetSpanStatus sets `span.SetStatus` to the proper status depending on `err`. If
// `err` is `nil` assumes `trace.StatusCodeOk`.
func SetSpanStatus(span *trace.Span, err error) {
	status := trace.Status{}
	if err != nil {
		status.Code = int32(toStatusCode(err))
		status.Message = err.Error()
	}
	span.SetStatus(status)
}

// StartSpan wraps go.opencensus.io/oc.StartSpan, but, if the span is sampling,
// updates the context of the log entry in the context to the newly created value.
func StartSpan(ctx context.Context, name string, o ...trace.StartOption) (context.Context, *trace.Span) {
	ctx, s := trace.StartSpan(ctx, name, o...)
	if s.IsRecordingEvents() {
		ctx = log.UpdateContext(ctx)
	}

	return ctx, s
}

var WithServerSpanKind = trace.WithSpanKind(trace.SpanKindServer)
var WithClientSpanKind = trace.WithSpanKind(trace.SpanKindServer)

func spanKindToString(sk int) string {
	switch sk {
	case trace.SpanKindUnspecified:
		return "unknown"
	case trace.SpanKindClient:
		return "client"
	case trace.SpanKindServer:
		return "server"
	default:
		return ""
	}
}
