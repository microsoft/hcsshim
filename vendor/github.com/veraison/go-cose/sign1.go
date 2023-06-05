package cose

import (
	"bytes"
	"errors"
	"io"

	"github.com/fxamacker/cbor/v2"
)

// sign1Message represents a COSE_Sign1 CBOR object:
//
//	COSE_Sign1 = [
//	    Headers,
//	    payload : bstr / nil,
//	    signature : bstr
//	]
//
// Reference: https://tools.ietf.org/html/rfc8152#section-4.2
type sign1Message struct {
	_           struct{} `cbor:",toarray"`
	Protected   cbor.RawMessage
	Unprotected cbor.RawMessage
	Payload     byteString
	Signature   byteString
}

// sign1MessagePrefix represents the fixed prefix of COSE_Sign1_Tagged.
var sign1MessagePrefix = []byte{
	0xd2, // #6.18
	0x84, // Array of length 4
}

// Sign1Message represents a decoded COSE_Sign1 message.
//
// Reference: https://tools.ietf.org/html/rfc8152#section-4.2
type Sign1Message struct {
	Headers   Headers
	Payload   []byte
	Signature []byte
}

// NewSign1Message returns a Sign1Message with header initialized.
func NewSign1Message() *Sign1Message {
	return &Sign1Message{
		Headers: Headers{
			Protected:   ProtectedHeader{},
			Unprotected: UnprotectedHeader{},
		},
	}
}

// MarshalCBOR encodes Sign1Message into a COSE_Sign1_Tagged object.
func (m *Sign1Message) MarshalCBOR() ([]byte, error) {
	content, err := m.getContent()
	if err != nil {
		return nil, err
	}

	return encMode.Marshal(cbor.Tag{
		Number:  CBORTagSign1Message,
		Content: content,
	})
}

// UnmarshalCBOR decodes a COSE_Sign1_Tagged object into Sign1Message.
func (m *Sign1Message) UnmarshalCBOR(data []byte) error {
	if m == nil {
		return errors.New("cbor: UnmarshalCBOR on nil Sign1Message pointer")
	}

	// fast message check
	if !bytes.HasPrefix(data, sign1MessagePrefix) {
		return errors.New("cbor: invalid COSE_Sign1_Tagged object")
	}

	return m.doUnmarshal(data[1:])
}

// Sign signs a Sign1Message using the provided Signer.
// The signature is stored in m.Signature.
//
// Note that m.Signature is only valid as long as m.Headers.Protected and
// m.Payload remain unchanged after calling this method.
// It is possible to modify m.Headers.Unprotected after signing,
// i.e., add counter signatures or timestamps.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
func (m *Sign1Message) Sign(rand io.Reader, external []byte, signer Signer) error {
	if m == nil {
		return errors.New("signing nil Sign1Message")
	}
	if m.Payload == nil {
		return ErrMissingPayload
	}
	if len(m.Signature) > 0 {
		return errors.New("Sign1Message signature already has signature bytes")
	}

	// check algorithm if present.
	// `alg` header MUST be present if there is no externally supplied data.
	alg := signer.Algorithm()
	err := m.Headers.ensureSigningAlgorithm(alg, external)
	if err != nil {
		return err
	}

	// sign the message
	toBeSigned, err := m.toBeSigned(external)
	if err != nil {
		return err
	}
	sig, err := signer.Sign(rand, toBeSigned)
	if err != nil {
		return err
	}

	m.Signature = sig
	return nil
}

// Verify verifies the signature on the Sign1Message returning nil on success or
// a suitable error if verification fails.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
func (m *Sign1Message) Verify(external []byte, verifier Verifier) error {
	if m == nil {
		return errors.New("verifying nil Sign1Message")
	}
	if m.Payload == nil {
		return ErrMissingPayload
	}
	if len(m.Signature) == 0 {
		return ErrEmptySignature
	}

	// check algorithm if present.
	// `alg` header MUST present if there is no externally supplied data.
	alg := verifier.Algorithm()
	err := m.Headers.ensureVerificationAlgorithm(alg, external)
	if err != nil {
		return err
	}

	// verify the message
	toBeSigned, err := m.toBeSigned(external)
	if err != nil {
		return err
	}
	return verifier.Verify(toBeSigned, m.Signature)
}

// toBeSigned constructs Sig_structure, computes and returns ToBeSigned.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
func (m *Sign1Message) toBeSigned(external []byte) ([]byte, error) {
	// create a Sig_structure and populate it with the appropriate fields.
	//
	//   Sig_structure = [
	//       context : "Signature1",
	//       body_protected : empty_or_serialized_map,
	//       external_aad : bstr,
	//       payload : bstr
	//   ]
	var protected cbor.RawMessage
	protected, err := m.Headers.MarshalProtected()
	if err != nil {
		return nil, err
	}
	protected, err = deterministicBinaryString(protected)
	if err != nil {
		return nil, err
	}
	if external == nil {
		external = []byte{}
	}
	sigStructure := []interface{}{
		"Signature1", // context
		protected,    // body_protected
		external,     // external_aad
		m.Payload,    // payload
	}

	// create the value ToBeSigned by encoding the Sig_structure to a byte
	// string.
	return encMode.Marshal(sigStructure)
}

func (m *Sign1Message) getContent() (sign1Message, error) {
	if m == nil {
		return sign1Message{}, errors.New("cbor: MarshalCBOR on nil Sign1Message pointer")
	}
	if len(m.Signature) == 0 {
		return sign1Message{}, ErrEmptySignature
	}
	protected, unprotected, err := m.Headers.marshal()
	if err != nil {
		return sign1Message{}, err
	}

	content := sign1Message{
		Protected:   protected,
		Unprotected: unprotected,
		Payload:     m.Payload,
		Signature:   m.Signature,
	}

	return content, nil
}

func (m *Sign1Message) doUnmarshal(data []byte) error {
	// decode to sign1Message and parse
	var raw sign1Message
	if err := decModeWithTagsForbidden.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw.Signature) == 0 {
		return ErrEmptySignature
	}
	msg := Sign1Message{
		Headers: Headers{
			RawProtected:   raw.Protected,
			RawUnprotected: raw.Unprotected,
		},
		Payload:   raw.Payload,
		Signature: raw.Signature,
	}
	if err := msg.Headers.UnmarshalFromRaw(); err != nil {
		return err
	}

	*m = msg
	return nil
}

// Sign1 signs a Sign1Message using the provided Signer.
//
// This method is a wrapper of `Sign1Message.Sign()`.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
func Sign1(rand io.Reader, signer Signer, headers Headers, payload []byte, external []byte) ([]byte, error) {
	msg := Sign1Message{
		Headers: headers,
		Payload: payload,
	}
	err := msg.Sign(rand, external, signer)
	if err != nil {
		return nil, err
	}
	return msg.MarshalCBOR()
}

type UntaggedSign1Message Sign1Message

// MarshalCBOR encodes UntaggedSign1Message into a COSE_Sign1 object.
func (m *UntaggedSign1Message) MarshalCBOR() ([]byte, error) {
	content, err := (*Sign1Message)(m).getContent()
	if err != nil {
		return nil, err
	}

	return encMode.Marshal(content)
}

// UnmarshalCBOR decodes a COSE_Sign1 object into an UnataggedSign1Message.
func (m *UntaggedSign1Message) UnmarshalCBOR(data []byte) error {
	if m == nil {
		return errors.New("cbor: UnmarshalCBOR on nil UntaggedSign1Message pointer")
	}

	if len(data) == 0 {
		return errors.New("cbor: zero length data")
	}

	// fast message check - ensure the frist byte indicates a four-element array
	if data[0] != sign1MessagePrefix[1] {
		return errors.New("cbor: invalid COSE_Sign1 object")
	}

	return (*Sign1Message)(m).doUnmarshal(data)
}

// Sign signs an UnttaggedSign1Message using the provided Signer.
// The signature is stored in m.Signature.
//
// Note that m.Signature is only valid as long as m.Headers.Protected and
// m.Payload remain unchanged after calling this method.
// It is possible to modify m.Headers.Unprotected after signing,
// i.e., add counter signatures or timestamps.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
func (m *UntaggedSign1Message) Sign(rand io.Reader, external []byte, signer Signer) error {
	return (*Sign1Message)(m).Sign(rand, external, signer)
}

// Verify verifies the signature on the UntaggedSign1Message returning nil on success or
// a suitable error if verification fails.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
func (m *UntaggedSign1Message) Verify(external []byte, verifier Verifier) error {
	return (*Sign1Message)(m).Verify(external, verifier)
}

// Sign1Untagged signs an UntaggedSign1Message using the provided Signer.
//
// This method is a wrapper of `UntaggedSign1Message.Sign()`.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
func Sign1Untagged(rand io.Reader, signer Signer, headers Headers, payload []byte, external []byte) ([]byte, error) {
	msg := UntaggedSign1Message{
		Headers: headers,
		Payload: payload,
	}
	err := msg.Sign(rand, external, signer)
	if err != nil {
		return nil, err
	}
	return msg.MarshalCBOR()
}
