package cose

import (
	"crypto"
	"crypto/ed25519"
	"io"
)

// ed25519Signer is a Pure EdDSA based signer with a generic crypto.Signer.
type ed25519Signer struct {
	key crypto.Signer
}

// Algorithm returns the signing algorithm associated with the private key.
func (es *ed25519Signer) Algorithm() Algorithm {
	return AlgorithmEd25519
}

// Sign signs message content with the private key, possibly using entropy from
// rand.
// The resulting signature should follow RFC 8152 section 8.2.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-8.2
func (es *ed25519Signer) Sign(rand io.Reader, content []byte) ([]byte, error) {
	// crypto.Hash(0) must be passed as an option.
	// Reference: https://pkg.go.dev/crypto/ed25519#PrivateKey.Sign
	return es.key.Sign(rand, content, crypto.Hash(0))
}

// ed25519Verifier is a Pure EdDSA based verifier with golang built-in keys.
type ed25519Verifier struct {
	key ed25519.PublicKey
}

// Algorithm returns the signing algorithm associated with the public key.
func (ev *ed25519Verifier) Algorithm() Algorithm {
	return AlgorithmEd25519
}

// Verify verifies message content with the public key, returning nil for
// success.
// Otherwise, it returns ErrVerification.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-8.2
func (ev *ed25519Verifier) Verify(content []byte, signature []byte) error {
	if verified := ed25519.Verify(ev.key, content, signature); !verified {
		return ErrVerification
	}
	return nil
}
