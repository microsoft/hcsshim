package cose

import (
	"crypto"
	"fmt"
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
	AlgorithmEd25519 Algorithm = -8

	// An invalid/unrecognised algorithm.
	AlgorithmInvalid Algorithm = 0
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
	case AlgorithmES256:
		return "ES256"
	case AlgorithmES384:
		return "ES384"
	case AlgorithmES512:
		return "ES512"
	case AlgorithmEd25519:
		// As stated in RFC 8152 8.2, only the pure EdDSA version is used for
		// COSE.
		return "EdDSA"
	default:
		return "unknown algorithm value " + strconv.Itoa(int(a))
	}
}

// MarshalCBOR marshals the Algorithm as a CBOR int.
func (a Algorithm) MarshalCBOR() ([]byte, error) {
	return encMode.Marshal(int64(a))
}

// UnmarshalCBOR populates the Algorithm from the provided CBOR value (must be
// int or tstr).
func (a *Algorithm) UnmarshalCBOR(data []byte) error {
	var raw intOrStr

	if err := raw.UnmarshalCBOR(data); err != nil {
		return fmt.Errorf("invalid algorithm value: %w", err)
	}

	if raw.IsString() {
		v := algorithmFromString(raw.String())
		if v == AlgorithmInvalid {
			return fmt.Errorf("unknown algorithm value %q", raw.String())
		}

		*a = v
	} else {
		v := raw.Int()
		*a = Algorithm(v)
	}

	return nil
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

// NOTE: there are currently no registered string values for an algorithm.
func algorithmFromString(v string) Algorithm {
	return AlgorithmInvalid
}
