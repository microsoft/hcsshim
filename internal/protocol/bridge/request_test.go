package bridge

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"
	oc "go.opencensus.io/trace"
	"go.opencensus.io/trace/tracestate"
)

func Test_NewRequestBase_NoSpan(t *testing.T) {
	r := NewRequestBase(context.Background(), t.Name())

	if r.ContainerID != t.Name() {
		t.Fatalf("expected ContainerID: %q, got: %q", t.Name(), r.ContainerID)
	}
	var empty guid.GUID
	if r.ActivityID != empty {
		t.Fatalf("expected ActivityID empty, got: %q", r.ActivityID.String())
	}
	if r.OpenCensusSpanContext != nil {
		t.Fatal("expected nil span context")
	}
}

func Test_NewRequestBase_WithSpan(t *testing.T) {
	ctx, span := oc.StartSpan(context.Background(), t.Name())
	defer span.End()
	r := NewRequestBase(ctx, t.Name())

	if r.ContainerID != t.Name() {
		t.Fatalf("expected ContainerID: %q, got: %q", t.Name(), r.ContainerID)
	}
	var empty guid.GUID
	if r.ActivityID != empty {
		t.Fatalf("expected ActivityID empty, got: %q", r.ActivityID.String())
	}
	if r.OpenCensusSpanContext == nil {
		t.Fatal("expected non-nil span context")
	}

	sc := span.SpanContext()
	encodedTraceID := hex.EncodeToString(sc.TraceID[:])
	if r.OpenCensusSpanContext.TraceID().String() != encodedTraceID {
		t.Fatalf("expected encoded TraceID: %q, got: %q", encodedTraceID, r.OpenCensusSpanContext.TraceID())
	}
	encodedSpanID := hex.EncodeToString(sc.SpanID[:])
	if r.OpenCensusSpanContext.SpanID().String() != encodedSpanID {
		t.Fatalf("expected encoded SpanID: %q, got: %q", encodedSpanID, r.OpenCensusSpanContext.SpanID())
	}
	encodedTraceOptions := uint32(sc.TraceOptions)
	if uint32(r.OpenCensusSpanContext.TraceFlags()) != encodedTraceOptions {
		t.Fatalf("expected encoded TraceOptions: %v, got: %v", encodedTraceOptions, r.OpenCensusSpanContext.TraceFlags())
	}
	reqTS := r.OpenCensusSpanContext.TraceState()
	if reqTS.Len() > 0 {
		t.Fatalf("expected encoded TraceState: '', got: %q", reqTS.String())
	}
}

func Test_NewRequestBase_WithSpan_TraceStateEmptyEntries(t *testing.T) {
	// Start a remote context span so we can forward trace state.
	ts, err := tracestate.New(nil)
	if err != nil {
		t.Fatalf("failed to make test Tracestate")
	}
	parent := oc.SpanContext{
		Tracestate: ts,
	}
	ctx, span := oc.StartSpanWithRemoteParent(context.Background(), t.Name(), parent)
	defer span.End()
	r := NewRequestBase(ctx, t.Name())

	if r.OpenCensusSpanContext == nil {
		t.Fatal("expected non-nil span context")
	}
	reqTS := r.OpenCensusSpanContext.TraceState()
	if reqTS.Len() > 0 {
		t.Fatalf("expected encoded TraceState: '', got: %q", reqTS.String())
	}
}

func Test_NewRequestBase_WithSpan_TraceStateEntries(t *testing.T) {
	// Start a remote context span so we can forward trace state.
	ts, err := tracestate.New(nil, tracestate.Entry{Key: "test", Value: "test"})
	if err != nil {
		t.Fatalf("failed to make test Tracestate")
	}
	parent := oc.SpanContext{
		Tracestate: ts,
	}
	ctx, span := oc.StartSpanWithRemoteParent(context.Background(), t.Name(), parent)
	defer span.End()
	r := NewRequestBase(ctx, t.Name())

	if r.OpenCensusSpanContext == nil {
		t.Fatal("expected non-nil span context")
	}
	encodedTraceState := "test=test"
	reqTS := r.OpenCensusSpanContext.TraceState()
	if reqTS.String() != encodedTraceState {
		t.Fatalf("expected encoded TraceState: %q, got: %q", encodedTraceState, reqTS)
	}
}
