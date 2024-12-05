package octtrpc

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/containerd/ttrpc"
	"go.opencensus.io/trace"
	"go.opencensus.io/trace/propagation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type spanExporter struct {
	spans []*trace.SpanData
}

func (e *spanExporter) ExportSpan(s *trace.SpanData) {
	e.spans = append(e.spans, s)
}

func TestClientInterceptor(t *testing.T) {
	for name, tc := range map[string]struct {
		methodName       string
		expectedSpanName string
		invokeErr        error
		expectedStatus   trace.Status
	}{
		"callWithMethodName": {
			methodName:       "TestMethod",
			expectedSpanName: "TestMethod",
		},
		"callWithWeirdSpanName": {
			methodName:       "/Test/Method/Foo",
			expectedSpanName: "Test.Method.Foo",
		},
		"callFailsWithGenericError": {
			invokeErr:      fmt.Errorf("generic error"),
			expectedStatus: trace.Status{Code: 13, Message: "generic error"},
		},
		"callFailsWithGRPCError": {
			invokeErr:      status.Error(codes.AlreadyExists, "already exists"),
			expectedStatus: trace.Status{Code: int32(codes.AlreadyExists), Message: "already exists"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			exporter := &spanExporter{}
			trace.RegisterExporter(exporter)
			interceptor := ClientInterceptor(WithSampler(trace.AlwaysSample()))

			methodName := tc.methodName
			if methodName == "" {
				methodName = "TestMethod"
			}

			var md []*ttrpc.KeyValue

			_ = interceptor(
				context.Background(),
				&ttrpc.Request{},
				&ttrpc.Response{},
				&ttrpc.UnaryClientInfo{
					FullMethod: methodName,
				},
				func(ctx context.Context, req *ttrpc.Request, resp *ttrpc.Response) error {
					md = req.Metadata
					return tc.invokeErr
				},
			)

			if len(exporter.spans) != 1 {
				t.Fatalf("expected exporter to have 1 span but got %d", len(exporter.spans))
			}
			span := exporter.spans[0]
			if tc.expectedSpanName != "" && span.Name != tc.expectedSpanName {
				t.Errorf("expected span name %s but got %s", tc.expectedSpanName, span.Name)
			}
			if span.SpanKind != trace.SpanKindClient {
				t.Errorf("expected client span kind but got %v", span.SpanKind)
			}
			var spanMD string
			for _, kvp := range md {
				if kvp.Key == metadataTraceContextKey {
					spanMD = kvp.Value
					break
				}
			}
			if spanMD == "" {
				t.Error("expected span metadata in the request")
			} else {
				expectedSpanMD := base64.StdEncoding.EncodeToString(propagation.Binary(span.SpanContext))
				if spanMD != expectedSpanMD {
					t.Errorf("expected span metadata %s but got %s", expectedSpanMD, spanMD)
				}
			}
			if span.Status != tc.expectedStatus {
				t.Errorf("expected status %+v but got %+v", tc.expectedStatus, span.Status)
			}
		})
	}
}

func TestServerInterceptor(t *testing.T) {
	for name, tc := range map[string]struct {
		methodName       string
		expectedSpanName string
		methodErr        error
		expectedStatus   trace.Status
		parentSpan       *trace.SpanContext
	}{
		"callWithMethodName": {
			methodName:       "TestMethod",
			expectedSpanName: "TestMethod",
		},
		"callWithWeirdSpanName": {
			methodName:       "/Test/Method/Foo",
			expectedSpanName: "Test.Method.Foo",
		},
		"callWithRemoteSpanParent": {
			parentSpan: &trace.SpanContext{
				TraceID: trace.TraceID{0xa0, 0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8, 0xa9, 0xaa, 0xab, 0xac, 0xad, 0xae, 0xaf},
				SpanID:  trace.SpanID{0xb0, 0xb1, 0xb2, 0xb3, 0xb4, 0xb5, 0xb6, 0xb7},
			},
		},
		"callFailsWithGenericError": {
			methodErr:      fmt.Errorf("generic error"),
			expectedStatus: trace.Status{Code: 13, Message: "generic error"},
		},
		"callFailsWithGRPCError": {
			methodErr:      status.Error(codes.AlreadyExists, "already exists"),
			expectedStatus: trace.Status{Code: int32(codes.AlreadyExists), Message: "already exists"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			exporter := &spanExporter{}
			trace.RegisterExporter(exporter)
			interceptor := ServerInterceptor(WithSampler(trace.AlwaysSample()))

			ctx := context.Background()
			if tc.parentSpan != nil {
				ctx = ttrpc.WithMetadata(ctx, ttrpc.MD{metadataTraceContextKey: []string{base64.StdEncoding.EncodeToString(propagation.Binary(*tc.parentSpan))}})
			}
			methodName := tc.methodName
			if methodName == "" {
				methodName = "TestMethod"
			}

			_, _ = interceptor(
				ctx,
				nil,
				&ttrpc.UnaryServerInfo{
					FullMethod: methodName,
				},
				func(ctx context.Context, unmarshal func(interface{}) error) (interface{}, error) {
					return nil, tc.methodErr
				},
			)

			if len(exporter.spans) != 1 {
				t.Fatalf("expected exporter to have 1 span but got %d", len(exporter.spans))
			}
			span := exporter.spans[0]
			if tc.expectedSpanName != "" && span.Name != tc.expectedSpanName {
				t.Errorf("expected span name %s but got %s", tc.expectedSpanName, span.Name)
			}
			if span.SpanKind != trace.SpanKindServer {
				t.Errorf("expected server span kind but got %v", span.SpanKind)
			}
			if tc.parentSpan != nil {
				if span.TraceID != tc.parentSpan.TraceID {
					t.Errorf("expected trace id %v but got %v", tc.parentSpan.TraceID, span.TraceID)
				}
				if span.ParentSpanID != tc.parentSpan.SpanID {
					t.Errorf("expected parent span id %v but got %v", tc.parentSpan.SpanID, span.ParentSpanID)
				}
				if !span.HasRemoteParent {
					t.Error("expected span to have remote parent")
				}
			}
			if span.Status != tc.expectedStatus {
				t.Errorf("expected status %+v but got %+v", tc.expectedStatus, span.Status)
			}
		})
	}
}
