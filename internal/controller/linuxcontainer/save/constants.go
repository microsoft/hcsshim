//go:build windows && lcow

// Package save defines the wire format owned by the linuxcontainer
// controller for live migration. The [Payload] envelope carries the
// container's bookkeeping plus child process states as opaque [anypb.Any]
// payloads; this package owns the envelope itself, not the inner process
// controller schema.
package save

// SchemaVersion is the on-the-wire compatibility version stamped into
// [Payload.SchemaVersion]. Bump on any breaking change to payload.proto.
const SchemaVersion uint32 = 1

// TypeURL identifies a linuxcontainer [Payload] when wrapped in an
// [anypb.Any]. It is opaque to clients and only meaningful between two
// shims that agree on [SchemaVersion].
const TypeURL = "type.microsoft.com/hcsshim.controller.linuxcontainer.save.v1.Payload"
