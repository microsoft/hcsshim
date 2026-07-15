//go:build windows && (lcow || wcow)

// Package save defines the wire format owned by the network controller for
// live migration. The [Payload] message is self-contained and carries the
// network controller's serialized state across shims.
package save

// SchemaVersion is the on-the-wire compatibility version stamped into
// [Payload.SchemaVersion]. Bump on any breaking change to payload.proto.
const SchemaVersion uint32 = 1

// TypeURL identifies a network [Payload] when wrapped in an [anypb.Any]. It is
// opaque to clients and only meaningful between two shims that agree on
// [SchemaVersion].
const TypeURL = "type.microsoft.com/hcsshim.controller.network.save.v1.Payload"
