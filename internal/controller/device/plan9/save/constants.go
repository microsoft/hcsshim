//go:build windows && lcow

// Package save defines the wire format owned by the Plan9 sub-controller
// for live migration. The [Payload] message is self-contained and carries the
// Plan9 sub-controller's serialized state (LCOW 9P file shares and their
// in-guest mounts) across shims.
package save

// SchemaVersion is the on-the-wire compatibility version stamped into
// [Payload.SchemaVersion]. Bump on any breaking change to payload.proto.
const SchemaVersion uint32 = 1

// TypeURL identifies a Plan9 [Payload] when wrapped in an [anypb.Any]. It is
// opaque to clients and only meaningful between two shims that agree on
// [SchemaVersion].
const TypeURL = "type.microsoft.com/hcsshim.controller.plan9.save.v1.Payload"
