package oc

import (
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
)

const spanMessage = "Span"

var _errorCodeKey = logrus.ErrorKey + "Code"

// LogrusExporter is an OpenCensus `trace.Exporter` that exports
// `trace.SpanData` to logrus output.
type LogrusExporter struct{}

var _ trace.Exporter = &LogrusExporter{}

// ExportSpan exports `s` based on the the following rules:
//
// 1. All output will contain `s.Attributes`, `s.SpanKind`, `s.TraceID`,
// `s.SpanID`, and `s.ParentSpanID` for correlation
//
// 2. Any calls to .Annotate will not be supported.
//
// 3. The span itself will be written at `logrus.InfoLevel` unless
// `s.Status.Code != 0` in which case it will be written at `logrus.ErrorLevel`
// providing `s.Status.Message` as the error value.
func (le *LogrusExporter) ExportSpan(s *trace.SpanData) {
	if s.DroppedAnnotationCount > 0 {
		logrus.WithFields(logrus.Fields{
			"name":            s.Name,
			logfields.TraceID: s.TraceID.String(),
			logfields.SpanID:  s.SpanID.String(),
			"dropped":         s.DroppedAttributeCount,
			"maxAttributes":   len(s.Attributes),
		}).Warning("span had dropped attributes")
	}

	// Combine all span annotations with traceID, spanID, parentSpanID, and error
	entry := logrus.WithFields(logrus.Fields(s.Attributes))
	// All span fields are string, so we can safely skip overhead in entry.WithFields
	// and add them directly to entry.Data.
	// Avoid growing the entry.Data map and reallocating buckets by creating new
	// `logrus.Fields` and copying data over.
	data := make(logrus.Fields, len(entry.Data)+10) // should only add 10 new entries, max
	//copy old data
	for k, v := range entry.Data {
		data[k] = v
	}
	data[logfields.Name] = s.Name
	data[logfields.TraceID] = s.TraceID.String()
	data[logfields.SpanID] = s.SpanID.String()
	data[logfields.ParentSpanID] = s.ParentSpanID.String()
	data[logfields.StartTime] = log.FormatTime(s.StartTime)
	data[logfields.EndTime] = log.FormatTime(s.EndTime)
	data[logfields.Duration] = s.EndTime.Sub(s.StartTime).String()
	if sk := spanKindToString(s.SpanKind); sk != "" {
		data["spanKind"] = sk
	}

	level := logrus.InfoLevel
	if s.Status.Code != 0 {
		level = logrus.ErrorLevel
		data[logrus.ErrorKey] = s.Status.Message

		if _, ok := data[_errorCodeKey]; !ok {
			data[_errorCodeKey] = s.Status.Code
		}
	}

	entry.Data = data
	entry.Time = s.StartTime
	entry.Log(level, spanMessage)
}

// GetSpanName checks if the entry appears to be span entry exported by the `LogrusExporter`
// and returns the the span name.
//
// This is intended to be used with the winio ETW exporter.
func GetSpanName(e *logrus.Entry) string {
	if s, ok := (e.Data[logfields.Name]).(string); ok && e.Message == spanMessage {
		return s
	}

	return ""
}
