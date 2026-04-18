package cose

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
)

const (
	KeyLabelOKPCurve int64 = -1
	KeyLabelOKPX     int64 = -2
	KeyLabelOKPD     int64 = -4

	KeyLabelEC2Curve int64 = -1
	KeyLabelEC2X     int64 = -2
	KeyLabelEC2Y     int64 = -3
	KeyLabelEC2D     int64 = -4

	KeyLabelSymmetricK int64 = -1
)

const (
	keyLabelKeyType   int64 = 1
	keyLabelKeyID     int64 = 2
	keyLabelAlgorithm int64 = 3
	keyLabelKeyOps    int64 = 4
	keyLabelBaseIV    int64 = 5
)

// KeyOp represents a key_ops value used to restrict purposes for which a Key
// may be used.
//
// https://datatracker.ietf.org/doc/html/rfc8152#section-7.1
type KeyOp int64

const (
	// Reserved value.
	KeyOpReserved KeyOp = 0

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

// KeyOpFromString returns the KeyOp corresponding to the specified name.
// The values are taken from https://www.rfc-editor.org/rfc/rfc7517#section-4.3
func KeyOpFromString(val string) (KeyOp, bool) {
	switch val {
	case "sign":
		return KeyOpSign, true
	case "verify":
		return KeyOpVerify, true
	case "encrypt":
		return KeyOpEncrypt, true
	case "decrypt":
		return KeyOpDecrypt, true
	case "wrapKey":
		return KeyOpWrapKey, true
	case "unwrapKey":
		return KeyOpUnwrapKey, true
	case "deriveKey":
		return KeyOpDeriveKey, true
	case "deriveBits":
		return KeyOpDeriveBits, true
	default:
		return KeyOpReserved, false
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
	case KeyOpReserved:
		return "Reserved"
	default:
		return "unknown key_op value " + strconv.Itoa(int(ko))
	}
}

// KeyType identifies the family of keys represented by the associated Key.
//
// https://datatracker.ietf.org/doc/html/rfc8152#section-13
type KeyType int64

const (
	KeyTypeReserved  KeyType = 0
	KeyTypeOKP       KeyType = 1
	KeyTypeEC2       KeyType = 2
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
	case KeyTypeReserved:
		return "Reserved"
	default:
		return "unknown key type value " + strconv.Itoa(int(kt))
	}
}

// Curve represents the EC2/OKP key's curve.
//
// https://datatracker.ietf.org/doc/html/rfc8152#section-13.1
type Curve int64

const (
	// Reserved value
	CurveReserved Curve = 0

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
	case CurveReserved:
		return "Reserved"
	default:
		return "unknown curve value " + strconv.Itoa(int(c))
	}
}

// Key represents a COSE_Key structure, as defined by RFC8152.
// Note: currently, this does NOT support RFC8230 (RSA algorithms).
type Key struct {
	// Type identifies the family of keys for this structure, and thus,
	// which of the key-type-specific parameters need to be set.
	Type KeyType
	// ID is the identification value matched to the kid in the message.
	ID []byte
	// Algorithm is used to restrict the algorithm that is used with the
	// key. If it is set, the application MUST verify that it matches the
	// algorithm for which the Key is being used.
	Algorithm Algorithm
	// Ops can be set to restrict the set of operations that the Key is used for.
	Ops []KeyOp
	// BaseIV is the Base IV to be xor-ed with Partial IVs.
	BaseIV []byte

	// Any additional parameter (label,value) pairs.
	Params map[any]any
}

// NewKeyOKP returns a Key created using the provided Octet Key Pair data.
func NewKeyOKP(alg Algorithm, x, d []byte) (*Key, error) {
	if alg != AlgorithmEdDSA {
		return nil, fmt.Errorf("unsupported algorithm %q", alg)
	}

	key := &Key{
		Type:      KeyTypeOKP,
		Algorithm: alg,
		Params: map[any]any{
			KeyLabelOKPCurve: CurveEd25519,
		},
	}
	if x != nil {
		key.Params[KeyLabelOKPX] = x
	}
	if d != nil {
		key.Params[KeyLabelOKPD] = d
	}
	if err := key.validate(KeyOpReserved); err != nil {
		return nil, err
	}
	return key, nil
}

// ParamBytes returns the value of the parameter with the given label, if it
// exists and is of type []byte or can be converted to []byte.
func (k *Key) ParamBytes(label any) ([]byte, bool) {
	v, ok, err := decodeBytes(k.Params, label)
	return v, ok && err == nil
}

// ParamInt returns the value of the parameter with the given label, if it
// exists and is of type int64 or can be converted to int64.
func (k *Key) ParamInt(label any) (int64, bool) {
	v, ok, err := decodeInt(k.Params, label)
	return v, ok && err == nil
}

// ParamUint returns the value of the parameter with the given label, if it
// exists and is of type uint64 or can be converted to uint64.
func (k *Key) ParamUint(label any) (uint64, bool) {
	v, ok, err := decodeUint(k.Params, label)
	return v, ok && err == nil
}

// ParamString returns the value of the parameter with the given label, if it
// exists and is of type string or can be converted to string.
func (k *Key) ParamString(label any) (string, bool) {
	v, ok, err := decodeString(k.Params, label)
	return v, ok && err == nil
}

// ParamBool returns the value of the parameter with the given label, if it
// exists and is of type bool or can be converted to bool.
func (k *Key) ParamBool(label any) (bool, bool) {
	v, ok, err := decodeBool(k.Params, label)
	return v, ok && err == nil
}

// OKP returns the Octet Key Pair parameters for the key.
func (k *Key) OKP() (crv Curve, x []byte, d []byte) {
	v, ok := k.ParamInt(KeyLabelOKPCurve)
	if ok {
		crv = Curve(v)
	}
	x, _ = k.ParamBytes(KeyLabelOKPX)
	d, _ = k.ParamBytes(KeyLabelOKPD)
	return
}

// NewKeyEC2 returns a Key created using the provided elliptic curve key
// data.
func NewKeyEC2(alg Algorithm, x, y, d []byte) (*Key, error) {
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
		Type:      KeyTypeEC2,
		Algorithm: alg,
		Params: map[any]any{
			KeyLabelEC2Curve: curve,
		},
	}
	if x != nil {
		key.Params[KeyLabelEC2X] = x
	}
	if y != nil {
		key.Params[KeyLabelEC2Y] = y
	}
	if d != nil {
		key.Params[KeyLabelEC2D] = d
	}
	if err := key.validate(KeyOpReserved); err != nil {
		return nil, err
	}
	return key, nil
}

// EC2 returns the Elliptic Curve parameters for the key.
func (k *Key) EC2() (crv Curve, x []byte, y, d []byte) {
	v, ok := k.ParamInt(KeyLabelEC2Curve)
	if ok {
		crv = Curve(v)
	}
	x, _ = k.ParamBytes(KeyLabelEC2X)
	y, _ = k.ParamBytes(KeyLabelEC2Y)
	d, _ = k.ParamBytes(KeyLabelEC2D)
	return
}

// NewKeySymmetric returns a Key created using the provided Symmetric key
// bytes.
func NewKeySymmetric(k []byte) *Key {
	return &Key{
		Type: KeyTypeSymmetric,
		Params: map[any]any{
			KeyLabelSymmetricK: k,
		},
	}
}

// Symmetric returns the Symmetric parameters for the key.
func (key *Key) Symmetric() (k []byte) {
	k, _ = key.ParamBytes(KeyLabelSymmetricK)
	return
}

// NewKeyFromPublic returns a Key created using the provided crypto.PublicKey.
// Supported key formats are: *ecdsa.PublicKey and ed25519.PublicKey
func NewKeyFromPublic(pub crypto.PublicKey) (*Key, error) {
	switch vk := pub.(type) {
	case *ecdsa.PublicKey:
		alg := algorithmFromEllipticCurve(vk.Curve)

		if alg == AlgorithmReserved {
			return nil, fmt.Errorf("unsupported curve: %v", vk.Curve)
		}

		return NewKeyEC2(alg, vk.X.Bytes(), vk.Y.Bytes(), nil)
	case ed25519.PublicKey:
		return NewKeyOKP(AlgorithmEdDSA, []byte(vk), nil)
	default:
		return nil, ErrInvalidPubKey
	}
}

// NewKeyFromPrivate returns a Key created using provided crypto.PrivateKey.
// Supported key formats are: *ecdsa.PrivateKey and ed25519.PrivateKey
func NewKeyFromPrivate(priv crypto.PrivateKey) (*Key, error) {
	switch sk := priv.(type) {
	case *ecdsa.PrivateKey:
		alg := algorithmFromEllipticCurve(sk.Curve)

		if alg == AlgorithmReserved {
			return nil, fmt.Errorf("unsupported curve: %v", sk.Curve)
		}

		return NewKeyEC2(alg, sk.X.Bytes(), sk.Y.Bytes(), sk.D.Bytes())
	case ed25519.PrivateKey:
		return NewKeyOKP(AlgorithmEdDSA, []byte(sk[32:]), []byte(sk[:32]))
	default:
		return nil, ErrInvalidPrivKey
	}
}

var (
	// The following errors are used multiple times
	// in Key.validate. We declare them here to avoid
	// duplication. They are not considered public errors.
	errCoordOverflow    = fmt.Errorf("%w: overflowing coordinate", ErrInvalidKey)
	errReqParamsMissing = fmt.Errorf("%w: required parameters missing", ErrInvalidKey)
	errInvalidCurve     = fmt.Errorf("%w: curve not supported for the given key type", ErrInvalidKey)
)

// Validate ensures that the parameters set inside the Key are internally
// consistent (e.g., that the key type is appropriate to the curve).
// It also checks that the key is valid for the requested operation.
func (k Key) validate(op KeyOp) error {
	switch k.Type {
	case KeyTypeEC2:
		crv, x, y, d := k.EC2()
		switch op {
		case KeyOpVerify:
			if len(x) == 0 || len(y) == 0 {
				return ErrEC2NoPub
			}
		case KeyOpSign:
			if len(d) == 0 {
				return ErrNotPrivKey
			}
		}
		if crv == CurveReserved || (len(x) == 0 && len(y) == 0 && len(d) == 0) {
			return errReqParamsMissing
		}
		if size := curveSize(crv); size > 0 {
			// RFC 8152 Section 13.1.1 says that x and y leading zero octets MUST be preserved,
			// but the Go crypto/elliptic package trims them. So we relax the check
			// here to allow for omitted leading zero octets, we will add them back
			// when marshaling.
			if len(x) > size || len(y) > size || len(d) > size {
				return errCoordOverflow
			}
		}
		switch crv {
		case CurveX25519, CurveX448, CurveEd25519, CurveEd448:
			return errInvalidCurve
		default:
			// ok -- a key may contain a currently unsupported curve
			// see https://www.rfc-editor.org/rfc/rfc8152#section-13.1.1
		}
	case KeyTypeOKP:
		crv, x, d := k.OKP()
		switch op {
		case KeyOpVerify:
			if len(x) == 0 {
				return ErrOKPNoPub
			}
		case KeyOpSign:
			if len(d) == 0 {
				return ErrNotPrivKey
			}
		}
		if crv == CurveReserved || (len(x) == 0 && len(d) == 0) {
			return errReqParamsMissing
		}
		if (len(x) > 0 && len(x) != ed25519.PublicKeySize) || (len(d) > 0 && len(d) != ed25519.SeedSize) {
			return errCoordOverflow
		}
		switch crv {
		case CurveP256, CurveP384, CurveP521:
			return errInvalidCurve
		default:
			// ok -- a key may contain a currently unsupported curve
			// see https://www.rfc-editor.org/rfc/rfc8152#section-13.2
		}
	case KeyTypeSymmetric:
		k := k.Symmetric()
		if len(k) == 0 {
			return errReqParamsMissing
		}
	case KeyTypeReserved:
		return fmt.Errorf("%w: kty value 0", ErrInvalidKey)
	default:
		// Unknown key type, we can't validate custom parameters.
	}

	// If Algorithm is set, it must match the specified key parameters.
	if k.Algorithm != AlgorithmReserved {
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

func (k Key) canOp(op KeyOp) bool {
	if k.Ops == nil {
		return true
	}
	for _, kop := range k.Ops {
		if kop == op {
			return true
		}
	}
	return false
}

// MarshalCBOR encodes Key into a COSE_Key object.
func (k *Key) MarshalCBOR() ([]byte, error) {
	tmp := map[any]any{
		keyLabelKeyType: k.Type,
	}
	if k.ID != nil {
		tmp[keyLabelKeyID] = k.ID
	}
	if k.Algorithm != AlgorithmReserved {
		tmp[keyLabelAlgorithm] = k.Algorithm
	}
	if k.Ops != nil {
		tmp[keyLabelKeyOps] = k.Ops
	}
	if k.BaseIV != nil {
		tmp[keyLabelBaseIV] = k.BaseIV
	}
	existing := make(map[any]struct{}, len(k.Params))
	for label, v := range k.Params {
		lbl, ok := normalizeLabel(label)
		if !ok {
			return nil, fmt.Errorf("invalid label type %T", label)
		}
		if _, ok := existing[lbl]; ok {
			return nil, fmt.Errorf("duplicate label %v", lbl)
		}
		existing[lbl] = struct{}{}
		tmp[lbl] = v
	}
	if k.Type == KeyTypeEC2 {
		// If EC2 key, ensure that x and y are padded to the correct size.
		crv, x, y, _ := k.EC2()
		if size := curveSize(crv); size > 0 {
			if 0 < len(x) && len(x) < size {
				tmp[KeyLabelEC2X] = append(make([]byte, size-len(x), size), x...)
			}
			if 0 < len(y) && len(y) < size {
				tmp[KeyLabelEC2Y] = append(make([]byte, size-len(y), size), y...)
			}
		}
	}
	return encMode.Marshal(tmp)
}

// UnmarshalCBOR decodes a COSE_Key object into Key.
func (k *Key) UnmarshalCBOR(data []byte) error {
	var tmp map[any]any
	if err := decMode.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*k = Key{}
	kty, exist, err := decodeInt(tmp, keyLabelKeyType)
	if !exist {
		return errors.New("kty: missing")
	}
	if err != nil {
		return fmt.Errorf("kty: %w", err)
	}
	k.Type = KeyType(kty)
	if k.Type == KeyTypeReserved {
		return errors.New("kty: invalid value 0")
	}
	k.ID, _, err = decodeBytes(tmp, keyLabelKeyID)
	if err != nil {
		return fmt.Errorf("kid: %w", err)
	}
	alg, _, err := decodeInt(tmp, keyLabelAlgorithm)
	if err != nil {
		return fmt.Errorf("alg: %w", err)
	}
	k.Algorithm = Algorithm(alg)
	key_ops, err := decodeSlice(tmp, keyLabelKeyOps)
	if err != nil {
		return fmt.Errorf("key_ops: %w", err)
	}
	if len(key_ops) > 0 {
		k.Ops = make([]KeyOp, len(key_ops))
		for i, op := range key_ops {
			switch op := op.(type) {
			case int64:
				k.Ops[i] = KeyOp(op)
			case string:
				var ok bool
				if k.Ops[i], ok = KeyOpFromString(op); !ok {
					return fmt.Errorf("key_ops: unknown entry value %q", op)
				}
			default:
				return fmt.Errorf("key_ops: invalid entry type %T", op)
			}
		}
	}
	k.BaseIV, _, err = decodeBytes(tmp, keyLabelBaseIV)
	if err != nil {
		return fmt.Errorf("base_iv: %w", err)
	}

	delete(tmp, keyLabelKeyType)
	delete(tmp, keyLabelKeyID)
	delete(tmp, keyLabelAlgorithm)
	delete(tmp, keyLabelKeyOps)
	delete(tmp, keyLabelBaseIV)

	if len(tmp) > 0 {
		k.Params = make(map[any]any, len(tmp))
		for lbl, v := range tmp {
			switch lbl := lbl.(type) {
			case int64:
				if (k.Type == KeyTypeEC2 || k.Type == KeyTypeOKP) &&
					(lbl == KeyLabelEC2Curve || lbl == KeyLabelOKPCurve) {
					v = Curve(v.(int64))
				}
				k.Params[lbl] = v
			case string:
				k.Params[lbl] = v
			default:
				return fmt.Errorf("invalid label type %T", lbl)
			}
		}
	}
	return k.validate(KeyOpReserved)
}

// PublicKey returns a crypto.PublicKey generated using Key's parameters.
func (k *Key) PublicKey() (crypto.PublicKey, error) {
	if err := k.validate(KeyOpVerify); err != nil {
		return nil, err
	}
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

		_, x, y, _ := k.EC2()

		pub := &ecdsa.PublicKey{Curve: curve, X: new(big.Int), Y: new(big.Int)}
		pub.X.SetBytes(x)
		pub.Y.SetBytes(y)

		return pub, nil
	case AlgorithmEdDSA:
		_, x, _ := k.OKP()
		return ed25519.PublicKey(x), nil
	default:
		return nil, ErrAlgorithmNotSupported
	}
}

// PrivateKey returns a crypto.PrivateKey generated using Key's parameters.
// Compressed point is not supported for EC2 keys.
func (k *Key) PrivateKey() (crypto.PrivateKey, error) {
	if err := k.validate(KeyOpSign); err != nil {
		return nil, err
	}
	alg, err := k.deriveAlgorithm()
	if err != nil {
		return nil, err
	}

	switch alg {
	case AlgorithmES256, AlgorithmES384, AlgorithmES512:
		_, x, y, d := k.EC2()
		if len(x) == 0 || len(y) == 0 {
			return nil, fmt.Errorf("%w: compressed point not supported", ErrInvalidPrivKey)
		}

		var curve elliptic.Curve
		switch alg {
		case AlgorithmES256:
			curve = elliptic.P256()
		case AlgorithmES384:
			curve = elliptic.P384()
		case AlgorithmES512:
			curve = elliptic.P521()
		}

		bx := new(big.Int).SetBytes(x)
		by := new(big.Int).SetBytes(y)
		bd := new(big.Int).SetBytes(d)

		return &ecdsa.PrivateKey{
			PublicKey: ecdsa.PublicKey{Curve: curve, X: bx, Y: by},
			D:         bd,
		}, nil
	case AlgorithmEdDSA:
		_, x, d := k.OKP()
		if len(x) == 0 {
			return ed25519.NewKeyFromSeed(d), nil
		}

		buf := make([]byte, ed25519.PrivateKeySize)

		copy(buf, d)
		copy(buf[32:], x)

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
	if k.Algorithm != AlgorithmReserved {
		return k.Algorithm, nil
	}

	return k.deriveAlgorithm()
}

// Signer returns a Signer created using Key.
func (k *Key) Signer() (Signer, error) {
	if !k.canOp(KeyOpSign) {
		return nil, ErrOpNotSupported
	}
	priv, err := k.PrivateKey()
	if err != nil {
		return nil, err
	}

	alg, err := k.AlgorithmOrDefault()
	if err != nil {
		return nil, err
	}

	signer, ok := priv.(crypto.Signer)
	if !ok {
		return nil, ErrInvalidPrivKey
	}

	return NewSigner(alg, signer)
}

// Verifier returns a Verifier created using Key.
func (k *Key) Verifier() (Verifier, error) {
	if !k.canOp(KeyOpVerify) {
		return nil, ErrOpNotSupported
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
	switch k.Type {
	case KeyTypeEC2:
		crv, _, _, _ := k.EC2()
		switch crv {
		case CurveP256:
			return AlgorithmES256, nil
		case CurveP384:
			return AlgorithmES384, nil
		case CurveP521:
			return AlgorithmES512, nil
		default:
			return AlgorithmReserved, fmt.Errorf(
				"unsupported curve %q for key type EC2", crv.String())
		}
	case KeyTypeOKP:
		crv, _, _ := k.OKP()
		switch crv {
		case CurveEd25519:
			return AlgorithmEdDSA, nil
		default:
			return AlgorithmReserved, fmt.Errorf(
				"unsupported curve %q for key type OKP", crv.String())
		}
	default:
		// Symmetric algorithms are not supported in the current inmplementation.
		return AlgorithmReserved, fmt.Errorf("unexpected key type %q", k.Type.String())
	}
}

func algorithmFromEllipticCurve(c elliptic.Curve) Algorithm {
	switch c {
	case elliptic.P256():
		return AlgorithmES256
	case elliptic.P384():
		return AlgorithmES384
	case elliptic.P521():
		return AlgorithmES512
	default:
		return AlgorithmReserved
	}
}

func curveSize(crv Curve) int {
	var bitSize int
	switch crv {
	case CurveP256:
		bitSize = elliptic.P256().Params().BitSize
	case CurveP384:
		bitSize = elliptic.P384().Params().BitSize
	case CurveP521:
		bitSize = elliptic.P521().Params().BitSize
	}
	return (bitSize + 7) / 8
}

func decodeBytes(dic map[any]any, lbl any) (b []byte, ok bool, err error) {
	val, ok := dic[lbl]
	if !ok {
		return nil, false, nil
	}
	if b, ok = val.([]byte); ok {
		return b, true, nil
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("invalid type: expected []uint8, got %T", val)
		}
	}()
	return reflect.ValueOf(val).Bytes(), true, nil
}

func decodeInt(dic map[any]any, lbl any) (int64, bool, error) {
	val, ok := dic[lbl]
	if !ok {
		return 0, false, nil
	}
	if b, ok := val.(int64); ok {
		return b, true, nil
	}
	if v := reflect.ValueOf(val); v.CanInt() {
		return v.Int(), true, nil
	}
	return 0, true, fmt.Errorf("invalid type: expected int64, got %T", val)
}

func decodeUint(dic map[any]any, lbl any) (uint64, bool, error) {
	val, ok := dic[lbl]
	if !ok {
		return 0, false, nil
	}
	if b, ok := val.(uint64); ok {
		return b, true, nil
	}
	v := reflect.ValueOf(val)
	if v.CanUint() {
		return v.Uint(), true, nil
	}
	if v.CanInt() {
		if b := v.Int(); b >= 0 {
			return uint64(b), true, nil
		}
	}
	return 0, true, fmt.Errorf("invalid type: expected uint64, got %T", val)
}

func decodeString(dic map[any]any, lbl any) (string, bool, error) {
	val, ok := dic[lbl]
	if !ok {
		return "", false, nil
	}
	if b, ok := val.(string); ok {
		return b, true, nil
	}
	if v := reflect.ValueOf(val); v.Kind() == reflect.String {
		return v.String(), true, nil
	}
	return "", true, fmt.Errorf("invalid type: expected uint64, got %T", val)
}

func decodeBool(dic map[any]any, lbl any) (bool, bool, error) {
	val, ok := dic[lbl]
	if !ok {
		return false, false, nil
	}
	if b, ok := val.(bool); ok {
		return b, true, nil
	}
	if v := reflect.ValueOf(val); v.Kind() == reflect.Bool {
		return v.Bool(), true, nil
	}
	return false, true, fmt.Errorf("invalid type: expected uint64, got %T", val)
}

func decodeSlice(dic map[any]any, lbl any) ([]any, error) {
	v, ok := dic[lbl]
	if !ok {
		return nil, nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid type: expected []any, got %T", v)
	}
	return arr, nil
}
