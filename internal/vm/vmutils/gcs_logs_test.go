//go:build windows

package vmutils

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestGCSLogEntry_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedLevel  logrus.Level
		expectedMsg    string
		expectedFields map[string]interface{}
		wantErr        bool
	}{
		{
			name:          "basic log entry",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"test message"}`,
			expectedLevel: logrus.InfoLevel,
			expectedMsg:   "test message",
		},
		{
			name:          "debug level becomes info",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"debug","msg":"debug message"}`,
			expectedLevel: logrus.DebugLevel,
			expectedMsg:   "debug message",
		},
		{
			name:          "error level stays error",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"error","msg":"error message"}`,
			expectedLevel: logrus.ErrorLevel,
			expectedMsg:   "error message",
		},
		{
			name:          "fatal level clamped to error",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"fatal","msg":"fatal message"}`,
			expectedLevel: logrus.ErrorLevel,
			expectedMsg:   "fatal message",
		},
		{
			name:          "panic level clamped to error",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"panic","msg":"panic message"}`,
			expectedLevel: logrus.ErrorLevel,
			expectedMsg:   "panic message",
		},
		{
			name:          "missing level defaults to info",
			input:         `{"time":"2024-01-15T10:30:00Z","msg":"message without level"}`,
			expectedLevel: logrus.InfoLevel,
			expectedMsg:   "message without level",
		},
		{
			name:          "ETW source with message field",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"","Source":"ETW","message":"etw message"}`,
			expectedLevel: logrus.InfoLevel,
			expectedMsg:   "etw message",
		},
		{
			name:          "ETW source with Message field (capitalized)",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"","Source":"ETW","Message":"ETW capitalized message"}`,
			expectedLevel: logrus.InfoLevel,
			expectedMsg:   "ETW capitalized message",
		},
		{
			name:          "custom fields preserved",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"test","customField":"customValue","numericField":42}`,
			expectedLevel: logrus.InfoLevel,
			expectedMsg:   "test",
			expectedFields: map[string]interface{}{
				"customField":  "customValue",
				"numericField": int64(42), // whole numbers normalized to int64
			},
		},
		{
			name:          "floating point whole number normalized to int64",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"test","count":100.0}`,
			expectedLevel: logrus.InfoLevel,
			expectedMsg:   "test",
			expectedFields: map[string]interface{}{
				"count": int64(100),
			},
		},
		{
			name:          "floating point with decimal preserved",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"test","ratio":3.14}`,
			expectedLevel: logrus.InfoLevel,
			expectedMsg:   "test",
			expectedFields: map[string]interface{}{
				"ratio": 3.14,
			},
		},
		{
			name:    "invalid json",
			input:   `{invalid json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := make(map[string]interface{})
			entry := GCSLogEntry{Fields: fields}

			err := json.Unmarshal([]byte(tt.input), &entry)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if entry.Level != tt.expectedLevel {
				t.Errorf("level = %v, want %v", entry.Level, tt.expectedLevel)
			}

			if entry.Message != tt.expectedMsg {
				t.Errorf("message = %q, want %q", entry.Message, tt.expectedMsg)
			}

			// Check that standard fields are removed from Fields map
			standardFields := []string{"time", "level", "msg", "message", "Message"}
			for _, f := range standardFields {
				if _, exists := entry.Fields[f]; exists {
					t.Errorf("standard field %q should be removed from Fields map", f)
				}
			}

			// Check expected custom fields
			for k, v := range tt.expectedFields {
				if entry.Fields[k] != v {
					t.Errorf("field %q = %v (%T), want %v (%T)", k, entry.Fields[k], entry.Fields[k], v, v)
				}
			}
		})
	}
}

func TestGCSLogEntry_UnmarshalJSON_Time(t *testing.T) {
	input := `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"test"}`
	fields := make(map[string]interface{})
	entry := GCSLogEntry{Fields: fields}

	err := json.Unmarshal([]byte(input), &entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if !entry.Time.Equal(expectedTime) {
		t.Errorf("time = %v, want %v", entry.Time, expectedTime)
	}
}

func TestGCSLogEntryStandard(t *testing.T) {
	input := `{"time":"2024-01-15T10:30:00Z","level":"warning","msg":"warning message"}`
	var entry GCSLogEntryStandard

	err := json.Unmarshal([]byte(input), &entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.Level != logrus.WarnLevel {
		t.Errorf("level = %v, want %v", entry.Level, logrus.WarnLevel)
	}

	if entry.Message != "warning message" {
		t.Errorf("message = %q, want %q", entry.Message, "warning message")
	}
}

func TestParseGCSLogrus(t *testing.T) {
	// Create a hook to capture log entries
	hook := &testLogHook{}
	originalOutput := logrus.StandardLogger().Out
	logrus.SetOutput(io.Discard)
	logrus.AddHook(hook)
	defer func() {
		logrus.SetOutput(originalOutput)
		// Remove the hook by creating a new logger
		logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	}()

	tests := []struct {
		name          string
		vmID          string
		input         string
		expectedCount int
		validate      func(t *testing.T, entries []*logrus.Entry)
	}{
		{
			name:          "single log entry",
			vmID:          "test-vm-1",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"test message"}` + "\n",
			expectedCount: 1,
			validate: func(t *testing.T, entries []*logrus.Entry) {
				t.Helper()
				if entries[0].Message != "test message" {
					t.Errorf("message = %q, want %q", entries[0].Message, "test message")
				}
				if entries[0].Data["uvm-id"] != "test-vm-1" {
					t.Errorf("uvm-id = %v, want %v", entries[0].Data["uvm-id"], "test-vm-1")
				}
			},
		},
		{
			name:          "multiple log entries",
			vmID:          "test-vm-2",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"first"}` + "\n" + `{"time":"2024-01-15T10:30:01Z","level":"warning","msg":"second"}` + "\n",
			expectedCount: 2,
			validate: func(t *testing.T, entries []*logrus.Entry) {
				t.Helper()
				if entries[0].Message != "first" {
					t.Errorf("first message = %q, want %q", entries[0].Message, "first")
				}
				if entries[1].Message != "second" {
					t.Errorf("second message = %q, want %q", entries[1].Message, "second")
				}
			},
		},
		{
			name:          "empty input",
			vmID:          "test-vm-3",
			input:         "",
			expectedCount: 0,
		},
		{
			name:          "entry with custom fields",
			vmID:          "test-vm-4",
			input:         `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"test","customKey":"customValue"}` + "\n",
			expectedCount: 1,
			validate: func(t *testing.T, entries []*logrus.Entry) {
				t.Helper()
				if entries[0].Data["customKey"] != "customValue" {
					t.Errorf("customKey = %v, want %v", entries[0].Data["customKey"], "customValue")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook.Reset()

			reader := strings.NewReader(tt.input)
			handler := ParseGCSLogrus(tt.vmID)
			handler(reader)

			if len(hook.Entries) != tt.expectedCount {
				t.Errorf("captured %d entries, want %d", len(hook.Entries), tt.expectedCount)
			}

			if tt.validate != nil && len(hook.Entries) > 0 {
				tt.validate(t, hook.Entries)
			}
		})
	}
}

func TestParseGCSLogrus_InvalidJSON(t *testing.T) {
	hook := &testLogHook{}
	originalOutput := logrus.StandardLogger().Out
	logrus.SetOutput(io.Discard)
	logrus.AddHook(hook)
	defer func() {
		logrus.SetOutput(originalOutput)
		logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	}()

	// Test with invalid JSON followed by valid trailing content (simulating panic stack)
	input := `{"invalid json`
	reader := strings.NewReader(input)

	handler := ParseGCSLogrus("test-vm")
	handler(reader)

	// Should capture error log for read failure and/or termination message
	// The exact behavior depends on whether there's trailing content
	// At minimum, should not panic
}

func TestParseGCSLogrus_TrailingContent(t *testing.T) {
	hook := &testLogHook{}
	originalOutput := logrus.StandardLogger().Out
	logrus.SetOutput(io.Discard)
	logrus.AddHook(hook)
	defer func() {
		logrus.SetOutput(originalOutput)
		logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	}()

	// Valid entry followed by panic stack trace
	input := `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"last message"}
panic: something went wrong
goroutine 1 [running]:
main.main()
	/app/main.go:10 +0x45`

	reader := strings.NewReader(input)
	handler := ParseGCSLogrus("test-vm")
	handler(reader)

	// Should have processed the valid entry and logged the panic stack
	found := false
	for _, entry := range hook.Entries {
		if entry.Message == "last message" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'last message' entry")
	}

	// Should also have logged the "gcs terminated" error with stderr content
	foundTerminated := false
	for _, entry := range hook.Entries {
		if entry.Message == "gcs terminated" {
			foundTerminated = true
			stderr, ok := entry.Data["stderr"].(string)
			if !ok {
				t.Error("stderr field not found or not a string")
			} else if !strings.Contains(stderr, "panic: something went wrong") {
				t.Errorf("stderr = %q, should contain panic message", stderr)
			}
			break
		}
	}
	if !foundTerminated {
		t.Error("expected to find 'gcs terminated' entry")
	}
}

func TestParseGCSLogrus_VMTimeField(t *testing.T) {
	hook := &testLogHook{}
	originalOutput := logrus.StandardLogger().Out
	logrus.SetOutput(io.Discard)
	logrus.AddHook(hook)
	defer func() {
		logrus.SetOutput(originalOutput)
		logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	}()

	input := `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"test"}` + "\n"
	reader := strings.NewReader(input)

	handler := ParseGCSLogrus("test-vm")
	handler(reader)

	if len(hook.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(hook.Entries))
	}

	vmTime, ok := hook.Entries[0].Data["vm.time"]
	if !ok {
		t.Error("vm.time field not found")
	}

	expectedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if vmTimeTyped, ok := vmTime.(time.Time); ok {
		if !vmTimeTyped.Equal(expectedTime) {
			t.Errorf("vm.time = %v, want %v", vmTimeTyped, expectedTime)
		}
	} else {
		t.Errorf("vm.time is not time.Time, got %T", vmTime)
	}
}

func TestOutputHandlerCreator_Interface(t *testing.T) {
	// Verify that ParseGCSLogrus satisfies the OutputHandlerCreator interface
	var creator OutputHandlerCreator = ParseGCSLogrus
	handler := creator("test-vm")
	if handler == nil {
		t.Error("ParseGCSLogrus should return a non-nil handler")
	}
}

// testLogHook is a logrus hook that captures log entries for testing
type testLogHook struct {
	Entries []*logrus.Entry
}

func (h *testLogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *testLogHook) Fire(entry *logrus.Entry) error {
	h.Entries = append(h.Entries, entry)
	return nil
}

func (h *testLogHook) Reset() {
	h.Entries = nil
}

// BenchmarkGCSLogEntry_UnmarshalJSON benchmarks JSON unmarshaling performance
func BenchmarkGCSLogEntry_UnmarshalJSON(b *testing.B) {
	input := []byte(`{"time":"2024-01-15T10:30:00Z","level":"info","msg":"benchmark test message","field1":"value1","field2":42,"field3":3.14}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fields := make(map[string]interface{})
		entry := GCSLogEntry{Fields: fields}
		_ = json.Unmarshal(input, &entry)
	}
}

// BenchmarkParseGCSLogrus benchmarks the log parsing handler
func BenchmarkParseGCSLogrus(b *testing.B) {
	logrus.SetOutput(io.Discard)

	input := `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"benchmark message","field":"value"}
{"time":"2024-01-15T10:30:01Z","level":"warning","msg":"second message","count":42}
{"time":"2024-01-15T10:30:02Z","level":"error","msg":"third message","error":"something failed"}
`
	inputBytes := []byte(input)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(inputBytes)
		handler := ParseGCSLogrus("bench-vm")
		handler(reader)
	}
}
