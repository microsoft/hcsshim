package cose

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"errors"
	"fmt"
	"math/big"
	"strconv"

	cbor "github.com/fxamacker/cbor/v2"
)

const (
	// An inviald key_op value
	KeyOpInvalid KeyOp = 0

	// The key is used to create signatures. Requires private key fields.
	KeyOpSign KeyOp = 1

	// The key is used for verification of signatures.
	KeyOpVerify KeyOp = 2

	// The key is used for key transport encryption.
	KeyOpEncrypt KeyOp = 3

	// The key is used for key transport decryption. Requires private key fields.
	KeyOpDecrypt KeyOp = 4

	// The key is used for key wrap encryption.
	KeyOpWrapKey KeyOp = 5

	// The key is used for key wrap decryption.
	KeyOpUnwrapKey KeyOp = 6

	// The key is used for deriving keys. Requires private key fields.
	KeyOpDeriveKey KeyOp = 7

	// The key is used for deriving bits not to be used as a key. Requires
	// private key fields.
	KeyOpDeriveBits KeyOp = 8

	// The key is used for creating MACs.
	KeyOpMACCreate KeyOp = 9

	// The key is used for validating MACs.
	KeyOpMACVerify KeyOp = 10
)

// KeyOp represents a key_ops value used to restrict purposes for which a Key
// may be used.
type KeyOp int64

// KeyOpFromString returns the KeyOp corresponding to the specified name.
// The values are taken from https://www.rfc-editor.org/rfc/rfc7517#section-4.3
func KeyOpFromString(val string) (KeyOp, error) {
	switch val {
	case "sign":
		return KeyOpSign, nil
	case "verify":
		return KeyOpVerify, nil
	case "encrypt":
		return KeyOpEncrypt, nil
	case "decrypt":
		return KeyOpDecrypt, nil
	case "wrapKey":
		return KeyOpWrapKey, nil
	case "unwrapKey":
		return KeyOpUnwrapKey, nil
	case "deriveKey":
		return KeyOpDeriveKey, nil
	case "deriveBits":
		return KeyOpDeriveBits, nil
	default:
		return KeyOpInvalid, fmt.Errorf("unknown key_ops value %q", val)
	}
}

// String returns a string representation of the KeyType. Note does not
// represent a valid value  of the corresponding serialized entry, and must not
// be used as such. (The values returned _mostly_ correspond to those accepted
// by KeyOpFromString, except for MAC create/verify, which are not defined by
// RFC7517).
func (ko KeyOp) String() string {
	switch ko {
	case KeyOpSign:
		return "sign"
	case KeyOpVerify:
		return "verify"
	case KeyOpEncrypt:
		return "encrypt"
	case KeyOpDecrypt:
		return "decrypt"
	case KeyOpWrapKey:
		return "wrapKey"
	case KeyOpUnwrapKey:
		return "unwrapKey"
	case KeyOpDeriveKey:
		return "deriveKey"
	case KeyOpDeriveBits:
		return "deriveBits"
	case KeyOpMACCreate:
		return "MAC create"
	case KeyOpMACVerify:
		return "MAC verify"
	default:
		return "unknown key_op value " + strconv.Itoa(int(ko))
	}
}

// IsSupported returnns true if the specified value is represents one of the
// key_ops defined in
// https://www.rfc-editor.org/rfc/rfc9052.html#name-cose-key-common-parameters
func (ko KeyOp) IsSupported() bool {
	return ko >= 1 && ko <= 10
}

// MarshalCBOR marshals the KeyOp as a CBOR int.
func (ko KeyOp) MarshalCBOR() ([]byte, error) {
	return encMode.Marshal(int64(ko))
}

// UnmarshalCBOR populates the KeyOp from the provided CBOR value (must be int
// or tstr).
func (ko *KeyOp) UnmarshalCBOR(data []byte) error {
	var raw intOrStr

	if err := raw.UnmarshalCBOR(data); err != nil {
		return fmt.Errorf("invalid key_ops value %w", err)
	}

	if raw.IsString() {
		v, err := KeyOpFromString(raw.String())
		if err != nil {
			return err
		}

		*ko = v
	} else {
		v := raw.Int()
		*ko = KeyOp(v)

		if !ko.IsSupported() {
			return fmt.Errorf("unknown key_ops value %d", v)
		}
	}

	return nil
}

// KeyType identifies the family of keys represented by the associated Key.
// This determines which files within the Key must be set in order for it to be
// valid.
type KeyType int64

const (
	// Invlaid key type
	KeyTypeInvalid KeyType = 0
	// Octet Key Pair
	KeyTypeOKP KeyType = 1
	// Elliptic Curve Keys w/ x- and y-coordinate pair
	KeyTypeEC2 KeyType = 2
	// Symmetric Keys
	KeyTypeSymmetric KeyType = 4
)

// String returns a string representation of the KeyType. Note does not
// represent a valid value  of the corresponding serialized entry, and must
// not be used as such.
func (kt KeyType) String() string {
	switch kt {
	case KeyTypeOKP:
		return "OKP"
	case KeyTypeEC2:
		return "EC2"
	case KeyTypeSymmetric:
		return "Symmetric"
	default:
		return "unknown key type value " + strconv.Itoa(int(kt))
	}
}

// MarshalCBOR marshals the KeyType as a CBOR int.
func (kt KeyType) MarshalCBOR() ([]byte, error) {
	return encMode.Marshal(int(kt))
}

// UnmarshalCBOR populates the KeyType from the provided CBOR value (must be
// int or tstr).
func (kt *KeyType) UnmarshalCBOR(data []byte) error {
	var raw intOrStr

	if err := raw.UnmarshalCBOR(data); err != nil {
		return fmt.Errorf("invalid key type value: %w", err)
	}

	if raw.IsString() {
		v, err := keyTypeFromString(raw.String())

		if err != nil {
			return err
		}

		*kt = v
	} else {
		v := raw.Int()

		if v == 0 {
			// 0  is reserved, and so can never be valid
			return fmt.Errorf("invalid key type value 0")
		}

		if v > 4 || v < 0 || v == 3 {
			return fmt.Errorf("unknown key type value %d", v)
		}

		*kt = KeyType(v)
	}

	return nil
}

// NOTE: there are currently no registered string key type values.
func keyTypeFromString(v string) (KeyType, error) {
	return KeyTypeInvalid, fmt.Errorf("unknown key type value %q", v)
}

const (

	// Invalid/unrecognised curve
	CurveInvalid Curve = 0

	// NIST P-256 also known as secp256r1
	CurveP256 Curve = 1

	// NIST P-384 also known as secp384r1
	CurveP384 Curve = 2

	// NIST P-521 also known as secp521r1
	CurveP521 Curve = 3

	// X25519 for use w/ ECDH only
	CurveX25519 Curve = 4

	// X448 for use w/ ECDH only
	CurveX448 Curve = 5

	// Ed25519 for use /w EdDSA only
	CurveEd25519 Curve = 6

	// Ed448 for use /w EdDSA only
	CurveEd448 Curve = 7
)

// Curve represents the EC2/OKP key's curve. See:
// https://datatracker.ietf.org/doc/html/rfc8152#section-13.1
type Curve int64

// String returns a string representation of the Curve. Note does not
// represent a valid value  of the corresponding serialized entry, and must
// not be used as such.
func (c Curve) String() string {
	switch c {
	case CurveP256:
		return "P-256"
	case CurveP384:
		return "P-384"
	case CurveP521:
		return "P-521"
	case CurveX25519:
		return "X25519"
	case CurveX448:
		return "X448"
	case CurveEd25519:
		return "Ed25519"
	case CurveEd448:
		return "Ed448"
	default:
		return "unknown curve value " + strconv.Itoa(int(c))
	}
}

// MarshalCBOR marshals the KeyType as a CBOR int.
func (c Curve) MarshalCBOR() ([]byte, error) {
	return encMode.Marshal(int(c))
}

// UnmarshalCBOR populates the KeyType from the provided CBOR value (must be
// int or tstr).
func (c *Curve) UnmarshalCBOR(data []byte) error {
	var raw intOrStr

	if err := raw.UnmarshalCBOR(data); err != nil {
		return fmt.Errorf("invalid curve value: %w", err)
	}

	if raw.IsString() {
		v, err := curveFromString(raw.String())

		if err != nil {
			return err
		}

		*c = v
	} else {
		v := raw.Int()

		if v < 1 || v > 7 {
			return fmt.Errorf("unknown curve value %d", v)
		}

		*c = Curve(v)
	}

	return nil
}

// NOTE: there are currently no registered string values for curves.
func curveFromString(v string) (Curve, error) {
	return CurveInvalid, fmt.Errorf("unknown curve value %q", v)
}

// Key represents a COSE_Key structure, as defined by RFC8152.
// Note: currently, this does NOT support RFC8230 (RSA algorithms).
type Key struct {
	// Common parameters. These are independent of the key type. Only
	// KeyType common parameter MUST be set.

	// KeyType identifies the family of keys for this structure, and thus,
	// which of the key-type-specific parameters need to be set.
	KeyType KeyType `cbor:"1,keyasint"`
	// KeyID is the identification value matched to the kid in the message.
	KeyID []byte `cbor:"2,keyasint,omitempty"`
	// KeyOps can be set to restrict the set of operations that the Key is used for.
	KeyOps []KeyOp `cbor:"4,keyasint,omitempty"`
	// BaseIV is the Base IV to be xor-ed with Partial IVs.
	BaseIV []byte `cbor:"5,keyasint,omitempty"`

	// Algorithm is used to restrict the algorithm that is used with the
	// key. If it is set, the application MUST verify that it matches the
	// algorithm for which the Key is being used.
	Algorithm Algorithm `cbor:"-"`
	// Curve is EC identifier -- taken form "COSE Elliptic Curves" IANA registry.
	// Populated from keyStruct.RawKeyParam when key type is EC2 or OKP.
	Curve Curve `cbor:"-"`
	// K is the key value. Populated from keyStruct.RawKeyParam when key
	// type is Symmetric.
	K []byte `cbor:"-"`

	// EC2/OKP params

	// X is the x-coordinate
	X []byte `cbor:"-2,keyasint,omitempty"`
	// Y is the y-coordinate (sign bits are not supported)
	Y []byte `cbor:"-3,keyasint,omitempty"`
	// D is the private key
	D []byte `cbor:"-4,keyasint,omitempty"`
}

// NewOKPKey returns a Key created using the provided Octet Key Pair data.
func NewOKPKey(alg Algorithm, x, d []byte) (*Key, error) {
	if alg != AlgorithmEd25519 {
		return nil, fmt.Errorf("unsupported algorithm %q", alg)
	}

	key := &Key{
		KeyType:   KeyTypeOKP,
		Algorithm: alg,
		Curve:     CurveEd25519,
		X:         x,
		D:         d,
	}
	return key, key.Validate()
}

// NewEC2Key returns a Key created using the provided elliptic curve key
// data.
func NewEC2Key(alg Algorithm, x, y, d []byte) (*Key, error) {
	var curve Curve

	switch alg {
	case AlgorithmES256:
		curve = CurveP256
	case AlgorithmES384:
		curve = CurveP384
	case AlgorithmES512:
		curve = CurveP521
	default:
		return nil, fmt.Errorf("unsupported algorithm %q", alg)
	}

	key := &Key{
		KeyType:   KeyTypeEC2,
		Algorithm: alg,
		Curve:     curve,
		X:         x,
		Y:         y,
		D:         d,
	}
	return key, key.Validate()
}

// NewSymmetricKey returns a Key created using the provided Symmetric key
// bytes.
func NewSymmetricKey(k []byte) (*Key, error) {
	key := &Key{
		KeyType: KeyTypeSymmetric,
		K:       k,
	}
	return key, key.Validate()
}

// NewKeyFromPublic returns a Key created using the provided crypto.PublicKey
// and Algorithm.
func NewKeyFromPublic(alg Algorithm, pub crypto.PublicKey) (*Key, error) {
	switch alg {
	case AlgorithmES256, AlgorithmES384, AlgorithmES512:
		vk, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%v: %w", alg, ErrInvalidPubKey)
		}

		return NewEC2Key(alg, vk.X.Bytes(), vk.Y.Bytes(), nil)
	case AlgorithmEd25519:
		vk, ok := pub.(ed25519.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%v: %w", alg, ErrInvalidPubKey)
		}

		return NewOKPKey(alg, []byte(vk), nil)
	default:
		return nil, ErrAlgorithmNotSupported
	}
}

// NewKeyFromPrivate returns a Key created using provided crypto.PrivateKey
// and Algorithm.
func NewKeyFromPrivate(alg Algorithm, priv crypto.PrivateKey) (*Key, error) {
	switch alg {
	case AlgorithmES256, AlgorithmES384, AlgorithmES512:
		sk, ok := priv.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%v: %w", alg, ErrInvalidPrivKey)
		}

		return NewEC2Key(alg, sk.X.Bytes(), sk.Y.Bytes(), sk.D.Bytes())
	case AlgorithmEd25519:
		sk, ok := priv.(ed25519.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%v: %w", alg, ErrInvalidPrivKey)
		}
		return NewOKPKey(alg, []byte(sk[32:]), []byte(sk[:32]))
	default:
		return nil, ErrAlgorithmNotSupported
	}
}

// Validate ensures that the parameters set inside the Key are internally
// consistent (e.g., that the key type is appropriate to the curve.)
func (k Key) Validate() error {
	switch k.KeyType {
	case KeyTypeEC2:
		switch k.Curve {
		case CurveP256, CurveP384, CurveP521:
			// ok
		default:
			return fmt.Errorf(
				"EC2 curve must be P-256, P-384, or P-521; found %q",
				k.Curve.String(),
			)
		}
	case KeyTypeOKP:
		switch k.Curve {
		case CurveX25519, CurveX448, CurveEd25519, CurveEd448:
			// ok
		default:
			return fmt.Errorf(
				"OKP curve must be X25519, X448, Ed25519, or Ed448; found %q",
				k.Curve.String(),
			)
		}
	case KeyTypeSymmetric:
	default:
		return errors.New(k.KeyType.String())
	}

	// If Algorithm is set, it must match the specified key parameters.
	if k.Algorithm != AlgorithmInvalid {
		expectedAlg, err := k.deriveAlgorithm()
		if err != nil {
			return err
		}

		if k.Algorithm != expectedAlg {
			return fmt.Errorf(
				"found algorithm %q (expected %q)",
				k.Algorithm.String(),
				expectedAlg.String(),
			)
		}
	}

	return nil
}

type keyalias Key

type marshaledKey struct {
	keyalias

	// RawAlgorithm contains the raw Algorithm value, this is necessary
	// because cbor library ignores omitempty on types that implement the
	// cbor.Marshaler interface.
	RawAlgorithm cbor.RawMessage `cbor:"3,keyasint,omitempty"`

	// RawKeyParam contains the raw CBOR encoded data for the label -1.
	// Depending on the KeyType this is used to populate either Curve or K
	// below.
	RawKeyParam cbor.RawMessage `cbor:"-1,keyasint,omitempty"`
}

// MarshalCBOR encodes Key into a COSE_Key object.
func (k *Key) MarshalCBOR() ([]byte, error) {
	tmp := marshaledKey{
		keyalias: keyalias(*k),
	}
	var err error

	switch k.KeyType {
	case KeyTypeSymmetric:
		if tmp.RawKeyParam, err = encMode.Marshal(k.K); err != nil {
			return nil, err
		}
	case KeyTypeEC2, KeyTypeOKP:
		if tmp.RawKeyParam, err = encMode.Marshal(k.Curve); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid key type: %q", k.KeyType.String())
	}

	if k.Algorithm != AlgorithmInvalid {
		if tmp.RawAlgorithm, err = encMode.Marshal(k.Algorithm); err != nil {
			return nil, err
		}
	}

	return encMode.Marshal(tmp)
}

// UnmarshalCBOR decodes a COSE_Key object into Key.
func (k *Key) UnmarshalCBOR(data []byte) error {
	var tmp marshaledKey

	if err := decMode.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*k = Key(tmp.keyalias)

	if tmp.RawAlgorithm != nil {
		if err := decMode.Unmarshal(tmp.RawAlgorithm, &k.Algorithm); err != nil {
			return err
		}
	}

	switch k.KeyType {
	case KeyTypeEC2:
		if tmp.RawKeyParam == nil {
			return errors.New("missing Curve parameter (required for EC2 key type)")
		}

		if err := decMode.Unmarshal(tmp.RawKeyParam, &k.Curve); err != nil {
			return err
		}
	case KeyTypeOKP:
		if tmp.RawKeyParam == nil {
			return errors.New("missing Curve parameter (required for OKP key type)")
		}

		if err := decMode.Unmarshal(tmp.RawKeyParam, &k.Curve); err != nil {
			return err
		}
	case KeyTypeSymmetric:
		if tmp.RawKeyParam == nil {
			return errors.New("missing K parameter (required for Symmetric key type)")
		}

		if err := decMode.Unmarshal(tmp.RawKeyParam, &k.K); err != nil {
			return err
		}
	default:
		// this should not be reachable as KeyType.UnmarshalCBOR would
		// result in an error during decMode.Unmarshal() above, if the
		// value in the data doesn't correspond to one of the above
		// types.
		return fmt.Errorf("unexpected key type %q", k.KeyType.String())
	}

	return k.Validate()
}

// PublicKey returns a crypto.PublicKey generated using Key's parameters.
func (k *Key) PublicKey() (crypto.PublicKey, error) {
	alg, err := k.deriveAlgorithm()
	if err != nil {
		return nil, err
	}

	switch alg {
	case AlgorithmES256, AlgorithmES384, AlgorithmES512:
		var curve elliptic.Curve

		switch alg {
		case AlgorithmES256:
			curve = elliptic.P256()
		case AlgorithmES384:
			curve = elliptic.P384()
		case AlgorithmES512:
			curve = elliptic.P521()
		}

		pub := &ecdsa.PublicKey{Curve: curve, X: new(big.Int), Y: new(big.Int)}
		pub.X.SetBytes(k.X)
		pub.Y.SetBytes(k.Y)

		return pub, nil
	case AlgorithmEd25519:
		return ed25519.PublicKey(k.X), nil
	default:
		return nil, ErrAlgorithmNotSupported
	}
}

// PrivateKey returns a crypto.PrivateKey generated using Key's parameters.
func (k *Key) PrivateKey() (crypto.PrivateKey, error) {
	alg, err := k.deriveAlgorithm()
	if err != nil {
		return nil, err
	}

	if len(k.D) == 0 {
		return nil, ErrNotPrivKey
	}

	switch alg {
	case AlgorithmES256, AlgorithmES384, AlgorithmES512:
		var curve elliptic.Curve

		switch alg {
		case AlgorithmES256:
			curve = elliptic.P256()
		case AlgorithmES384:
			curve = elliptic.P384()
		case AlgorithmES512:
			curve = elliptic.P521()
		}

		priv := &ecdsa.PrivateKey{
			PublicKey: ecdsa.PublicKey{Curve: curve, X: new(big.Int), Y: new(big.Int)},
			D:         new(big.Int),
		}
		priv.X.SetBytes(k.X)
		priv.Y.SetBytes(k.Y)
		priv.D.SetBytes(k.D)

		return priv, nil
	case AlgorithmEd25519:
		buf := make([]byte, ed25519.PrivateKeySize)

		copy(buf, k.D)
		copy(buf[32:], k.X)

		return ed25519.PrivateKey(buf), nil
	default:
		return nil, ErrAlgorithmNotSupported
	}
}

// AlgorithmOrDefault returns the Algorithm associated with Key. If Key.Algorithm is
// set, that is what is returned. Otherwise, the algorithm is inferred using
// Key.Curve. This method does NOT validate that Key.Algorithm, if set, aligns
// with Key.Curve.
func (k *Key) AlgorithmOrDefault() (Algorithm, error) {
	if k.Algorithm != AlgorithmInvalid {
		return k.Algorithm, nil
	}

	return k.deriveAlgorithm()
}

// Signer returns a Signer created using Key.
func (k *Key) Signer() (Signer, error) {
	if err := k.Validate(); err != nil {
		return nil, err
	}

	if k.KeyOps != nil {
		signFound := false

		for _, kop := range k.KeyOps {
			if kop == KeyOpSign {
				signFound = true
				break
			}
		}

		if !signFound {
			return nil, ErrSignOpNotSupported
		}
	}

	priv, err := k.PrivateKey()
	if err != nil {
		return nil, err
	}

	alg, err := k.AlgorithmOrDefault()
	if err != nil {
		return nil, err
	}

	var signer crypto.Signer
	var ok bool

	switch alg {
	case AlgorithmES256, AlgorithmES384, AlgorithmES512:
		signer, ok = priv.(*ecdsa.PrivateKey)
		if !ok {
			return nil, ErrInvalidPrivKey
		}
	case AlgorithmEd25519:
		signer, ok = priv.(ed25519.PrivateKey)
		if !ok {
			return nil, ErrInvalidPrivKey
		}
	default:
		return nil, ErrAlgorithmNotSupported
	}

	return NewSigner(alg, signer)
}

// Verifier returns a Verifier created using Key.
func (k *Key) Verifier() (Verifier, error) {
	if err := k.Validate(); err != nil {
		return nil, err
	}

	if k.KeyOps != nil {
		verifyFound := false

		for _, kop := range k.KeyOps {
			if kop == KeyOpVerify {
				verifyFound = true
				break
			}
		}

		if !verifyFound {
			return nil, ErrVerifyOpNotSupported
		}
	}

	pub, err := k.PublicKey()
	if err != nil {
		return nil, err
	}

	alg, err := k.AlgorithmOrDefault()
	if err != nil {
		return nil, err
	}

	return NewVerifier(alg, pub)
}

// deriveAlgorithm derives the intended algorithm for the key from its curve.
// The deriviation is based on the recommendation in RFC8152 that SHA-256 is
// only used with P-256, etc. For other combinations, the Algorithm in the Key
// must be explicitly set,so that this derivation is not used.
func (k *Key) deriveAlgorithm() (Algorithm, error) {
	switch k.KeyType {
	case KeyTypeEC2, KeyTypeOKP:
		switch k.Curve {
		case CurveP256:
			return AlgorithmES256, nil
		case CurveP384:
			return AlgorithmES384, nil
		case CurveP521:
			return AlgorithmES512, nil
		case CurveEd25519:
			return AlgorithmEd25519, nil
		default:
			return AlgorithmInvalid, fmt.Errorf("unsupported curve %q", k.Curve.String())
		}
	default:
		// Symmetric algorithms are not supported in the current inmplementation.
		return AlgorithmInvalid, fmt.Errorf("unexpected key type %q", k.KeyType.String())
	}
}
