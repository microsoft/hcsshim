package cose

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/fxamacker/cbor/v2"
)

// COSE Header labels registered in the IANA "COSE Header Parameters" registry.
//
// Reference: https://www.iana.org/assignments/cose/cose.xhtml#header-parameters
const (
	HeaderLabelAlgorithm         int64 = 1
	HeaderLabelCritical          int64 = 2
	HeaderLabelContentType       int64 = 3
	HeaderLabelKeyID             int64 = 4
	HeaderLabelIV                int64 = 5
	HeaderLabelPartialIV         int64 = 6
	HeaderLabelCounterSignature  int64 = 7
	HeaderLabelCounterSignature0 int64 = 9
	HeaderLabelX5Bag             int64 = 32
	HeaderLabelX5Chain           int64 = 33
	HeaderLabelX5T               int64 = 34
	HeaderLabelX5U               int64 = 35
)

// ProtectedHeader contains parameters that are to be cryptographically
// protected.
type ProtectedHeader map[interface{}]interface{}

// MarshalCBOR encodes the protected header into a CBOR bstr object.
// A zero-length header is encoded as a zero-length string rather than as a
// zero-length map (encoded as h'a0').
func (h ProtectedHeader) MarshalCBOR() ([]byte, error) {
	var encoded []byte
	if len(h) == 0 {
		encoded = []byte{}
	} else {
		err := validateHeaderParameters(h, true)
		if err != nil {
			return nil, fmt.Errorf("protected header: %w", err)
		}
		encoded, err = encMode.Marshal(map[interface{}]interface{}(h))
		if err != nil {
			return nil, err
		}
	}
	return encMode.Marshal(encoded)
}

// UnmarshalCBOR decodes a CBOR bstr object into ProtectedHeader.
//
// ProtectedHeader is an empty_or_serialized_map where
//
//	empty_or_serialized_map = bstr .cbor header_map / bstr .size 0
func (h *ProtectedHeader) UnmarshalCBOR(data []byte) error {
	if h == nil {
		return errors.New("cbor: UnmarshalCBOR on nil ProtectedHeader pointer")
	}
	var encoded byteString
	if err := encoded.UnmarshalCBOR(data); err != nil {
		return err
	}
	if encoded == nil {
		return errors.New("cbor: nil protected header")
	}
	if len(encoded) == 0 {
		*h = make(ProtectedHeader)
	} else {
		if encoded[0]>>5 != 5 { // major type 5: map
			return errors.New("cbor: protected header: require map type")
		}
		if err := validateHeaderLabelCBOR(encoded); err != nil {
			return err
		}
		var header map[interface{}]interface{}
		if err := decMode.Unmarshal(encoded, &header); err != nil {
			return err
		}
		candidate := ProtectedHeader(header)
		if err := validateHeaderParameters(candidate, true); err != nil {
			return fmt.Errorf("protected header: %w", err)
		}

		// cast to type Algorithm if `alg` presents
		if alg, err := candidate.Algorithm(); err == nil {
			candidate.SetAlgorithm(alg)
		}

		*h = candidate
	}
	return nil
}

// SetAlgorithm sets the algorithm value to the algorithm header.
func (h ProtectedHeader) SetAlgorithm(alg Algorithm) {
	h[HeaderLabelAlgorithm] = alg
}

// Algorithm gets the algorithm value from the algorithm header.
func (h ProtectedHeader) Algorithm() (Algorithm, error) {
	value, ok := h[HeaderLabelAlgorithm]
	if !ok {
		return 0, ErrAlgorithmNotFound
	}
	switch alg := value.(type) {
	case Algorithm:
		return alg, nil
	case int:
		return Algorithm(alg), nil
	case int8:
		return Algorithm(alg), nil
	case int16:
		return Algorithm(alg), nil
	case int32:
		return Algorithm(alg), nil
	case int64:
		return Algorithm(alg), nil
	case string:
		v := algorithmFromString(alg)

		var err error
		if v == AlgorithmInvalid {
			err = fmt.Errorf("unknown algorithm value %q", alg)
		}

		return v, err
	default:
		return AlgorithmInvalid, ErrInvalidAlgorithm
	}
}

// Critical indicates which protected header labels an application that is
// processing a message is required to understand.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-3.1
func (h ProtectedHeader) Critical() ([]interface{}, error) {
	value, ok := h[HeaderLabelCritical]
	if !ok {
		return nil, nil
	}
	err := ensureCritical(value, h)
	if err != nil {
		return nil, err
	}
	return value.([]interface{}), nil
}

// ensureCritical ensures all critical headers are present in the protected bucket.
func ensureCritical(value interface{}, headers map[interface{}]interface{}) error {
	labels, ok := value.([]interface{})
	if !ok {
		return errors.New("invalid crit header")
	}
	// if present, the array MUST have at least one value in it.
	if len(labels) == 0 {
		return errors.New("empty crit header")
	}
	for _, label := range labels {
		if !canInt(label) && !canTstr(label) {
			return fmt.Errorf("require int / tstr type, got '%T': %v", label, label)
		}
		if _, ok := headers[label]; !ok {
			return fmt.Errorf("missing critical header: %v", label)
		}
	}
	return nil
}

// UnprotectedHeader contains parameters that are not cryptographically
// protected.
type UnprotectedHeader map[interface{}]interface{}

// MarshalCBOR encodes the unprotected header into a CBOR map object.
// A zero-length header is encoded as a zero-length map (encoded as h'a0').
func (h UnprotectedHeader) MarshalCBOR() ([]byte, error) {
	if len(h) == 0 {
		return []byte{0xa0}, nil
	}
	if err := validateHeaderParameters(h, false); err != nil {
		return nil, fmt.Errorf("unprotected header: %w", err)
	}
	return encMode.Marshal(map[interface{}]interface{}(h))
}

// UnmarshalCBOR decodes a CBOR map object into UnprotectedHeader.
//
// UnprotectedHeader is a header_map.
func (h *UnprotectedHeader) UnmarshalCBOR(data []byte) error {
	if h == nil {
		return errors.New("cbor: UnmarshalCBOR on nil UnprotectedHeader pointer")
	}
	if data == nil {
		return errors.New("cbor: nil unprotected header")
	}
	if len(data) == 0 {
		return errors.New("cbor: unprotected header: missing type")
	}
	if data[0]>>5 != 5 { // major type 5: map
		return errors.New("cbor: unprotected header: require map type")
	}
	if err := validateHeaderLabelCBOR(data); err != nil {
		return err
	}
	var header map[interface{}]interface{}
	if err := decMode.Unmarshal(data, &header); err != nil {
		return err
	}
	if err := validateHeaderParameters(header, false); err != nil {
		return fmt.Errorf("unprotected header: %w", err)
	}
	*h = header
	return nil
}

// Headers represents "two buckets of information that are not
// considered to be part of the payload itself, but are used for
// holding information about content, algorithms, keys, or evaluation
// hints for the processing of the layer."
//
// It is represented by CDDL fragments:
//
//	Headers = (
//	    protected : empty_or_serialized_map,
//	    unprotected : header_map
//	)
//
//	header_map = {
//	    Generic_Headers,
//	    * label => values
//	}
//
//	label  = int / tstr
//	values = any
//
//	empty_or_serialized_map = bstr .cbor header_map / bstr .size 0
//
// # See Also
//
// https://tools.ietf.org/html/rfc8152#section-3
type Headers struct {
	// RawProtected contains the raw CBOR encoded data for the protected header.
	// It is populated when decoding.
	// Applications can use this field for customized encoding / decoding of
	// the protected header in case the default decoder provided by this library
	// is not preferred.
	RawProtected cbor.RawMessage

	// Protected contains parameters that are to be cryptographically protected.
	// When encoding or signing, the protected header is encoded using the
	// default CBOR encoder if RawProtected is set to nil. Otherwise,
	// RawProtected will be used with Protected ignored.
	Protected ProtectedHeader

	// RawUnprotected contains the raw CBOR encoded data for the unprotected
	// header. It is populated when decoding.
	// Applications can use this field for customized encoding / decoding of
	// the unprotected header in case the default decoder provided by this
	// library is not preferred.
	RawUnprotected cbor.RawMessage

	// Unprotected contains parameters that are not cryptographically protected.
	// When encoding, the unprotected header is encoded using the default CBOR
	// encoder if RawUnprotected is set to nil. Otherwise, RawUnprotected will
	// be used with Unprotected ignored.
	Unprotected UnprotectedHeader
}

// marshal encoded both headers.
// It returns RawProtected and RawUnprotected if those are set.
func (h *Headers) marshal() (cbor.RawMessage, cbor.RawMessage, error) {
	if err := h.ensureIV(); err != nil {
		return nil, nil, err
	}
	protected, err := h.MarshalProtected()
	if err != nil {
		return nil, nil, err
	}
	unprotected, err := h.MarshalUnprotected()
	if err != nil {
		return nil, nil, err
	}
	return protected, unprotected, nil
}

// MarshalProtected encodes the protected header.
// RawProtected is returned if it is not set to nil.
func (h *Headers) MarshalProtected() ([]byte, error) {
	if len(h.RawProtected) > 0 {
		return h.RawProtected, nil
	}
	return encMode.Marshal(h.Protected)
}

// MarshalUnprotected encodes the unprotected header.
// RawUnprotected is returned if it is not set to nil.
func (h *Headers) MarshalUnprotected() ([]byte, error) {
	if len(h.RawUnprotected) > 0 {
		return h.RawUnprotected, nil
	}
	return encMode.Marshal(h.Unprotected)
}

// UnmarshalFromRaw decodes Protected from RawProtected and Unprotected from
// RawUnprotected.
func (h *Headers) UnmarshalFromRaw() error {
	if err := decMode.Unmarshal(h.RawProtected, &h.Protected); err != nil {
		return fmt.Errorf("cbor: invalid protected header: %w", err)
	}
	if err := decMode.Unmarshal(h.RawUnprotected, &h.Unprotected); err != nil {
		return fmt.Errorf("cbor: invalid unprotected header: %w", err)
	}
	if err := h.ensureIV(); err != nil {
		return err
	}
	return nil
}

// ensureSigningAlgorithm ensures the presence of the `alg` header if there is
// no externally supplied data for signing.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
func (h *Headers) ensureSigningAlgorithm(alg Algorithm, external []byte) error {
	candidate, err := h.Protected.Algorithm()
	switch err {
	case nil:
		if candidate != alg {
			return fmt.Errorf("%w: signer %v: header %v", ErrAlgorithmMismatch, alg, candidate)
		}
		return nil
	case ErrAlgorithmNotFound:
		if len(external) > 0 {
			return nil
		}
		if h.RawProtected != nil {
			return ErrAlgorithmNotFound
		}
		if h.Protected == nil {
			h.Protected = make(ProtectedHeader)
		}
		h.Protected.SetAlgorithm(alg)
		return nil
	}
	return err
}

// ensureVerificationAlgorithm ensures the presence of the `alg` header if there
// is no externally supplied data for verification.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-4.4
func (h *Headers) ensureVerificationAlgorithm(alg Algorithm, external []byte) error {
	candidate, err := h.Protected.Algorithm()
	switch err {
	case nil:
		if candidate != alg {
			return fmt.Errorf("%w: verifier %v: header %v", ErrAlgorithmMismatch, alg, candidate)
		}
		return nil
	case ErrAlgorithmNotFound:
		if len(external) > 0 {
			return nil
		}
	}
	return err
}

// ensureIV ensures IV and Partial IV are not both present
// in the protected and unprotected headers.
// It does not check if they are both present within one header,
// as it will be checked later on.
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-3.1
func (h *Headers) ensureIV() error {
	if hasLabel(h.Protected, HeaderLabelIV) && hasLabel(h.Unprotected, HeaderLabelPartialIV) {
		return errors.New("IV (protected) and PartialIV (unprotected) parameters must not both be present")
	}
	if hasLabel(h.Protected, HeaderLabelPartialIV) && hasLabel(h.Unprotected, HeaderLabelIV) {
		return errors.New("IV (unprotected) and PartialIV (protected) parameters must not both be present")
	}
	return nil
}

// hasLabel returns true if h contains label.
func hasLabel(h map[interface{}]interface{}, label interface{}) bool {
	_, ok := h[label]
	return ok
}

// validateHeaderParameters validates all headers conform to the spec.
func validateHeaderParameters(h map[interface{}]interface{}, protected bool) error {
	existing := make(map[interface{}]struct{}, len(h))
	for label, value := range h {
		// Validate that all header labels are integers or strings.
		// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-1.4
		label, ok := normalizeLabel(label)
		if !ok {
			return errors.New("header label: require int / tstr type")
		}

		// Validate that there are no duplicated labels.
		// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-3
		if _, ok := existing[label]; ok {
			return fmt.Errorf("header label: duplicated label: %v", label)
		} else {
			existing[label] = struct{}{}
		}

		// Validate the generic parameters.
		// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-3.1
		switch label {
		case HeaderLabelAlgorithm:
			_, is_alg := value.(Algorithm)
			if !is_alg && !canInt(value) && !canTstr(value) {
				return errors.New("header parameter: alg: require int / tstr type")
			}
		case HeaderLabelCritical:
			if !protected {
				return errors.New("header parameter: crit: not allowed")
			}
			if err := ensureCritical(value, h); err != nil {
				return fmt.Errorf("header parameter: crit: %w", err)
			}
		case HeaderLabelContentType:
			if !canTstr(value) && !canUint(value) {
				return errors.New("header parameter: content type: require tstr / uint type")
			}
		case HeaderLabelKeyID:
			if !canBstr(value) {
				return errors.New("header parameter: kid: require bstr type")
			}
		case HeaderLabelIV:
			if !canBstr(value) {
				return errors.New("header parameter: IV: require bstr type")
			}
			if hasLabel(h, HeaderLabelPartialIV) {
				return errors.New("header parameter: IV and PartialIV: parameters must not both be present")
			}
		case HeaderLabelPartialIV:
			if !canBstr(value) {
				return errors.New("header parameter: Partial IV: require bstr type")
			}
			if hasLabel(h, HeaderLabelIV) {
				return errors.New("header parameter: IV and PartialIV: parameters must not both be present")
			}
		}
	}
	return nil
}

// canUint reports whether v can be used as a CBOR uint type.
func canUint(v interface{}) bool {
	switch v := v.(type) {
	case uint, uint8, uint16, uint32, uint64:
		return true
	case int:
		return v >= 0
	case int8:
		return v >= 0
	case int16:
		return v >= 0
	case int32:
		return v >= 0
	case int64:
		return v >= 0
	}
	return false
}

// canInt reports whether v can be used as a CBOR int type.
func canInt(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return true
	}
	return false
}

// canTstr reports whether v can be used as a CBOR tstr type.
func canTstr(v interface{}) bool {
	_, ok := v.(string)
	return ok
}

// canBstr reports whether v can be used as a CBOR bstr type.
func canBstr(v interface{}) bool {
	_, ok := v.([]byte)
	return ok
}

// normalizeLabel tries to cast label into a int64 or a string.
// Returns (nil, false) if the label type is not valid.
func normalizeLabel(label interface{}) (interface{}, bool) {
	switch v := label.(type) {
	case int:
		label = int64(v)
	case int8:
		label = int64(v)
	case int16:
		label = int64(v)
	case int32:
		label = int64(v)
	case int64:
		label = int64(v)
	case uint:
		label = int64(v)
	case uint8:
		label = int64(v)
	case uint16:
		label = int64(v)
	case uint32:
		label = int64(v)
	case uint64:
		label = int64(v)
	case string:
		// no conversion
	default:
		return nil, false
	}
	return label, true
}

// headerLabelValidator is used to validate the header label of a COSE header.
type headerLabelValidator struct {
	value interface{}
}

// String prints the value without brackets `{}`. Useful in error printing.
func (hlv headerLabelValidator) String() string {
	return fmt.Sprint(hlv.value)
}

// UnmarshalCBOR decodes the label value of a COSE header, and returns error if
// label is not a int (major type 0, 1) or string (major type 3).
func (hlv *headerLabelValidator) UnmarshalCBOR(data []byte) error {
	if len(data) == 0 {
		return errors.New("cbor: header label: missing type")
	}
	switch data[0] >> 5 {
	case 0, 1, 3:
		err := decMode.Unmarshal(data, &hlv.value)
		if err != nil {
			return err
		}
		if _, ok := hlv.value.(big.Int); ok {
			return errors.New("cbor: header label: int key must not be higher than 1<<63 - 1")
		}
		return nil
	}
	return errors.New("cbor: header label: require int / tstr type")
}

// discardedCBORMessage is used to read CBOR message and discard it.
type discardedCBORMessage struct{}

// UnmarshalCBOR discards the read CBOR object.
func (discardedCBORMessage) UnmarshalCBOR(data []byte) error {
	return nil
}

// validateHeaderLabelCBOR validates if all header labels are integers or
// strings of a CBOR map object.
//
//	label = int / tstr
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-1.4
func validateHeaderLabelCBOR(data []byte) error {
	var header map[headerLabelValidator]discardedCBORMessage
	return decMode.Unmarshal(data, &header)
}
