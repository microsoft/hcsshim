//go:build windows

package vmutils

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

// OutputHandler processes the output stream from a program running in a UVM (Utility VM).
// It is responsible for reading, parsing, and handling the output data.
type OutputHandler func(io.Reader)

// OutputHandlerCreator is a factory function that creates an OutputHandler for a specific VM.
// It takes a VM ID and returns a configured OutputHandler for that VM's output.
type OutputHandlerCreator func(string) OutputHandler

// Ensure ParseGCSLogrus implements the OutputHandlerCreator type.
var _ OutputHandlerCreator = ParseGCSLogrus

// ParseGCSLogrus creates an OutputHandler that parses and logs GCS (Guest Compute Service) output.
// It processes JSON-formatted log entries from the GCS and forwards them to the host's logging system.
//
// The handler performs the following operations:
//   - Decodes JSON log entries from the GCS output stream
//   - Enriches each log entry with VM-specific metadata (VM ID and timestamp)
//   - Handles error conditions including disconnections and malformed input
//   - Captures and logs panic stacks or other non-JSON output from GCS termination
//
// Parameters:
//   - vmID: The unique identifier of the VM whose logs are being processed
//
// Returns:
//   - OutputHandler: A configured handler function for processing the VM's log stream
func ParseGCSLogrus(vmID string) OutputHandler {
	return func(r io.Reader) {
		// Create JSON decoder for streaming log entries
		j := json.NewDecoder(r)

		// Duplicate the base logger to avoid interfering with other log operations
		e := log.L.Dup()
		fields := e.Data

		// Process log entries continuously until error or EOF
		for {
			// Clear fields from previous iteration
			clear(fields)

			// Prepare new log entry with reused fields map
			gcsEntry := GCSLogEntry{Fields: fields}
			err := j.Decode(&gcsEntry)

			if err != nil {
				// Handle decoding errors, EOF, or disconnections
				// Log the error unless it's an expected EOF or network disconnect
				// (WSAECONNABORTED or WSAECONNRESET indicate expected shutdown/disconnect)
				if !errors.Is(err, io.EOF) && !hcs.IsAny(err, windows.WSAECONNABORTED, windows.WSAECONNRESET) {
					logrus.WithFields(logrus.Fields{
						logfields.UVMID: vmID,
						logrus.ErrorKey: err,
					}).Error("gcs log read")
				}

				// Read any remaining data (likely a panic stack trace)
				// and log it if non-empty
				rest, _ := io.ReadAll(io.MultiReader(j.Buffered(), r))
				rest = bytes.TrimSpace(rest)
				if len(rest) != 0 {
					logrus.WithFields(logrus.Fields{
						logfields.UVMID: vmID,
						"stderr":        string(rest),
					}).Error("gcs terminated")
				}
				break
			}

			// Enrich log entry with VM metadata
			fields[logfields.UVMID] = vmID
			fields["vm.time"] = gcsEntry.Time

			// Forward the log entry to the host logger with original level and message
			e.Log(gcsEntry.Level, gcsEntry.Message)
		}
	}
}

// GCSLogEntryStandard represents the standard fields of a GCS log entry.
// These fields are common across all log entries and map directly to JSON fields.
type GCSLogEntryStandard struct {
	Time    time.Time    `json:"time"`
	Level   logrus.Level `json:"level"`
	Message string       `json:"msg"`
}

// GCSLogEntry represents a complete GCS log entry including standard fields
// and any additional custom fields that may be present in the log output.
type GCSLogEntry struct {
	GCSLogEntryStandard
	Fields map[string]interface{}
}

// UnmarshalJSON implements json.Unmarshaler for GCSLogEntry.
// It performs custom unmarshaling with the following behaviors:
//   - Sets default log level to Info if not specified
//   - Clamps log levels to Error or above (prevents Fatal/Panic propagation)
//   - Handles ETW (Event Tracing for Windows) log entries with alternate message field names
//   - Removes standard fields (time, level, msg) from the Fields map to avoid duplication
//   - Normalizes floating-point numbers that are whole numbers to int64
//
// This method is optimized to minimize allocations and redundant map operations.
func (e *GCSLogEntry) UnmarshalJSON(b []byte) error {
	// Default the log level to info.
	e.Level = logrus.InfoLevel

	// Unmarshal standard fields first
	if err := json.Unmarshal(b, &e.GCSLogEntryStandard); err != nil {
		return err
	}

	// Unmarshal all fields including custom ones
	if err := json.Unmarshal(b, &e.Fields); err != nil {
		return err
	}

	// Do not allow fatal or panic level errors to propagate.
	if e.Level < logrus.ErrorLevel {
		e.Level = logrus.ErrorLevel
	}

	// Handle ETW (Event Tracing for Windows) log entries that may have
	// alternate message field names ("message" or "Message" instead of "msg")
	if e.Fields["Source"] == "ETW" {
		// Check for alternate message fields and use the first one found
		if msg, ok := e.Fields["message"].(string); ok {
			e.Message = msg
		} else if msg, ok := e.Fields["Message"].(string); ok {
			e.Message = msg
		}
	}

	// Batch delete standard and alternate fields to avoid duplication
	// This is more efficient than multiple individual delete calls
	fieldsToDelete := []string{"time", "level", "msg", "message", "Message"}
	for _, field := range fieldsToDelete {
		delete(e.Fields, field)
	}

	// Normalize floating-point values that represent whole numbers to int64.
	// This reduces type inconsistencies in log field values.
	for k, v := range e.Fields {
		if d, ok := v.(float64); ok && float64(int64(d)) == d {
			e.Fields[k] = int64(d)
		}
	}

	return nil
}
