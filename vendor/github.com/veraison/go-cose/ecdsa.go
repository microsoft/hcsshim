package cose

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/asn1"
	"errors"
	"fmt"
	"io"
	"math/big"
)

// I2OSP - Integer-to-Octet-String primitive converts a nonnegative integer to
// an octet string of a specified length `len(buf)`, and stores it in `buf`.
// I2OSP is used for encoding ECDSA signature (r, s) into byte strings.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8017#section-4.1
func I2OSP(x *big.Int, buf []byte) error {
	if x.Sign() < 0 {
		return errors.New("I2OSP: negative integer")
	}
	if x.BitLen() > len(buf)*8 {
		return errors.New("I2OSP: integer too large")
	}
	x.FillBytes(buf)
	return nil
}

// OS2IP - Octet-String-to-Integer primitive converts an octet string to a
// nonnegative integer.
// OS2IP is used for decoding ECDSA signature (r, s) from byte strings.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8017#section-4.2
func OS2IP(x []byte) *big.Int {
	return new(big.Int).SetBytes(x)
}

// ecdsaKeySigner is a ECDSA-based signer with golang built-in keys.
type ecdsaKeySigner struct {
	alg Algorithm
	key *ecdsa.PrivateKey
}

// Algorithm returns the signing algorithm associated with the private key.
func (es *ecdsaKeySigner) Algorithm() Algorithm {
	return es.alg
}

// Sign signs message content with the private key using entropy from rand.
// The resulting signature should follow RFC 8152 section 8.1,
// although it does not follow the recommendation of being deterministic.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-8.1
func (es *ecdsaKeySigner) Sign(rand io.Reader, content []byte) ([]byte, error) {
	digest, err := es.alg.computeHash(content)
	if err != nil {
		return nil, err
	}
	r, s, err := ecdsa.Sign(rand, es.key, digest)
	if err != nil {
		return nil, err
	}
	return encodeECDSASignature(es.key.Curve, r, s)
}

// ecdsaKeySigner is a ECDSA based signer with a generic crypto.Signer.
type ecdsaCryptoSigner struct {
	alg    Algorithm
	key    *ecdsa.PublicKey
	signer crypto.Signer
}

// Algorithm returns the signing algorithm associated with the private key.
func (es *ecdsaCryptoSigner) Algorithm() Algorithm {
	return es.alg
}

// Sign signs message content with the private key, possibly using entropy from
// rand.
// The resulting signature should follow RFC 8152 section 8.1.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-8.1
func (es *ecdsaCryptoSigner) Sign(rand io.Reader, content []byte) ([]byte, error) {
	digest, err := es.alg.computeHash(content)
	if err != nil {
		return nil, err
	}
	sigASN1, err := es.signer.Sign(rand, digest, nil)
	if err != nil {
		return nil, err
	}

	// decode ASN.1 decoded signature
	var sig struct {
		R, S *big.Int
	}
	if _, err := asn1.Unmarshal(sigASN1, &sig); err != nil {
		return nil, err
	}

	// encode signature in the COSE form
	return encodeECDSASignature(es.key.Curve, sig.R, sig.S)
}

// encodeECDSASignature encodes (r, s) into a signature binary string using the
// method specified by RFC 8152 section 8.1.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-8.1
func encodeECDSASignature(curve elliptic.Curve, r, s *big.Int) ([]byte, error) {
	n := (curve.Params().N.BitLen() + 7) / 8
	sig := make([]byte, n*2)
	if err := I2OSP(r, sig[:n]); err != nil {
		return nil, err
	}
	if err := I2OSP(s, sig[n:]); err != nil {
		return nil, err
	}
	return sig, nil
}

// decodeECDSASignature decodes (r, s) from a signature binary string using the
// method specified by RFC 8152 section 8.1.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-8.1
func decodeECDSASignature(curve elliptic.Curve, sig []byte) (r, s *big.Int, err error) {
	n := (curve.Params().N.BitLen() + 7) / 8
	if len(sig) != n*2 {
		return nil, nil, fmt.Errorf("invalid signature length: %d", len(sig))
	}
	return OS2IP(sig[:n]), OS2IP(sig[n:]), nil
}

// ecdsaVerifier is a ECDSA based verifier with golang built-in keys.
type ecdsaVerifier struct {
	alg Algorithm
	key *ecdsa.PublicKey
}

// Algorithm returns the signing algorithm associated with the public key.
func (ev *ecdsaVerifier) Algorithm() Algorithm {
	return ev.alg
}

// Verify verifies message content with the public key, returning nil for
// success.
// Otherwise, it returns ErrVerification.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-8.1
func (ev *ecdsaVerifier) Verify(content []byte, signature []byte) error {
	// compute digest
	digest, err := ev.alg.computeHash(content)
	if err != nil {
		return err
	}

	// verify signature
	r, s, err := decodeECDSASignature(ev.key.Curve, signature)
	if err != nil {
		return ErrVerification
	}
	if verified := ecdsa.Verify(ev.key, digest, r, s); !verified {
		return ErrVerification
	}
	return nil
}
