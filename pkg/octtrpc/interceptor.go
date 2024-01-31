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
	"google.golang.org/protobuf/proto"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
)

type options struct {
	sampler trace.Sampler
	attrs   []trace.Attribute
	// add the request/response messages as span attributes
	addMsg bool
	// hook to update/modify a copy of the request/response messages before adding them as span attributes
	msgAttrHook func(proto.Message)
}

func (o *options) msgHook(v any) any {
	m, ok := v.(proto.Message)
	if !ok || o.msgAttrHook == nil {
		return v
	}

	m = proto.Clone(m)
	o.msgAttrHook(m)
	return m
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

// WithAttributes specifies additional attributes to add to spans created by the interceptor.
func WithAttributes(attr ...trace.Attribute) Option {
	return func(opts *options) {
		opts.attrs = append(opts.attrs, attr...)
	}
}

// these are (currently) ServerInterceptor-specific options, but we cannot create a new [ServerOption] type
// since that would break our API

// WithAddMessage adds the request and response messages as attributes to the ttrpc method span.
//
// [ServerInterceptor] only.
func WithAddMessage() Option {
	return func(opts *options) {
		opts.addMsg = true
	}
}

// WithAddMessageHook specifies a hook to modify the ttrpc request or response messages
// before adding them to the ttrpc method span.
// This is intended to allow scrubbing sensitive fields from a the message.
//
// The function will be called with a clone of the original message (via [proto.Clone]) only if:
//   - the interceptor is created with the [WithAddMessage] option
//   - the function is non-nil
//   - the ttrpc request or response are of type [proto.Message]
//
// Since ttrpc is a gRPC replacement, we are guaranteed that the messages will
// implement [proto.Message].
//
// [ServerInterceptor] only.
func WithAddMessageHook(f func(proto.Message)) Option {
	return func(opts *options) {
		opts.msgAttrHook = f
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
		if len(o.attrs) > 0 {
			span.AddAttributes(o.attrs...)
		}

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

	return func(ctx context.Context, unmarshal ttrpc.Unmarshaler, info *ttrpc.UnaryServerInfo, method ttrpc.Method) (resp interface{}, err error) {
		name := convertMethodName(info.FullMethod)

		var span *trace.Span
		opts := []trace.StartOption{trace.WithSampler(o.sampler), oc.WithServerSpanKind}
		if parent, ok := getParentSpanFromContext(ctx); ok {
			ctx, span = oc.StartSpanWithRemoteParent(ctx, name, parent, opts...)
		} else {
			ctx, span = oc.StartSpan(ctx, name, opts...)
		}
		defer span.End()
		defer func() {
			if o.addMsg && err == nil {
				span.AddAttributes(trace.StringAttribute("response", log.Format(ctx, o.msgHook(resp))))
			}
			setSpanStatus(span, err)
		}()
		if len(o.attrs) > 0 {
			span.AddAttributes(o.attrs...)
		}

		return method(ctx, func(req interface{}) error {
			err := unmarshal(req)
			if o.addMsg {
				span.AddAttributes(trace.StringAttribute("request", log.Format(ctx, o.msgHook(req))))
			}
			return err
		})
	}
}
