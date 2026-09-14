// Package gcscompat defines the guest/host contract version that the GCS
// (guest) and hcsshim (host) exchange during protocol negotiation.
//
// Both the Windows host and the Linux guest compile the same constants from
// this package. Because the file carries no build constraint, the values can
// only differ at runtime when the two binaries were built from source commits
// that changed the contract. That makes the contract version a compact proxy
// for "were these two binaries built from contract-compatible source?", which
// the frozen bridge protocol version (prot.PvV4 = 4) cannot answer: the
// protocol version only bumps for an epochal bridge rewrite, whereas ordinary
// guest/host evolution happens within protocol version 4.
package gcscompat

const (
	// GuestHostContractVersion is the newest guest/host contract this binary
	// implements. Bump it in the same change that alters the guest/host
	// message contract in a way both sides must agree on (a change that is not
	// backward compatible).
	GuestHostContractVersion uint32 = 1

	// MinCompatibleContractVersion is the oldest peer contract this binary can
	// still interoperate with. Raise it only when support for older peers is
	// intentionally dropped.
	MinCompatibleContractVersion uint32 = 1
)

// Compatible reports whether a local contract range [localMin, localMax] and a
// remote contract range [remoteMin, remoteMax] intersect. Two peers can
// interoperate if and only if their advertised ranges overlap. The relation is
// symmetric, so either side can evaluate it.
func Compatible(localMin, localMax, remoteMin, remoteMax uint32) bool {
	return localMin <= remoteMax && remoteMin <= localMax
}
