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

	"github.com/Microsoft/hcsshim/internal/oc"
)

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
	name = strings.Replace(name, "/", ".", -1)
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
		sampler: oc.DefaultSampler,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return func(ctx context.Context, req *ttrpc.Request, resp *ttrpc.Response, info *ttrpc.UnaryClientInfo, inv ttrpc.Invoker) (err error) {
		ctx, span := oc.StartSpan(
			ctx,
			convertMethodName(info.FullMethod),
			trace.WithSampler(o.sampler),
			oc.WithClientSpanKind)
		defer span.End()
		defer setSpanStatus(span, err)

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
		sampler: oc.DefaultSampler,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return func(ctx context.Context, unmarshal ttrpc.Unmarshaler, info *ttrpc.UnaryServerInfo, method ttrpc.Method) (_ interface{}, err error) {
		name := convertMethodName(info.FullMethod)

		var span *trace.Span
		opts := []trace.StartOption{trace.WithSampler(o.sampler), oc.WithServerSpanKind}
		parent, ok := getParentSpanFromContext(ctx)
		if ok {
			ctx, span = oc.StartSpanWithRemoteParent(ctx, name, parent, opts...)
		} else {
			ctx, span = oc.StartSpan(ctx, name, opts...)
		}
		defer span.End()
		defer setSpanStatus(span, err)

		return method(ctx, unmarshal)
	}
}
