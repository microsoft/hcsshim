package cose

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
)

// Signer is an interface for private keys to sign COSE signatures.
type Signer interface {
	// Algorithm returns the signing algorithm associated with the private key.
	Algorithm() Algorithm

	// Sign signs message content with the private key, possibly using entropy
	// from rand.
	// The resulting signature should follow RFC 8152 section 8.
	//
	// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-8
	Sign(rand io.Reader, content []byte) ([]byte, error)
}

// NewSigner returns a signer with a given signing key.
// The signing key can be a golang built-in crypto private key, a key in HSM, or
// a remote KMS.
//
// Developers are encouraged to implement the `cose.Signer` interface instead of
// the `crypto.Signer` interface for better performance.
//
// All signing keys implementing `crypto.Signer` with `Public()` returning a
// public key of type `*rsa.PublicKey`, `*ecdsa.PublicKey`, or
// `ed25519.PublicKey` are accepted.
//
// Note: `*rsa.PrivateKey`, `*ecdsa.PrivateKey`, and `ed25519.PrivateKey`
// implement `crypto.Signer`.
func NewSigner(alg Algorithm, key crypto.Signer) (Signer, error) {
	switch alg {
	case AlgorithmPS256, AlgorithmPS384, AlgorithmPS512:
		vk, ok := key.Public().(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%v: %w", alg, ErrInvalidPubKey)
		}
		// RFC 8230 6.1 requires RSA keys having a minimum size of 2048 bits.
		// Reference: https://www.rfc-editor.org/rfc/rfc8230.html#section-6.1
		if vk.N.BitLen() < 2048 {
			return nil, errors.New("RSA key must be at least 2048 bits long")
		}
		return &rsaSigner{
			alg: alg,
			key: key,
		}, nil
	case AlgorithmES256, AlgorithmES384, AlgorithmES512:
		vk, ok := key.Public().(*ecdsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%v: %w", alg, ErrInvalidPubKey)
		}
		if sk, ok := key.(*ecdsa.PrivateKey); ok {
			return &ecdsaKeySigner{
				alg: alg,
				key: sk,
			}, nil
		}
		return &ecdsaCryptoSigner{
			alg:    alg,
			key:    vk,
			signer: key,
		}, nil
	case AlgorithmEd25519:
		if _, ok := key.Public().(ed25519.PublicKey); !ok {
			return nil, fmt.Errorf("%v: %w", alg, ErrInvalidPubKey)
		}
		return &ed25519Signer{
			key: key,
		}, nil
	default:
		return nil, ErrAlgorithmNotSupported
	}
}
