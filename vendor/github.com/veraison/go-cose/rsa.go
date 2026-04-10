package cose

import (
	"crypto"
	"crypto/rsa"
	"io"
)

// rsaSigner is a RSASSA-PSS based signer with a generic crypto.Signer.
//
// Reference: https://www.rfc-editor.org/rfc/rfc8230.html#section-2
type rsaSigner struct {
	alg Algorithm
	key crypto.Signer
}

// Algorithm returns the signing algorithm associated with the private key.
func (rs *rsaSigner) Algorithm() Algorithm {
	return rs.alg
}

// Sign signs message content with the private key, using entropy from rand.
// The resulting signature should follow RFC 8152 section 8.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-8
func (rs *rsaSigner) Sign(rand io.Reader, content []byte) ([]byte, error) {
	digest, err := rs.alg.computeHash(content)
	if err != nil {
		return nil, err
	}
	return rs.SignDigest(rand, digest)
}

// SignDigest signs message digest with the private key, possibly using
// entropy from rand.
// The resulting signature should follow RFC 8152 section 8.
func (rs *rsaSigner) SignDigest(rand io.Reader, digest []byte) ([]byte, error) {
	return rs.key.Sign(rand, digest, &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash, // defined in RFC 8230 sec 2
		Hash:       rs.alg.hashFunc(),
	})
}

// rsaVerifier is a RSASSA-PSS based verifier with golang built-in keys.
//
// Reference: https://www.rfc-editor.org/rfc/rfc8230.html#section-2
type rsaVerifier struct {
	alg Algorithm
	key *rsa.PublicKey
}

// Algorithm returns the signing algorithm associated with the public key.
func (rv *rsaVerifier) Algorithm() Algorithm {
	return rv.alg
}

// Verify verifies message content with the public key, returning nil for
// success.
// Otherwise, it returns ErrVerification.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-8
func (rv *rsaVerifier) Verify(content []byte, signature []byte) error {
	digest, err := rv.alg.computeHash(content)
	if err != nil {
		return err
	}
	return rv.VerifyDigest(digest, signature)
}

// VerifyDigest verifies message digest with the public key, returning nil
// for success.
// Otherwise, it returns ErrVerification.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-8.1
func (rv *rsaVerifier) VerifyDigest(digest []byte, signature []byte) error {
	if err := rsa.VerifyPSS(rv.key, rv.alg.hashFunc(), digest, signature, &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash, // defined in RFC 8230 sec 2
	}); err != nil {
		return ErrVerification
	}
	return nil
}
