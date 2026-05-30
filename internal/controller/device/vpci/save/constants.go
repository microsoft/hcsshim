//go:build windows && (lcow || wcow)

// Package save defines the wire format owned by the VPCI sub-controller for
// live migration. The [Payload] message is self-contained and carries the
// VPCI sub-controller's serialized state (assigned VPCI devices and their
// reference counts) across shims.
package save

// SchemaVersion is the on-the-wire compatibility version stamped into
// [Payload.SchemaVersion]. Bump on any breaking change to payload.proto.
const SchemaVersion uint32 = 1

// TypeURL identifies a VPCI [Payload] when wrapped in an [anypb.Any]. It is
// opaque to clients and only meaningful between two shims that agree on
// [SchemaVersion].
const TypeURL = "type.microsoft.com/hcsshim.controller.vpci.save.v1.Payload"
