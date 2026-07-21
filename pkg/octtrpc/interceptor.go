// Deprecated: Use github.com/containerd/otelttrpc instead.
package octtrpc

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/containerd/ttrpc"
	"go.opencensus.io/trace"
	"go.opencensus.io/trace/propagation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Microsoft/hcsshim/internal/log"
)

// The following helpers are inlined from the (removed) internal/oc package so
// that octtrpc remains self-contained while continuing to use OpenCensus. This
// package is deprecated (see the package doc); new code should use
// github.com/containerd/otelttrpc.

// defaultSampler mirrors the previous oc.DefaultSampler.
var defaultSampler = trace.AlwaysSample()

var (
	withClientSpanKind = trace.WithSpanKind(trace.SpanKindClient)
	withServerSpanKind = trace.WithSpanKind(trace.SpanKindServer)
)

// startSpan wraps "go.opencensus.io/trace".StartSpan, but, if the span is
// sampling, adds a log entry to the context that points to the newly created
// span.
func startSpan(ctx context.Context, name string, o ...trace.StartOption) (context.Context, *trace.Span) {
	ctx, s := trace.StartSpan(ctx, name, o...)
	return updateSpanContext(ctx, s)
}

// startSpanWithRemoteParent wraps
// "go.opencensus.io/trace".StartSpanWithRemoteParent.
//
// See startSpan for more information.
func startSpanWithRemoteParent(ctx context.Context, name string, parent trace.SpanContext, o ...trace.StartOption) (context.Context, *trace.Span) {
	ctx, s := trace.StartSpanWithRemoteParent(ctx, name, parent, o...)
	return updateSpanContext(ctx, s)
}

func updateSpanContext(ctx context.Context, s *trace.Span) (context.Context, *trace.Span) {
	if s.IsRecordingEvents() {
		ctx = log.UpdateContext(ctx)
	}
	return ctx, s
}

type options struct {
	sampler trace.Sampler
}

// Option represents an option function that can be used with the OC TTRPC
// interceptors.
type Option func(*options)

// WithSampler returns an option function to set the OC sampler used for the
// auto-created spans.
func WithSampler(sampler trace.Sampler) Option {
	return func(opts *options) {
		opts.sampler = sampler
	}
}

const metadataTraceContextKey = "octtrpc.tracecontext"

func convertMethodName(name string) string {
	name = strings.TrimPrefix(name, "/")
	name = strings.ReplaceAll(name, "/", ".")
	return name
}

func getParentSpanFromContext(ctx context.Context) (trace.SpanContext, bool) {
	md, _ := ttrpc.GetMetadata(ctx)
	traceContext := md[metadataTraceContextKey]
	if len(traceContext) > 0 {
		traceContextBinary, _ := base64.StdEncoding.DecodeString(traceContext[0])
		return propagation.FromBinary(traceContextBinary)
	}
	return trace.SpanContext{}, false
}

func setSpanStatus(span *trace.Span, err error) {
	// This error handling matches that used in ocgrpc.
	if err != nil {
		s, ok := status.FromError(err)
		if ok {
			span.SetStatus(trace.Status{Code: int32(s.Code()), Message: s.Message()})
		} else {
			span.SetStatus(trace.Status{Code: int32(codes.Internal), Message: err.Error()})
		}
	}
}

// ClientInterceptor returns a TTRPC unary client interceptor that automatically
// creates a new span for outgoing TTRPC calls, and passes the span context as
// metadata on the call.
func ClientInterceptor(opts ...Option) ttrpc.UnaryClientInterceptor {
	o := options{
		sampler: defaultSampler,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return func(ctx context.Context, req *ttrpc.Request, resp *ttrpc.Response, info *ttrpc.UnaryClientInfo, inv ttrpc.Invoker) (err error) {
		ctx, span := startSpan(
			ctx,
			convertMethodName(info.FullMethod),
			trace.WithSampler(o.sampler),
			withClientSpanKind)
		defer span.End()
		defer func() { setSpanStatus(span, err) }()

		spanContextBinary := propagation.Binary(span.SpanContext())
		b64 := base64.StdEncoding.EncodeToString(spanContextBinary)
		kvp := &ttrpc.KeyValue{Key: metadataTraceContextKey, Value: b64}
		req.Metadata = append(req.Metadata, kvp)

		return inv(ctx, req, resp)
	}
}

// ServerInterceptor returns a TTRPC unary server interceptor that automatically
// creates a new span for incoming TTRPC calls, and parents the span to the
// span context received via metadata, if it exists.
func ServerInterceptor(opts ...Option) ttrpc.UnaryServerInterceptor {
	o := options{
		sampler: defaultSampler,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return func(ctx context.Context, unmarshal ttrpc.Unmarshaler, info *ttrpc.UnaryServerInfo, method ttrpc.Method) (_ interface{}, err error) {
		name := convertMethodName(info.FullMethod)

		var span *trace.Span
		opts := []trace.StartOption{trace.WithSampler(o.sampler), withServerSpanKind}
		parent, ok := getParentSpanFromContext(ctx)
		if ok {
			ctx, span = startSpanWithRemoteParent(ctx, name, parent, opts...)
		} else {
			ctx, span = startSpan(ctx, name, opts...)
		}
		defer span.End()
		defer func() { setSpanStatus(span, err) }()

		return method(ctx, unmarshal)
	}
}
