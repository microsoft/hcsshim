package cose

import (
	"bytes"
	"errors"
	"io"

	"github.com/fxamacker/cbor/v2"
)

// CBOR Tags for COSE signatures registered in the IANA "CBOR Tags" registry.
//
// Reference: https://www.iana.org/assignments/cbor-tags/cbor-tags.xhtml#tags
const (
	CBORTagSignMessage  = 98
	CBORTagSign1Message = 18
)

// Pre-configured modes for CBOR encoding and decoding.
var (
	encMode                  cbor.EncMode
	decMode                  cbor.DecMode
	decModeWithTagsForbidden cbor.DecMode
)

func init() {
	var err error

	// init encode mode
	encOpts := cbor.EncOptions{
		Sort:        cbor.SortCoreDeterministic, // sort map keys
		IndefLength: cbor.IndefLengthForbidden,  // no streaming
	}
	encMode, err = encOpts.EncMode()
	if err != nil {
		panic(err)
	}

	// init decode mode
	decOpts := cbor.DecOptions{
		DupMapKey:   cbor.DupMapKeyEnforcedAPF, // duplicated key not allowed
		IndefLength: cbor.IndefLengthForbidden, // no streaming
		IntDec:      cbor.IntDecConvertSigned,  // decode CBOR uint/int to Go int64
	}
	decMode, err = decOpts.DecMode()
	if err != nil {
		panic(err)
	}
	decOpts.TagsMd = cbor.TagsForbidden
	decModeWithTagsForbidden, err = decOpts.DecMode()
	if err != nil {
		panic(err)
	}
}

// byteString represents a "bstr / nil" type.
type byteString []byte

// UnmarshalCBOR decodes data into a "bstr / nil" type.
// It also ensures the data is of major type 2 since []byte can be alternatively
// interpreted as an array of bytes.
//
// Note: `github.com/fxamacker/cbor/v2` considers the primitive value
// `undefined` (major type 7, value 23) as nil, which is not recognized by COSE.
//
// Related Code: https://github.com/fxamacker/cbor/blob/v2.4.0/decode.go#L709
//
// Reference: https://datatracker.ietf.org/doc/html/rfc8152#section-1.3
func (s *byteString) UnmarshalCBOR(data []byte) error {
	if s == nil {
		return errors.New("cbor: UnmarshalCBOR on nil byteString pointer")
	}
	if len(data) == 0 {
		return io.EOF // same error as returned by cbor.Unmarshal()
	}
	if bytes.Equal(data, []byte{0xf6}) {
		*s = nil
		return nil
	}
	if data[0]>>5 != 2 { // major type 2: bstr
		return errors.New("cbor: require bstr type")
	}
	return decModeWithTagsForbidden.Unmarshal(data, (*[]byte)(s))
}

// deterministicBinaryString converts a bstr into the deterministic encoding.
//
// Reference: https://www.rfc-editor.org/rfc/rfc9052.html#section-9
func deterministicBinaryString(data cbor.RawMessage) (cbor.RawMessage, error) {
	if len(data) == 0 {
		return nil, io.EOF
	}
	if data[0]>>5 != 2 { // major type 2: bstr
		return nil, errors.New("cbor: require bstr type")
	}

	// fast path: return immediately if bstr is already deterministic
	if err := decModeWithTagsForbidden.Valid(data); err != nil {
		return nil, err
	}
	ai := data[0] & 0x1f
	if ai < 24 {
		return data, nil
	}
	switch ai {
	case 24:
		if data[1] >= 24 {
			return data, nil
		}
	case 25:
		if data[1] != 0 {
			return data, nil
		}
	case 26:
		if data[1] != 0 || data[2] != 0 {
			return data, nil
		}
	case 27:
		if data[1] != 0 || data[2] != 0 || data[3] != 0 || data[4] != 0 {
			return data, nil
		}
	}

	// slow path: convert by re-encoding
	// error checking is not required since `data` has been validataed
	var s []byte
	_ = decModeWithTagsForbidden.Unmarshal(data, &s)
	return encMode.Marshal(s)
}
