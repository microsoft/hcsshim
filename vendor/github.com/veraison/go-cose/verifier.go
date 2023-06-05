package cose

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"
	"fmt"
)

// Verifier is an interface for public keys to verify COSE signatures.
type Verifier interface {
	// Algorithm returns the signing algorithm associated with the public key.
	Algorithm() Algorithm

	// Verify verifies message content with the public key, returning nil for
	// success.
	// Otherwise, it returns ErrVerification.
	//
	// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-8
	Verify(content, signature []byte) error
}

// NewVerifier returns a verifier with a given public key.
// Only golang built-in crypto public keys of type `*rsa.PublicKey`,
// `*ecdsa.PublicKey`, and `ed25519.PublicKey` are accepted.
func NewVerifier(alg Algorithm, key crypto.PublicKey) (Verifier, error) {
	switch alg {
	case AlgorithmPS256, AlgorithmPS384, AlgorithmPS512:
		vk, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%v: %w", alg, ErrInvalidPubKey)
		}
		// RFC 8230 6.1 requires RSA keys having a minimun size of 2048 bits.
		// Reference: https://www.rfc-editor.org/rfc/rfc8230.html#section-6.1
		if vk.N.BitLen() < 2048 {
			return nil, errors.New("RSA key must be at least 2048 bits long")
		}
		return &rsaVerifier{
			alg: alg,
			key: vk,
		}, nil
	case AlgorithmES256, AlgorithmES384, AlgorithmES512:
		vk, ok := key.(*ecdsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%v: %w", alg, ErrInvalidPubKey)
		}
		if !vk.Curve.IsOnCurve(vk.X, vk.Y) {
			return nil, errors.New("public key point is not on curve")
		}
		return &ecdsaVerifier{
			alg: alg,
			key: vk,
		}, nil
	case AlgorithmEd25519:
		vk, ok := key.(ed25519.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%v: %w", alg, ErrInvalidPubKey)
		}
		return &ed25519Verifier{
			key: vk,
		}, nil
	default:
		return nil, ErrAlgorithmNotSupported
	}
}
