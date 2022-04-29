// This package wraps differences between OpenCensus and OpenTelemetry until the switch
// to the later is complete.
package trace

import (
	"context"
	"strings"

	oc "go.opencensus.io/trace"
	"go.opencensus.io/trace/tracestate"
	otel "go.opentelemetry.io/otel/trace"
)

func GetSpanContext(ctx context.Context) *otel.SpanContext {
	if sc := otelSpanContext(ctx); sc != nil {
		return sc
	}
	return ocSpanContext(ctx)
}

func otelSpanContext(ctx context.Context) *otel.SpanContext {
	span := otel.SpanFromContext(ctx)
	if span == nil {
		return nil
	}
	sc := span.SpanContext()
	return &sc
}

func ocSpanContext(ctx context.Context) *otel.SpanContext {
	ocSpan := oc.FromContext(ctx)
	if ocSpan == nil {
		return nil
	}

	ocSC := ocSpan.SpanContext()
	config := otel.SpanContextConfig{
		TraceID:    otel.TraceID(ocSC.TraceID),
		SpanID:     otel.SpanID(ocSC.SpanID),
		TraceFlags: otel.TraceFlags(ocSC.TraceOptions),
	}

	if ts, err := otel.ParseTraceState(exportOCTracestate(ocSC.Tracestate)); err == nil {
		config.TraceState = ts
	}

	otSC := otel.NewSpanContext(config)
	return &otSC
}

func exportOCTracestate(ts *tracestate.Tracestate) string {
	b := &strings.Builder{}
	defer b.Reset()

	for i, e := range ts.Entries() {
		if i > 0 {
			if err := b.WriteByte(','); err != nil {
				return ""
			}
		}
		if _, err := b.WriteString(e.Key); err != nil {
			return ""
		}
		if err := b.WriteByte('='); err != nil {
			return ""
		}
		if _, err := b.WriteString(e.Value); err != nil {
			return ""
		}
	}

	return b.String()
}
