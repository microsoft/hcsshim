package cosesign1

import (
	"crypto/x509"
	"fmt"

	didx509resolver "github.com/Microsoft/didx509go/pkg/did-x509-resolver"

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

	issuer := getStringValue(protected, "iss")
	feed := getStringValue(protected, "feed")
	contenttype := getStringValue(protected, cose.HeaderLabelContentType)

	return &UnpackedCoseSign1{
		Pubcert:     leafCertBase64,
		Feed:        feed,
		Issuer:      issuer,
		Pubkey:      leafPubKeyBase64,
		ChainPem:    chainPEM,
		ContentType: contenttype,
		Payload:     msg.Payload,
		CertChain:   chain,
	}, nil
}
