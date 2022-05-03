package trace

import (
	"testing"

	"go.opencensus.io/trace/tracestate"
	otel "go.opentelemetry.io/otel/trace"
)

func TestExportTracestae(t *testing.T) {
	entries := map[string]string{
		"hello":             "world",
		"hi":                "back",
		"sdfs20394lskdjfsl": "random keybash",
		"sdfks@xdsd":        "id",
	}

	es := make([]tracestate.Entry, 0, len(entries))
	for k, v := range entries {
		es = append(es, tracestate.Entry{Key: k, Value: v})
	}

	ts, err := tracestate.New(nil, es...)
	if err != nil {
		t.Fatalf("could not create tracestate: %v", err)
	}

	s := exportOCTracestate(ts)
	if s == "" {
		t.Fatalf("could not export tracestate %v", ts)
	}
	if len(ts.Entries()) != len(entries) {
		t.Fatalf("could not add all entries %v to tracestate %v", entries, ts)
	}

	otTS, err := otel.ParseTraceState(s)
	if err != nil {
		t.Fatalf("could not parse tracestate %v: %v", ts, err)
	}

	if otTS.Len() != len(entries) {
		t.Fatalf("could not add all entries %v to otel tracestate %v", entries, otTS)
	}

	for k, v := range entries {
		if vv := otTS.Get(k); vv != v {
			t.Fatalf("got %q with key %q, wanted %v", vv, k, v)
		}
	}
}
