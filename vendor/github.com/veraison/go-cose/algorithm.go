package cose

import (
	"crypto"
	"strconv"
)

// Algorithms supported by this library.
//
// When using an algorithm which requires hashing,
// make sure the associated hash function is linked to the binary.
const (
	// RSASSA-PSS w/ SHA-256 by RFC 8230.
	// Requires an available crypto.SHA256.
	AlgorithmPS256 Algorithm = -37

	// RSASSA-PSS w/ SHA-384 by RFC 8230.
	// Requires an available crypto.SHA384.
	AlgorithmPS384 Algorithm = -38

	// RSASSA-PSS w/ SHA-512 by RFC 8230.
	// Requires an available crypto.SHA512.
	AlgorithmPS512 Algorithm = -39

	// ECDSA w/ SHA-256 by RFC 8152.
	// Requires an available crypto.SHA256.
	AlgorithmES256 Algorithm = -7

	// ECDSA w/ SHA-384 by RFC 8152.
	// Requires an available crypto.SHA384.
	AlgorithmES384 Algorithm = -35

	// ECDSA w/ SHA-512 by RFC 8152.
	// Requires an available crypto.SHA512.
	AlgorithmES512 Algorithm = -36

	// PureEdDSA by RFC 8152.
	//
	// Deprecated: use AlgorithmEdDSA instead, which has
	// the same value but with a more accurate name.
	AlgorithmEd25519 Algorithm = -8

	// PureEdDSA by RFC 8152.
	AlgorithmEdDSA Algorithm = -8

	// Reserved value.
	AlgorithmReserved Algorithm = 0
)

// Algorithms known, but not supported by this library.
//
// Signers and Verifiers requiring the algorithms below are not
// directly supported by this library. They need to be provided
// as an external [cose.Signer] or [cose.Verifier] implementation.
//
// An example use case where RS256 is allowed and used is in
// WebAuthn: https://www.w3.org/TR/webauthn-2/#sctn-sample-registration.
const (
	// RSASSA-PKCS1-v1_5 using SHA-256 by RFC 8812.
	AlgorithmRS256 Algorithm = -257

	// RSASSA-PKCS1-v1_5 using SHA-384 by RFC 8812.
	AlgorithmRS384 Algorithm = -258

	// RSASSA-PKCS1-v1_5 using SHA-512 by RFC 8812.
	AlgorithmRS512 Algorithm = -259
)

// Algorithm represents an IANA algorithm entry in the COSE Algorithms registry.
//
// # See Also
//
// COSE Algorithms: https://www.iana.org/assignments/cose/cose.xhtml#algorithms
//
// RFC 8152 16.4: https://datatracker.ietf.org/doc/html/rfc8152#section-16.4
type Algorithm int64

// String returns the name of the algorithm
func (a Algorithm) String() string {
	switch a {
	case AlgorithmPS256:
		return "PS256"
	case AlgorithmPS384:
		return "PS384"
	case AlgorithmPS512:
		return "PS512"
	case AlgorithmRS256:
		return "RS256"
	case AlgorithmRS384:
		return "RS384"
	case AlgorithmRS512:
		return "RS512"
	case AlgorithmES256:
		return "ES256"
	case AlgorithmES384:
		return "ES384"
	case AlgorithmES512:
		return "ES512"
	case AlgorithmEdDSA:
		// As stated in RFC 8152 8.2, only the pure EdDSA version is used for
		// COSE.
		return "EdDSA"
	case AlgorithmReserved:
		return "Reserved"
	default:
		return "Algorithm(" + strconv.Itoa(int(a)) + ")"
	}
}

// hashFunc returns the hash associated with the algorithm supported by this
// library.
func (a Algorithm) hashFunc() crypto.Hash {
	switch a {
	case AlgorithmPS256, AlgorithmES256:
		return crypto.SHA256
	case AlgorithmPS384, AlgorithmES384:
		return crypto.SHA384
	case AlgorithmPS512, AlgorithmES512:
		return crypto.SHA512
	default:
		return 0
	}
}

// computeHash computes the digest using the hash specified in the algorithm.
func (a Algorithm) computeHash(data []byte) ([]byte, error) {
	return computeHash(a.hashFunc(), data)
}

// computeHash computes the digest using the given hash.
func computeHash(h crypto.Hash, data []byte) ([]byte, error) {
	if !h.Available() {
		return nil, ErrUnavailableHashFunc
	}
	hh := h.New()
	if _, err := hh.Write(data); err != nil {
		return nil, err
	}
	return hh.Sum(nil), nil
}
