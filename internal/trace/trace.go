// This package wraps differences between OpenCensus and OpenTelemetry until the switch
// to the later is complete.
package trace

import (
	"context"
	"strings"

	"github.com/Microsoft/hcsshim/internal/log"
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
	// otel returns a noop span, not nil
	if span.TracerProvider() == otel.NewNoopTracerProvider() {
		return nil
	}
	sc := span.SpanContext()
	return &sc
}

func ocSpanContext(ctx context.Context) *otel.SpanContext {
	span := oc.FromContext(ctx)
	if span == nil {
		return nil
	}

	sc := span.SpanContext()
	config := otel.SpanContextConfig{
		TraceID:    otel.TraceID(sc.TraceID),
		SpanID:     otel.SpanID(sc.SpanID),
		TraceFlags: otel.TraceFlags(sc.TraceOptions),
	}

	ts, err := otel.ParseTraceState(exportOCTracestate(sc.Tracestate))
	if err != nil {
		log.G(ctx).WithError(err).Warn("failed to encode OpenCensus Tracestate")
	} else {
		config.TraceState = ts
	}

	otSC := otel.NewSpanContext(config)
	return &otSC
}

func exportOCTracestate(ts *tracestate.Tracestate) string {
	es := ts.Entries()
	if len(es) == 0 {
		return ""
	}

	b := &strings.Builder{}
	// random assumption that entries are about 32 bytes total
	b.Grow(len(es) * 32)

	for i, e := range es {
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
