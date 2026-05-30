//go:build windows && (lcow || wcow)

// Package save defines the wire format owned by the VM controller for
// live migration. The [Payload] envelope carries the VM's bookkeeping plus
// the sub-device controller states (SCSI, VPCI, Plan9) as opaque
// [anypb.Any] payloads; this package owns the envelope itself, not the
// inner sub-controller schemas.
package save

// SchemaVersion is the on-the-wire compatibility version stamped into
// [Payload.SchemaVersion]. Bump on any breaking change to payload.proto.
const SchemaVersion uint32 = 1

// TypeURL identifies a VM [Payload] when wrapped in an [anypb.Any]. It is
// opaque to clients and only meaningful between two shims that agree on
// [SchemaVersion].
const TypeURL = "type.microsoft.com/hcsshim.controller.vm.save.v1.Payload"
