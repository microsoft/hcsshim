package log

import (
	"bytes"
	"reflect"
	"time"

	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/containerd/containerd/log"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

const nullString = "null"

// Hook serves to intercept and format `logrus.Entry`s before they are passed
// to the ETW hook.
//
// The containerd shim discards the (formatted) logrus output, and outputs only via ETW.
// The Linux GCS outputs logrus entries over stdout, which is consumed by the shim and
// then re-output via the ETW hook.
type Hook struct {
	// EncodeAsJSON formats structs, maps, arrays, slices, and `bytes.Buffer` as JSON.
	// Variables of `bytes.Buffer` will be converted to `[]byte`.
	//
	// Default is true.
	EncodeAsJSON bool

	// FormatTime specifies the format for `time.Time` variables.
	// An empty string disabled formatting.
	//
	// Default is `"github.com/containerd/containerd/log".RFC3339NanoFixed`.
	TimeFormat string

	// AddSpanContext adds `logfields.TraceID` and `logfields.SpanID` fields to
	// the entry from the span context stored in `(*entry).Context`, if it exists.
	AddSpanContext bool
}

var _ logrus.Hook = &Hook{}

func NewHook() *Hook {
	return &Hook{
		EncodeAsJSON:   true,
		TimeFormat:     log.RFC3339NanoFixed,
		AddSpanContext: true,
	}
}

func (h *Hook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *Hook) Fire(e *logrus.Entry) (err error) {
	// JSON encode, if necessary, then add span information
	h.encode(e)
	h.addSpanContext(e)

	return nil
}

func (h *Hook) encode(e *logrus.Entry) {
	d := e.Data

	formatTime := len(h.TimeFormat) > 0
	if !(h.EncodeAsJSON || formatTime) {
		return
	}

	// todo: replace these with constraints.Integer, constraints.Float, etc with go1.18
	for k, v := range d {
		if formatTime {
			switch vv := v.(type) {
			case time.Time:
				d[k] = vv.Format(h.TimeFormat)
				continue
			}
		}

		if !h.EncodeAsJSON {
			continue
		}

		switch vv := v.(type) {
		// built in types
		case bool, string, error, uintptr,
			int8, int16, int32, int64, int,
			uint8, uint32, uint64, uint,
			float32, float64:
			continue

		case time.Duration:
			d[k] = vv.String()
			continue

		// `case bytes.Buffer,*bytes.Buffer` resolves `vv` to `interface{}`,
		// so cannot use `vv.Bytes`.
		// Could move to below the `reflect.Indirect()` call below, but
		// that would require additional typematching and dereferencing,
		// regardless.
		// Easier to keep these duplicate branches here.
		case bytes.Buffer:
			v = vv.Bytes()
		case *bytes.Buffer:
			v = vv.Bytes()
		}

		// dereference any pointers
		rv := reflect.Indirect(reflect.ValueOf(v))
		// check if `v` is a null pointer
		if !rv.IsValid() {
			d[k] = nullString
			continue
		}

		switch rv.Kind() {
		case reflect.Map, reflect.Struct, reflect.Array, reflect.Slice:
		default:
			continue
		}

		b, err := encode(v)
		if err != nil {
			// Errors are written to stderr (ie, to `panic.log`) and stops the remaining
			// hooks (ie, exporting to ETW) from firing. So add encoding errors to
			// the entry data to be written out, but keep on processing.
			d[k+"-"+logrus.ErrorKey] = err.Error()
		}

		// if  `err != nil`, then `b == nil` and this will be the empty string
		d[k] = string(b)
	}
}

func (h *Hook) addSpanContext(e *logrus.Entry) {
	ctx := e.Context
	if ctx == nil {
		return
	}
	span := trace.FromContext(ctx)
	if span == nil {
		return
	}
	sctx := span.SpanContext()
	e.Data[logfields.TraceID] = sctx.TraceID.String()
	e.Data[logfields.SpanID] = sctx.SpanID.String()
}
