package otelttrpc

import (
	"context"
	"strings"

	"github.com/containerd/ttrpc"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Microsoft/hcsshim/internal/otelutil"
)

const (
	metadataTraceContextKey = "otelttrpc.tracecontext"
	metadataTraceParent     = metadataTraceContextKey + ".traceparent"
	metadataTraceState      = metadataTraceContextKey + ".tracestate"
)

var propagator = propagation.TraceContext{}

func convertMethodName(name string) string {
	name = strings.TrimPrefix(name, "/")
	name = strings.Replace(name, "/", ".", -1)
	return name
}

func extractParentSpan(ctx context.Context) context.Context {
	var traceParent string
	var traceState string

	md, _ := ttrpc.GetMetadata(ctx)
	if tp, ok := md[metadataTraceContextKey]; ok && len(tp) > 0 {
		traceParent = tp[0]
	}
	if ts, ok := md[metadataTraceState]; ok && len(ts) > 0 {
		traceState = ts[0]
	}

	return propagator.Extract(ctx, propagation.MapCarrier{
		"traceparent": traceParent,
		"tracestate":  traceState,
	})
}

func setSpanStatus(span trace.Span, err error) {
	// This error handling matches that used in ocgrpc.
	if err != nil {
		s, ok := status.FromError(err)
		if ok {
			span.SetStatus(codes.Code(s.Code()), s.Message())
		} else {
			span.SetStatus(codes.Code(grpccodes.Internal), err.Error())
		}
	}
}

// ClientInterceptor returns a TTRPC unary client interceptor that automatically
// creates a new span for outgoing TTRPC calls, and passes the span context as
// metadata on the call.
func ClientInterceptor() ttrpc.UnaryClientInterceptor {
	return func(ctx context.Context, req *ttrpc.Request, resp *ttrpc.Response, info *ttrpc.UnaryClientInfo, inv ttrpc.Invoker) (err error) {
		ctx, span := otelutil.StartSpan(ctx, convertMethodName(info.FullMethod), otelutil.WithClientSpanKind)
		defer span.End()
		defer setSpanStatus(span, err)

		carrier := propagation.MapCarrier{}
		propagator.Inject(ctx, carrier)

		req.Metadata = append(req.Metadata,
			&ttrpc.KeyValue{Key: metadataTraceParent, Value: carrier.Get("traceparent")},
			&ttrpc.KeyValue{Key: metadataTraceState, Value: carrier.Get("tracestate")},
		)

		return inv(ctx, req, resp)
	}
}

// ServerInterceptor returns a TTRPC unary server interceptor that automatically
// creates a new span for incoming TTRPC calls, and parents the span to the
// span context received via metadata, if it exists.
func ServerInterceptor() ttrpc.UnaryServerInterceptor {
	return func(ctx context.Context, unmarshal ttrpc.Unmarshaler, info *ttrpc.UnaryServerInfo, method ttrpc.Method) (_ interface{}, err error) {
		name := convertMethodName(info.FullMethod)

		ctx, span := otelutil.StartSpan(extractParentSpan(ctx), name, otelutil.WithServerSpanKind)
		defer span.End()
		defer setSpanStatus(span, err)

		return method(ctx, unmarshal)
	}
}
