package cose

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/fxamacker/cbor/v2"
)

// signature represents a COSE_Signature CBOR object:
//
//	COSE_Signature =  [
//	    Headers,
//	    signature : bstr
//	]
//
// Reference: https://tools.ietf.org/html/rfc8152#section-4.1
type signature struct {
	_           struct{} `cbor:",toarray"`
	Protected   cbor.RawMessage
	Unprotected cbor.RawMessage
	Signature   byteString
}

// signaturePrefix represents the fixed prefix of COSE_Signature.
var signaturePrefix = []byte{
	0x83, // Array of length 3
}

// Signature represents a decoded COSE_Signature.
//
// Reference: https://tools.ietf.org/html/rfc8152#section-4.1
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
type Signature struct {
	Headers   Headers
	Signature []byte
}

// NewSignature returns a Signature with header initialized.
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
func NewSignature() *Signature {
	return &Signature{
		Headers: Headers{
			Protected:   ProtectedHeader{},
			Unprotected: UnprotectedHeader{},
		},
	}
}

// MarshalCBOR encodes Signature into a COSE_Signature object.
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
func (s *Signature) MarshalCBOR() ([]byte, error) {
	if s == nil {
		return nil, errors.New("cbor: MarshalCBOR on nil Signature pointer")
	}
	if len(s.Signature) == 0 {
		return nil, ErrEmptySignature
	}
	protected, unprotected, err := s.Headers.marshal()
	if err != nil {
		return nil, err
	}
	sig := signature{
		Protected:   protected,
		Unprotected: unprotected,
		Signature:   s.Signature,
	}
	return encMode.Marshal(sig)
}

// UnmarshalCBOR decodes a COSE_Signature object into Signature.
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
func (s *Signature) UnmarshalCBOR(data []byte) error {
	if s == nil {
		return errors.New("cbor: UnmarshalCBOR on nil Signature pointer")
	}

	// fast signature check
	if !bytes.HasPrefix(data, signaturePrefix) {
		return errors.New("cbor: invalid Signature object")
	}

	// decode to signature and parse
	var raw signature
	if err := decModeWithTagsForbidden.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw.Signature) == 0 {
		return ErrEmptySignature
	}
	sig := Signature{
		Headers: Headers{
			RawProtected:   raw.Protected,
			RawUnprotected: raw.Unprotected,
		},
		Signature: raw.Signature,
	}
	if err := sig.Headers.UnmarshalFromRaw(); err != nil {
		return err
	}

	*s = sig
	return nil
}

// Sign signs a Signature using the provided Signer.
// Signing a COSE_Signature requires the encoded protected header and the
// payload of its parent message.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
func (s *Signature) Sign(rand io.Reader, signer Signer, protected cbor.RawMessage, payload, external []byte) error {
	if s == nil {
		return errors.New("signing nil Signature")
	}
	if payload == nil {
		return ErrMissingPayload
	}
	if len(s.Signature) > 0 {
		return errors.New("Signature already has signature bytes")
	}
	if len(protected) == 0 || protected[0]>>5 != 2 { // protected is a bstr
		return errors.New("invalid body protected headers")
	}

	// check algorithm if present.
	// `alg` header MUST present if there is no externally supplied data.
	alg := signer.Algorithm()
	if err := s.Headers.ensureSigningAlgorithm(alg, external); err != nil {
		return err
	}

	// sign the message
	toBeSigned, err := s.toBeSigned(protected, payload, external)
	if err != nil {
		return err
	}
	sig, err := signer.Sign(rand, toBeSigned)
	if err != nil {
		return err
	}

	s.Signature = sig
	return nil
}

// Verify verifies the signature, returning nil on success or a suitable error
// if verification fails.
// Verifying a COSE_Signature requires the encoded protected header and the
// payload of its parent message.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
func (s *Signature) Verify(verifier Verifier, protected cbor.RawMessage, payload, external []byte) error {
	if s == nil {
		return errors.New("verifying nil Signature")
	}
	if payload == nil {
		return ErrMissingPayload
	}
	if len(s.Signature) == 0 {
		return ErrEmptySignature
	}
	if len(protected) == 0 || protected[0]>>5 != 2 { // protected is a bstr
		return errors.New("invalid body protected headers")
	}

	// check algorithm if present.
	// `alg` header MUST present if there is no externally supplied data.
	alg := verifier.Algorithm()
	err := s.Headers.ensureVerificationAlgorithm(alg, external)
	if err != nil {
		return err
	}

	// verify the message
	toBeSigned, err := s.toBeSigned(protected, payload, external)
	if err != nil {
		return err
	}
	return verifier.Verify(toBeSigned, s.Signature)
}

// toBeSigned constructs Sig_structure, computes and returns ToBeSigned.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
func (s *Signature) toBeSigned(bodyProtected cbor.RawMessage, payload, external []byte) ([]byte, error) {
	// create a Sig_structure and populate it with the appropriate fields.
	//
	//   Sig_structure = [
	//       context : "Signature",
	//       body_protected : empty_or_serialized_map,
	//       sign_protected : empty_or_serialized_map,
	//       external_aad : bstr,
	//       payload : bstr
	//   ]
	bodyProtected, err := deterministicBinaryString(bodyProtected)
	if err != nil {
		return nil, err
	}
	var signProtected cbor.RawMessage
	signProtected, err = s.Headers.MarshalProtected()
	if err != nil {
		return nil, err
	}
	signProtected, err = deterministicBinaryString(signProtected)
	if err != nil {
		return nil, err
	}
	if external == nil {
		external = []byte{}
	}
	sigStructure := []interface{}{
		"Signature",   // context
		bodyProtected, // body_protected
		signProtected, // sign_protected
		external,      // external_aad
		payload,       // payload
	}

	// create the value ToBeSigned by encoding the Sig_structure to a byte
	// string.
	return encMode.Marshal(sigStructure)
}

// signMessage represents a COSE_Sign CBOR object:
//
//	COSE_Sign = [
//	    Headers,
//	    payload : bstr / nil,
//	    signatures : [+ COSE_Signature]
//	]
//
// Reference: https://tools.ietf.org/html/rfc8152#section-4.1
type signMessage struct {
	_           struct{} `cbor:",toarray"`
	Protected   cbor.RawMessage
	Unprotected cbor.RawMessage
	Payload     byteString
	Signatures  []cbor.RawMessage
}

// signMessagePrefix represents the fixed prefix of COSE_Sign_Tagged.
var signMessagePrefix = []byte{
	0xd8, 0x62, // #6.98
	0x84, // Array of length 4
}

// SignMessage represents a decoded COSE_Sign message.
//
// Reference: https://tools.ietf.org/html/rfc8152#section-4.1
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
type SignMessage struct {
	Headers    Headers
	Payload    []byte
	Signatures []*Signature
}

// NewSignMessage returns a SignMessage with header initialized.
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
func NewSignMessage() *SignMessage {
	return &SignMessage{
		Headers: Headers{
			Protected:   ProtectedHeader{},
			Unprotected: UnprotectedHeader{},
		},
	}
}

// MarshalCBOR encodes SignMessage into a COSE_Sign_Tagged object.
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
func (m *SignMessage) MarshalCBOR() ([]byte, error) {
	if m == nil {
		return nil, errors.New("cbor: MarshalCBOR on nil SignMessage pointer")
	}
	if len(m.Signatures) == 0 {
		return nil, ErrNoSignatures
	}
	protected, unprotected, err := m.Headers.marshal()
	if err != nil {
		return nil, err
	}
	signatures := make([]cbor.RawMessage, 0, len(m.Signatures))
	for _, sig := range m.Signatures {
		sigCBOR, err := sig.MarshalCBOR()
		if err != nil {
			return nil, err
		}
		signatures = append(signatures, sigCBOR)
	}
	content := signMessage{
		Protected:   protected,
		Unprotected: unprotected,
		Payload:     m.Payload,
		Signatures:  signatures,
	}
	return encMode.Marshal(cbor.Tag{
		Number:  CBORTagSignMessage,
		Content: content,
	})
}

// UnmarshalCBOR decodes a COSE_Sign_Tagged object into SignMessage.
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
func (m *SignMessage) UnmarshalCBOR(data []byte) error {
	if m == nil {
		return errors.New("cbor: UnmarshalCBOR on nil SignMessage pointer")
	}

	// fast message check
	if !bytes.HasPrefix(data, signMessagePrefix) {
		return errors.New("cbor: invalid COSE_Sign_Tagged object")
	}

	// decode to signMessage and parse
	var raw signMessage
	if err := decModeWithTagsForbidden.Unmarshal(data[2:], &raw); err != nil {
		return err
	}
	if len(raw.Signatures) == 0 {
		return ErrNoSignatures
	}
	signatures := make([]*Signature, 0, len(raw.Signatures))
	for _, sigCBOR := range raw.Signatures {
		sig := &Signature{}
		if err := sig.UnmarshalCBOR(sigCBOR); err != nil {
			return err
		}
		signatures = append(signatures, sig)
	}
	msg := SignMessage{
		Headers: Headers{
			RawProtected:   raw.Protected,
			RawUnprotected: raw.Unprotected,
		},
		Payload:    raw.Payload,
		Signatures: signatures,
	}
	if err := msg.Headers.UnmarshalFromRaw(); err != nil {
		return err
	}

	*m = msg
	return nil
}

// Sign signs a SignMessage using the provided signers corresponding to the
// signatures.
//
// See `Signature.Sign()` for advanced signing scenarios.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
func (m *SignMessage) Sign(rand io.Reader, external []byte, signers ...Signer) error {
	if m == nil {
		return errors.New("signing nil SignMessage")
	}
	if m.Payload == nil {
		return ErrMissingPayload
	}
	switch len(m.Signatures) {
	case 0:
		return ErrNoSignatures
	case len(signers):
		// no ops
	default:
		return fmt.Errorf("%d signers for %d signatures", len(signers), len(m.Signatures))
	}

	// populate common parameters
	var protected cbor.RawMessage
	protected, err := m.Headers.MarshalProtected()
	if err != nil {
		return err
	}

	// sign message accordingly
	for i, signature := range m.Signatures {
		if err := signature.Sign(rand, signers[i], protected, m.Payload, external); err != nil {
			return err
		}
	}

	return nil
}

// Verify verifies the signatures on the SignMessage against the corresponding
// verifier, returning nil on success or a suitable error if verification fails.
//
// See `Signature.Verify()` for advanced verification scenarios like threshold
// policies.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
func (m *SignMessage) Verify(external []byte, verifiers ...Verifier) error {
	if m == nil {
		return errors.New("verifying nil SignMessage")
	}
	if m.Payload == nil {
		return ErrMissingPayload
	}
	switch len(m.Signatures) {
	case 0:
		return ErrNoSignatures
	case len(verifiers):
		// no ops
	default:
		return fmt.Errorf("%d verifiers for %d signatures", len(verifiers), len(m.Signatures))
	}

	// populate common parameters
	var protected cbor.RawMessage
	protected, err := m.Headers.MarshalProtected()
	if err != nil {
		return err
	}

	// verify message accordingly
	for i, signature := range m.Signatures {
		if err := signature.Verify(verifiers[i], protected, m.Payload, external); err != nil {
			return err
		}
	}
	return nil
}
