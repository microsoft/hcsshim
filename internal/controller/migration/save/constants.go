//go:build windows && lcow

// Package save defines the top-level sandbox-level wire format used to hand
// off an LCOW sandbox between shims during live migration. The [Payload]
// envelope only carries opaque [anypb.Any] payloads owned by the VM
// controller and each pod controller; this package owns the envelope itself,
// not the inner controller schemas.
package save

// SchemaVersion is the on-the-wire compatibility version stamped into
// [Payload.SchemaVersion]. Bump on any breaking change to payload.proto.
const SchemaVersion uint32 = 1

// TypeURL identifies a sandbox-level [Payload] when wrapped in an [anypb.Any].
// It is opaque to clients and only meaningful between two shims that agree
// on [SchemaVersion].
const TypeURL = "type.microsoft.com/hcsshim.controller.migration.save.v1.Payload"
