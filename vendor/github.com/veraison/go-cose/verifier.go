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

// DigestVerifier is an interface for public keys to verify digested COSE signatures.
type DigestVerifier interface {
	// Algorithm returns the signing algorithm associated with the public key.
	Algorithm() Algorithm

	// VerifyDigest verifies message digest with the public key, returning nil
	// for success.
	// Otherwise, it returns ErrVerification.
	VerifyDigest(digest, signature []byte) error
}

// NewVerifier returns a verifier with a given public key.
// Only golang built-in crypto public keys of type `*rsa.PublicKey`,
// `*ecdsa.PublicKey`, and `ed25519.PublicKey` are accepted.
// When `*ecdsa.PublicKey` is specified, its curve must be supported by
// crypto/ecdh.
//
// The returned signer for rsa and ecdsa keys also implements
// `cose.DigestSigner`.
func NewVerifier(alg Algorithm, key crypto.PublicKey) (Verifier, error) {
	var errReason string
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
		if _, err := vk.ECDH(); err != nil {
			if err.Error() == "ecdsa: invalid public key" {
				return nil, fmt.Errorf("%v: %w", alg, ErrInvalidPubKey)
			}
			return nil, fmt.Errorf("%v: %w: %v", alg, ErrInvalidPubKey, err)
		}
		return &ecdsaVerifier{
			alg: alg,
			key: vk,
		}, nil
	case AlgorithmEdDSA:
		vk, ok := key.(ed25519.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%v: %w", alg, ErrInvalidPubKey)
		}
		return &ed25519Verifier{
			key: vk,
		}, nil
	case AlgorithmReserved:
		errReason = "can't be implemented"
	case AlgorithmRS256, AlgorithmRS384, AlgorithmRS512:
		errReason = "no built-in implementation available"
	default:
		errReason = "unknown algorithm"
	}
	return nil, fmt.Errorf("can't create new Verifier for %s: %s: %w", alg, errReason, ErrAlgorithmNotSupported)
}
