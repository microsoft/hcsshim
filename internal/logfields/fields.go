package logfields

const (
	// Identifiers

	Name      = "name"
	Namespace = "namespace"
	Operation = "operation"

	ID          = "id"
	SandboxID   = "sid"
	ContainerID = "cid"
	ExecID      = "eid"
	ProcessID   = "pid"
	TaskID      = "tid"
	UVMID       = "uvm-id"

	// networking and IO

	Bytes = "bytes"
	File  = "file"
	Path  = "path"
	Pipe  = "pipe"

	// Common Misc

	Attempt = "attemptNo"
	JSON    = "json"

	// Status

	ExitCode = "exitCode"

	// Time

	Duration  = "duration"
	EndTime   = "endTime"
	StartTime = "startTime"
	Timeout   = "timeout"

	// Keys/Values

	Doc           = "document"
	Field         = "field"
	Key           = "key"
	OCIAnnotation = "oci-annotation"
	Options       = "options"
	Value         = "value"

	// Golang type's

	ExpectedType = "expected-type"
	Bool         = "bool"
	Uint32       = "uint32"
	Uint64       = "uint64"

	// runhcs

	VMShimOperation = "vmshim-op"

	// logging and tracing

	TraceID      = "traceID"
	SpanID       = "spanID"
	ParentSpanID = "parentSpanID"
)
