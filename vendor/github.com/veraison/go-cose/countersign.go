package cose

import (
	"errors"
	"fmt"
	"io"

	"github.com/fxamacker/cbor/v2"
)

// Countersignature represents a decoded COSE_Countersignature.
//
// Reference: https://tools.ietf.org/html/rfc9338#section-3.1
//
// # Experimental
//
// Notice: The COSE Countersignature API is EXPERIMENTAL and may be changed or
// removed in a later release.
type Countersignature Signature

// NewCountersignature returns a Countersignature with header initialized.
//
// # Experimental
//
// Notice: The COSE Countersignature API is EXPERIMENTAL and may be changed or
// removed in a later release.
func NewCountersignature() *Countersignature {
	return (*Countersignature)(NewSignature())
}

// MarshalCBOR encodes Countersignature into a COSE_Countersignature object.
//
// # Experimental
//
// Notice: The COSE Countersignature API is EXPERIMENTAL and may be changed or
// removed in a later release.
func (s *Countersignature) MarshalCBOR() ([]byte, error) {
	if s == nil {
		return nil, errors.New("cbor: MarshalCBOR on nil Countersignature pointer")
	}
	// COSE_Countersignature share the exact same format as COSE_Signature
	return (*Signature)(s).MarshalCBOR()
}

// UnmarshalCBOR decodes a COSE_Countersignature object into Countersignature.
//
// # Experimental
//
// Notice: The COSE Countersignature API is EXPERIMENTAL and may be changed or
// removed in a later release.
func (s *Countersignature) UnmarshalCBOR(data []byte) error {
	if s == nil {
		return errors.New("cbor: UnmarshalCBOR on nil Countersignature pointer")
	}
	// COSE_Countersignature share the exact same format as COSE_Signature
	return (*Signature)(s).UnmarshalCBOR(data)
}

// Sign signs a Countersignature using the provided Signer.
// Signing a COSE_Countersignature requires the parent message to be completely
// fulfilled.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc9338#section-3.3
//
// # Experimental
//
// Notice: The COSE Countersignature API is EXPERIMENTAL and may be changed or
// removed in a later release.
func (s *Countersignature) Sign(rand io.Reader, signer Signer, parent any, external []byte) error {
	if s == nil {
		return errors.New("signing nil Countersignature")
	}
	if len(s.Signature) > 0 {
		return errors.New("Countersignature already has signature bytes")
	}

	// check algorithm if present.
	// `alg` header MUST present if there is no externally supplied data.
	alg := signer.Algorithm()
	if err := s.Headers.ensureSigningAlgorithm(alg, external); err != nil {
		return err
	}

	// sign the message
	toBeSigned, err := s.toBeSigned(parent, external)
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

// Verify verifies the countersignature, returning nil on success or a suitable error
// if verification fails.
// Verifying a COSE_Countersignature requires the parent message.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
//
// # Experimental
//
// Notice: The COSE Sign API is EXPERIMENTAL and may be changed or removed in a
// later release.
func (s *Countersignature) Verify(verifier Verifier, parent any, external []byte) error {
	if s == nil {
		return errors.New("verifying nil Countersignature")
	}
	if len(s.Signature) == 0 {
		return ErrEmptySignature
	}

	// check algorithm if present.
	// `alg` header MUST present if there is no externally supplied data.
	alg := verifier.Algorithm()
	err := s.Headers.ensureVerificationAlgorithm(alg, external)
	if err != nil {
		return err
	}

	// verify the message
	toBeSigned, err := s.toBeSigned(parent, external)
	if err != nil {
		return err
	}
	return verifier.Verify(toBeSigned, s.Signature)
}

// toBeSigned returns ToBeSigned from COSE_Countersignature object.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc9338#section-3.3
func (s *Countersignature) toBeSigned(target any, external []byte) ([]byte, error) {
	var signProtected cbor.RawMessage
	signProtected, err := s.Headers.MarshalProtected()
	if err != nil {
		return nil, err
	}
	return countersignToBeSigned(false, target, signProtected, external)
}

// countersignToBeSigned constructs Countersign_structure, computes and returns ToBeSigned.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc9338#section-3.3
func countersignToBeSigned(abbreviated bool, target any, signProtected cbor.RawMessage, external []byte) ([]byte, error) {
	// create a Countersign_structure and populate it with the appropriate fields.
	//
	//   Countersign_structure = [
	//       context : "CounterSignature" / "CounterSignature0" /
	//                 "CounterSignatureV2" / "CounterSignature0V2" /,
	//       body_protected : empty_or_serialized_map,
	//       ? sign_protected : empty_or_serialized_map,
	//       external_aad : bstr,
	//       payload : bstr,
	//       ? other_fields : [+ bstr ]
	//   ]

	var err error
	var bodyProtected cbor.RawMessage
	var otherFields []cbor.RawMessage
	var payload []byte

	switch t := target.(type) {
	case *SignMessage:
		return countersignToBeSigned(abbreviated, *t, signProtected, external)
	case SignMessage:
		if len(t.Signatures) == 0 {
			return nil, errors.New("SignMessage has no signatures yet")
		}
		bodyProtected, err = t.Headers.MarshalProtected()
		if err != nil {
			return nil, err
		}
		if t.Payload == nil {
			return nil, ErrMissingPayload
		}
		payload = t.Payload
	case *Sign1Message:
		return countersignToBeSigned(abbreviated, *t, signProtected, external)
	case Sign1Message:
		if len(t.Signature) == 0 {
			return nil, errors.New("Sign1Message was not signed yet")
		}
		bodyProtected, err = t.Headers.MarshalProtected()
		if err != nil {
			return nil, err
		}
		if t.Payload == nil {
			return nil, ErrMissingPayload
		}
		payload = t.Payload
		signature, err := encMode.Marshal(t.Signature)
		if err != nil {
			return nil, err
		}
		signature, err = deterministicBinaryString(signature)
		if err != nil {
			return nil, err
		}
		otherFields = []cbor.RawMessage{signature}
	case *Signature:
		return countersignToBeSigned(abbreviated, *t, signProtected, external)
	case Signature:
		bodyProtected, err = t.Headers.MarshalProtected()
		if err != nil {
			return nil, err
		}
		if len(t.Signature) == 0 {
			return nil, errors.New("Signature was not signed yet")
		}
		payload = t.Signature
	case *Countersignature:
		return countersignToBeSigned(abbreviated, *t, signProtected, external)
	case Countersignature:
		bodyProtected, err = t.Headers.MarshalProtected()
		if err != nil {
			return nil, err
		}
		if len(t.Signature) == 0 {
			return nil, errors.New("Countersignature was not signed yet")
		}
		payload = t.Signature
	default:
		return nil, fmt.Errorf("unsupported target %T", target)
	}

	var context string
	if len(otherFields) == 0 {
		if abbreviated {
			context = "CounterSignature0"
		} else {
			context = "CounterSignature"
		}
	} else {
		if abbreviated {
			context = "CounterSignature0V2"
		} else {
			context = "CounterSignatureV2"
		}
	}

	bodyProtected, err = deterministicBinaryString(bodyProtected)
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
	countersigStructure := []any{
		context,       // context
		bodyProtected, // body_protected
		signProtected, // sign_protected
		external,      // external_aad
		payload,       // payload
	}
	if len(otherFields) > 0 {
		countersigStructure = append(countersigStructure, otherFields)
	}

	// create the value ToBeSigned by encoding the Countersign_structure to a byte
	// string.
	return encMode.Marshal(countersigStructure)
}

// Countersign0 performs an abbreviated signature over a parent message using
// the provided Signer.
//
// The parent message must be completely fulfilled prior signing.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc9338#section-3.2
//
// # Experimental
//
// Notice: The COSE Countersignature API is EXPERIMENTAL and may be changed or
// removed in a later release.
func Countersign0(rand io.Reader, signer Signer, parent any, external []byte) ([]byte, error) {
	toBeSigned, err := countersignToBeSigned(true, parent, []byte{0x40}, external)
	if err != nil {
		return nil, err
	}
	return signer.Sign(rand, toBeSigned)
}

// VerifyCountersign0 verifies an abbreviated signature over a parent message
// using the provided Verifier.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc9338#section-3.2
//
// # Experimental
//
// Notice: The COSE Countersignature API is EXPERIMENTAL and may be changed or
// removed in a later release.
func VerifyCountersign0(verifier Verifier, parent any, external, signature []byte) error {
	toBeSigned, err := countersignToBeSigned(true, parent, []byte{0x40}, external)
	if err != nil {
		return err
	}
	return verifier.Verify(toBeSigned, signature)
}
