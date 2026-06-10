package cosesign1

import (
	"bytes"
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"math"

	didx509resolver "github.com/Microsoft/didx509go/pkg/did-x509-resolver"

	"github.com/fxamacker/cbor/v2"
	"github.com/sirupsen/logrus"

	"github.com/veraison/go-cose"
)

// helper functions to check that elements in the protected header are
// of the expected types.

// issuer and feed MUST be strings or not present
func getStringValue(m cose.ProtectedHeader, k any) string {
	val, ok := m[k]
	if !ok {
		return ""
	}
	str, ok := val.(string)
	if !ok {
		return ""
	}
	return str
}

// See https://datatracker.ietf.org/doc/draft-ietf-cose-x509/09/ x5chain section,
// The "chain" can be an array of arrays of bytes or just a single array of bytes
// in the single cert case. Each of these two functions handles of of these cases

// a DER chain is an array of arrays of bytes
func isDERChain(val interface{}) bool {
	valArray, ok := val.([]interface{})
	if !ok {
		return false
	}

	for _, element := range valArray {
		_, ok = element.([]byte)
		if !ok {
			return false
		}
	}

	return len(valArray) > 0
}

// a DER is an array of bytes
func isDEROnly(val interface{}) bool {
	_, ok := val.([]byte)
	return ok
}

type UnpackedCoseSign1 struct {
	Issuer      string
	Feed        string
	ContentType string
	Pubkey      string
	Pubcert     string
	ChainPem    string
	Payload     []byte
	CertChain   []*x509.Certificate
	Protected   cose.ProtectedHeader
	Unprotected cose.UnprotectedHeader
	// Receipts contains the parsed COSE Receipts attached to the unprotected
	// `receipts` header (label 394), if any. Receipts are parsed but not
	// validated; use (ParsedCOSEReceipt).Validate to validate each.
	Receipts []ParsedCOSEReceipt
}

// ParsedCOSEReceipt is a parsed COSE Receipt attached to a COSE Sign1
// envelope. It carries the original CBOR-encoded blob alongside the decoded
// COSE_Sign1 message and a few convenience fields extracted from its
// protected header.
type ParsedCOSEReceipt struct {
	// Raw is the original CBOR-encoded COSE_Sign1 receipt blob.
	Raw []byte
	// Message is the decoded COSE_Sign1 receipt.
	Message cose.Sign1Message
	// Issuer is the value of CWT claim `iss` from the receipt's protected CWT
	// Claims header
	Issuer string
	// The value of the receipt's protected `kid` header, interpreted
	// as a string (CCF uses ASCII hex)
	Kid string
	// The expected hash of the Signed Statement this receipt is for.
	ExpectedDataHash []byte
}

// parseCOSEReceipts decodes the unprotected `receipts` header (label 394)
// into []ParsedCOSEReceipt. It does not validate the receipts.
func parseCOSEReceipts(unprotected cose.UnprotectedHeader) ([]ParsedCOSEReceipt, error) {
	rcptsVal, ok := unprotected[COSE_Header_Receipts]
	if !ok {
		return nil, nil
	}
	rcptsArr, ok := rcptsVal.([]interface{})
	if !ok {
		return nil, fmt.Errorf("receipts header is not an array (got %T)", rcptsVal)
	}
	out := make([]ParsedCOSEReceipt, 0, len(rcptsArr))
	for i, r := range rcptsArr {
		rb, ok := r.([]byte)
		if !ok {
			return nil, fmt.Errorf("receipt %d is not a byte string (got %T)", i, r)
		}
		var msg cose.Sign1Message
		if err := msg.UnmarshalCBOR(rb); err != nil {
			return nil, fmt.Errorf("receipt %d: parsing COSE_Sign1: %w", i, err)
		}
		rcpt := ParsedCOSEReceipt{Raw: rb, Message: msg}
		if kidVal, ok := msg.Headers.Protected[COSE_Header_kid]; ok {
			if kidBytes, ok := kidVal.([]byte); ok {
				rcpt.Kid = string(kidBytes)
			} else {
				return nil, fmt.Errorf("receipt %d: kid is not a byte string (got %T)", i, kidVal)
			}
		} else {
			return nil, fmt.Errorf("receipt %d: kid header missing", i)
		}
		if cwtVal, ok := msg.Headers.Protected[COSE_Header_CWTClaims]; ok {
			if cwt, ok := cwtVal.(map[interface{}]interface{}); ok {
				issVal, issPresent := cwt[CWT_Issuer]
				if !issPresent {
					return nil, fmt.Errorf("receipt %d: issuer (iss) claim missing from CWT claims", i)
				}
				if iss, ok := issVal.(string); ok {
					rcpt.Issuer = iss
				} else {
					return nil, fmt.Errorf("receipt %d: issuer is not a string (got %T)", i, issVal)
				}
			} else {
				return nil, fmt.Errorf("receipt %d: CWT claims is not a map (got %T)", i, cwtVal)
			}
		} else {
			return nil, fmt.Errorf("receipt %d: CWT claims missing", i)
		}
		out = append(out, rcpt)
	}
	return out, nil
}

// This function is rather unpleasant in that it both decodes the COSE Sign1 document and its various
// crypto parts AND checks that those parts are sound in this context. Higher layers may yet refuse the
// payload for reasons beyond the scope of the checking of the document itself.
// While this function could be decomposed into "unpack" and "verify" there would need to be extra state,
// such as the cert pools, stored in some state object. Then the sensible pattern would be to have
// accessors and member functions such as "verity()". However that was done there could exist state objects
// for badly formed COSE Sign1 documents and that would complicate the jobs of callers.
//
// raw: an array of bytes comprising the COSE Sign1 document.
func UnpackAndValidateCOSE1CertChain(raw []byte) (*UnpackedCoseSign1, error) {
	var msg cose.Sign1Message
	err := msg.UnmarshalCBOR(raw)
	if err != nil {
		return nil, err
	}

	protected := msg.Headers.Protected
	val, ok := protected[cose.HeaderLabelAlgorithm]
	if !ok {
		return nil, fmt.Errorf("algorithm missing")
	}

	algo, ok := val.(cose.Algorithm)
	if !ok {
		return nil, fmt.Errorf("algorithm wrong type")
	}

	logrus.Debugf("COSE Sign1 unpack: algorithm %d", algo)

	// The spec says this is ordered - leaf, intermediates, root. X5Bag is unordered and would need sorting
	chainDER, ok := protected[cose.HeaderLabelX5Chain]

	if !ok {
		return nil, fmt.Errorf("x5Chain missing")
	}

	var chainIA []interface{}

	// The HeaderLabelX5Chain entry in the cose header may be a blob (single cert) or an array of blobs (a chain) see https://datatracker.ietf.org/doc/draft-ietf-cose-x509/08/
	if isDERChain(chainDER) {
		chainIA = chainDER.([]interface{})
	} else if isDEROnly(chainDER) {
		chainIA = append(chainIA, chainDER)
	} else {
		return nil, fmt.Errorf("x5Chain wrong type")
	}

	var chain []*x509.Certificate

	for index, element := range chainIA {
		cert, err := x509.ParseCertificate(element.([]byte))
		if err == nil {
			chain = append(chain, cert)
		} else {
			logrus.Debugf("Parse certificate failed on %d: %s", index, err.Error())
			return nil, err
		}
	}

	// A reasonable chain will have a handful of certs, typically 3 or 4,
	// so limit to an arbitary 100 which would be quite unreasonable
	chainLen := len(chain)
	if chainLen > 100 || chainLen < 1 {
		return nil, fmt.Errorf("unreasonable number of certs (%d) in COSE_Sign1 document", chainLen)
	}

	var leafCert = chain[0]
	var leafCertBase64 = x509ToBase64(leafCert)
	var leafPubKey = leafCert.PublicKey
	var leafPubKeyBase64 = keyToBase64(leafPubKey)

	var chainPEM string
	for i, c := range chain {
		if i > 0 {
			chainPEM += "\n"
		}
		chainPEM += convertx509ToPEM(c)
	}

	logrus.Debugln("Certificate chain:")
	logrus.Debugln(chainPEM)

	// First check that the chain is itself good.
	// Note that a single cert would fail here as it
	// falls back to the system root certs which will
	// never have issued the leaf cert.

	if len(chain) > 1 {
		// Any valid cert chain is good for us as we will be matching
		// part of the chain with a customer provided cert fingerprint.
		_, err = didx509resolver.VerifyCertificateChain(chain, chain[len(chain)-1:], false)

		if err != nil {
			return nil, fmt.Errorf("certificate chain verification failed - %w", err)
		}
	}

	// Next check that the signature over the document was made with the private key matching the
	// public key we extracted from the leaf cert.

	verifier, err := cose.NewVerifier(algo, leafPubKey)
	if err != nil {
		logrus.Debugf("cose.NewVerifier failed (algo %d): %s", algo, err.Error())
		return nil, err
	}

	err = msg.Verify(nil, verifier)
	if err != nil {
		logrus.Debugf("msg.Verify failed: algo = %d err = %s", algo, err.Error())
		return nil, err
	}

	cwt, hasCwt := protected[COSE_Header_CWTClaims]
	var issuer, feed string
	if hasCwt {
		cwt, ok := cwt.(map[interface{}]interface{})
		if !ok {
			return nil, fmt.Errorf("expected CWTClaims header to be a map[any]any, got %T", cwt)
		}
		issuer = getStringValue(cwt, CWT_Issuer)
		feed = getStringValue(cwt, CWT_Subject)
	} else {
		issuer = getStringValue(protected, "iss")
		feed = getStringValue(protected, "feed")
	}

	contenttype := getStringValue(protected, cose.HeaderLabelContentType)

	receipts, err := parseCOSEReceipts(msg.Headers.Unprotected)
	if err != nil {
		return nil, fmt.Errorf("parsing receipts: %w", err)
	}
	if len(receipts) > 0 {
		dataHash, err := computeSignedStatementDataHash(raw)
		if err != nil {
			return nil, fmt.Errorf("computing signed statement data hash: %w", err)
		}
		for i := range receipts {
			receipts[i].ExpectedDataHash = dataHash
		}
	}

	return &UnpackedCoseSign1{
		Pubcert:     leafCertBase64,
		Feed:        feed,
		Issuer:      issuer,
		Pubkey:      leafPubKeyBase64,
		ChainPem:    chainPEM,
		ContentType: contenttype,
		Payload:     msg.Payload,
		CertChain:   chain,
		Protected:   protected,
		Unprotected: msg.Headers.Unprotected,
		Receipts:    receipts,
	}, nil
}

// asInt64 coerces a CBOR-decoded integer value (which may be returned as
// int64, uint64 or int by different decoders) to an int64.
func asInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case uint64:
		if n > math.MaxInt64 {
			logrus.Errorf("Unable to convert %v to int64 due to overflow", n)
			return 0, false
		}
		return int64(n), true
	case uint:
		// uint is 64bit on 64bit platforms, so can overflow int64
		if n > math.MaxInt64 {
			logrus.Errorf("Unable to convert %v to int64 due to overflow", n)
			return 0, false
		}
		return int64(n), true
	}
	return 0, false
}

// Validate validates the COSE Receipt's structure and signature.  See
// https://www.ietf.org/archive/id/draft-ietf-cose-merkle-tree-proofs-18.html
// for details about COSE Receipts.
//
// It checks that:
//   - the protected header carries a vds (label 395),
//   - the payload is detached,
//   - the unprotected `vdp` header (label 396) contains at least one
//     inclusion proof (key -1) encoded as a byte string,
//   - the Merkle root recomputed from each inclusion proof verifies the
//     receipt's COSE_Sign1 signature, using the public key in `keys` indexed by
//     r.Kid.
//   - The data-hash in the receipt matches the expected hash of the signed
//     statement it is for.
func (r ParsedCOSEReceipt) Validate(keys map[string]crypto.PublicKey) error {
	msg := r.Message

	vdsVal, ok := msg.Headers.Protected[COSE_Header_vds]
	if !ok {
		return fmt.Errorf("missing vds (label %d) in protected header", COSE_Header_vds)
	}
	vds, ok := asInt64(vdsVal)
	if !ok {
		return fmt.Errorf("vds has wrong type: %T", vdsVal)
	}

	if msg.Payload != nil {
		return fmt.Errorf("payload must be detached but has %d bytes", len(msg.Payload))
	}

	algoVal, ok := msg.Headers.Protected[cose.HeaderLabelAlgorithm]
	if !ok {
		return fmt.Errorf("missing algorithm in protected header")
	}
	algo, ok := algoVal.(cose.Algorithm)
	if !ok {
		return fmt.Errorf("algorithm has wrong type: %T", algoVal)
	}

	pubKey, ok := keys[r.Kid]
	if !ok {
		return fmt.Errorf("no key for kid %s", r.Kid)
	}

	vdpVal, ok := msg.Headers.Unprotected[COSE_Header_vdp]
	if !ok {
		return fmt.Errorf("missing vdp (label %d) in unprotected header", COSE_Header_vdp)
	}
	vdpMap, ok := vdpVal.(map[interface{}]interface{})
	if !ok {
		return fmt.Errorf("vdp has wrong type: %T", vdpVal)
	}
	inclVal, ok := vdpMap[COSE_ProofInclusion]
	if !ok {
		return fmt.Errorf("no inclusion proofs (key %d) in vdp", COSE_ProofInclusion)
	}
	inclArr, ok := inclVal.([]interface{})
	if !ok {
		return fmt.Errorf("inclusion proofs has wrong type: %T", inclVal)
	}
	if len(inclArr) == 0 {
		return fmt.Errorf("inclusion proofs array is empty")
	}

	verifier, err := cose.NewVerifier(algo, pubKey)
	if err != nil {
		return fmt.Errorf("cose.NewVerifier (algo %d): %w", algo, err)
	}

	for i, p := range inclArr {
		pb, ok := p.([]byte)
		if !ok {
			return fmt.Errorf("inclusion proof %d is not a byte string (got %T)", i, p)
		}
		var root, dataHash []byte
		switch vds {
		case COSE_vds_CCF_LEDGER_SHA256:
			root, dataHash, err = CCF_ComputeRoot(pb)
		default:
			return fmt.Errorf("only receipts with CCF profile supported (got vds %d)", vds)
		}
		if err != nil {
			return fmt.Errorf("inclusion proof %d: %w", i, err)
		}
		if !bytes.Equal(dataHash, r.ExpectedDataHash) {
			return fmt.Errorf("inclusion proof %d: leaf data-hash %x does not match the expected value %x for the signed envelope", i, dataHash, r.ExpectedDataHash)
		}
		logrus.Debugf("receipt inclusion proof %d recomputed root: %x", i, root)
		// Verify the receipt's COSE_Sign1 signature using the recomputed
		// Merkle root as the detached payload.
		msg.Payload = root
		if err := msg.Verify(nil, verifier); err != nil {
			return fmt.Errorf("inclusion proof %d: signature verification failed (recomputed root=%x, kid=%s, alg=%d): %w", i, root, r.Kid, algo, err)
		}
		msg.Payload = nil
	}
	return nil
}

// Decodes a CCF inclusion proof (the bstr-wrapped CBOR `ccf-inclusion-proof`
// structure) and recomputes the Merkle root using the algorithm described in
// section 3.2 of
// https://datatracker.ietf.org/doc/html/draft-ietf-scitt-receipts-ccf-profile-02
// Returns the recomputed Merkle root and the data-hash from the leaf (this
// needs to be verified by the caller against an expected value).
func CCF_ComputeRoot(proofBytes []byte) ([]byte, []byte, error) {
	var proof map[int64]interface{}
	if err := cbor.Unmarshal(proofBytes, &proof); err != nil {
		return nil, nil, fmt.Errorf("decoding inclusion proof: %w", err)
	}
	// ccf-inclusion-proof = bstr .cbor {
	//   &(leaf: 1) => ccf-leaf
	//   &(path: 2) => [+ ccf-proof-element]
	// }
	leafVal, ok := proof[1]
	if !ok {
		return nil, nil, fmt.Errorf("missing leaf (key 1)")
	}
	pathVal, ok := proof[2]
	if !ok {
		return nil, nil, fmt.Errorf("missing path (key 2)")
	}

	// ccf-leaf = [
	//   ; Byte string of size HASH_SIZE(32)
	//   internal-transaction-hash: bstr .size 32
	//
	//   ; Text string of at most 1024 bytes
	//   internal-evidence: tstr .size (1..1024)
	//
	//   ; Byte string of size HASH_SIZE(32)
	//   data-hash: bstr .size 32
	// ]
	leafArr, ok := leafVal.([]interface{})
	if !ok || len(leafArr) != 3 {
		return nil, nil, fmt.Errorf("leaf must be a 3-element array, got %T len %d", leafVal, lenOf(leafVal))
	}
	internalTxHash, ok := leafArr[0].([]byte)
	if !ok || len(internalTxHash) != 32 {
		return nil, nil, fmt.Errorf("leaf.internal-transaction-hash must be a 32-byte bstr, got %T", leafArr[0])
	}
	internalEvidenceStr, ok := leafArr[1].(string)
	if !ok {
		return nil, nil, fmt.Errorf("leaf.internal-evidence must be a text tstr, got %T", leafArr[1])
	}
	internalEvidence := []byte(internalEvidenceStr)
	if len(internalEvidence) < 1 || len(internalEvidence) > 1024 {
		return nil, nil, fmt.Errorf("leaf.internal-evidence has invalid length %d", len(internalEvidence))
	}
	dataHash, ok := leafArr[2].([]byte)
	if !ok || len(dataHash) != 32 {
		return nil, nil, fmt.Errorf("leaf.data-hash must be a 32-byte bstr, got %T", leafArr[2])
	}

	// Leaf hash:
	//   h := HASH(internal-transaction-hash || HASH(internal-evidence) || data-hash)
	evidenceHash := sha256.Sum256(internalEvidence)
	leafConcat := make([]byte, 0, 32+32+32)
	leafConcat = append(leafConcat, internalTxHash...)
	leafConcat = append(leafConcat, evidenceHash[:]...)
	leafConcat = append(leafConcat, dataHash...)
	leafHash := sha256.Sum256(leafConcat)
	h := leafHash[:]
	logrus.Debugf("CCF leaf: internal-tx-hash=%x evidence=%q (hash=%x) data-hash=%x -> leaf=%x", internalTxHash, internalEvidence, evidenceHash[:], dataHash, h)

	pathArr, ok := pathVal.([]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("path must be an array")
	}
	if len(pathArr) == 0 {
		return nil, nil, fmt.Errorf("path must contain at least one element")
	}

	for i, el := range pathArr {
		// ccf-proof-element = [
		//   ; Position of the element
		//   left: bool
		//
		//   ; Hash of the proof element: byte string of size HASH_SIZE(32)
		//   hash: bstr .size 32
		// ]
		elArr, ok := el.([]interface{})
		if !ok || len(elArr) != 2 {
			return nil, nil, fmt.Errorf("path element %d must be a 2-element array", i)
		}
		left, ok := elArr[0].(bool)
		if !ok {
			return nil, nil, fmt.Errorf("path element %d left flag must be a bool", i)
		}
		hash, ok := elArr[1].([]byte)
		if !ok {
			return nil, nil, fmt.Errorf("path element %d hash must be a 32-byte bstr, got %T", i, elArr[1])
		}
		if len(hash) != 32 {
			return nil, nil, fmt.Errorf("path element %d hash must be 32 bytes, got %d bytes", i, len(hash))
		}
		var concat []byte
		if left {
			concat = append(concat, hash...)
			concat = append(concat, h...)
		} else {
			concat = append(concat, h...)
			concat = append(concat, hash...)
		}
		sum := sha256.Sum256(concat)
		h = sum[:]
		logrus.Debugf("CCF path step %d: left=%v sibling=%x -> h=%x", i, left, hash, h)
	}
	return h, dataHash, nil
}

// computeSignedStatementDataHash returns sha256 of the tagged COSE_Sign1
// envelope with its unprotected header reset to an empty map. This should match
// the data-hash in the CCF receipt.
//
// This is the hash of the Signed Statement as defined by
// https://datatracker.ietf.org/doc/html/draft-ietf-scitt-architecture-22
func computeSignedStatementDataHash(envelope []byte) ([]byte, error) {
	var arr struct {
		_         struct{} `cbor:",toarray"`
		Protected cbor.RawMessage
		Unprot    map[interface{}]interface{}
		Payload   cbor.RawMessage
		Signature cbor.RawMessage
	}
	if err := cbor.Unmarshal(envelope, &arr); err != nil {
		return nil, fmt.Errorf("decoding COSE_Sign1: %w", err)
	}
	arr.Unprot = map[interface{}]interface{}{}
	em, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return nil, err
	}
	body, err := em.Marshal(arr)
	if err != nil {
		return nil, fmt.Errorf("encoding stripped COSE_Sign1: %w", err)
	}
	tagged, err := em.Marshal(cbor.Tag{Number: COSE_Sign1_Tag, Content: cbor.RawMessage(body)})
	if err != nil {
		return nil, fmt.Errorf("tagging COSE_Sign1: %w", err)
	}
	digest := sha256.Sum256(tagged)
	return digest[:], nil
}

func lenOf(v interface{}) int {
	if a, ok := v.([]interface{}); ok {
		return len(a)
	}
	return -1
}
