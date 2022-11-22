package cosesign1

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"reflect"

	"github.com/veraison/go-cose"
)

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

// helper functions to check that elements in the protected header are
// of the expected types.

// issuer and feed MUST be strings or not present
func isAString(val interface{}) bool {
	_, ok := val.(string)
	return ok
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

	if len(valArray) > 0 {
		_, ok = valArray[0].([]byte)
		return ok
	}

	return false
}

// a DER is an array of bytes
func isDEROnly(val interface{}) bool {
	_, ok := val.([]byte)
	return ok
}

func UnpackAndValidateCOSE1CertChain(raw []byte, optionalPubKeyPEM []byte, optionalRootCAPEM []byte, verbose bool) (*UnpackedCoseSign1, error) {
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

	if verbose {
		log.Printf("algo %d aka %s", algo, algo)
	}

	// The spec says this is ordered - leaf, intermediates, root. X5Bag is unordered and would need sorting
	chainDER, ok := protected[cose.HeaderLabelX5Chain]
	log.Println(reflect.TypeOf(chainDER).String())

	if !ok {
		return nil, fmt.Errorf("x5Chain missing")
	}

	if !isDERChain(chainDER) && !isDEROnly(chainDER) {
		return nil, fmt.Errorf("x5Chain wrong type")
	}

	var issuer string
	val, ok = protected["iss" /* HeaderLabelIssuer */]
	if ok && isAString(val) {
		issuer = val.(string)
	}

	var feed string
	val, ok = protected["feed" /* HeaderLabelFeed */]
	if ok && isAString(val) {
		feed = val.(string)
	}

	// The HeaderLabelX5Chain entry in the cose header may be a blob (single cert) or an array of blobs (a chain) see https://datatracker.ietf.org/doc/draft-ietf-cose-x509/08/
	var chain []*x509.Certificate
	var chainIA []interface{}
	if isDERChain(chainDER) {
		chainIA = chainDER.([]interface{})
	} else {
		chainIA = append(chainIA, chainDER)
	}

	for _, element := range chainIA {
		cert, err := x509.ParseCertificate(element.([]byte))
		chain = append(chain, cert)
		if err != nil {
			if verbose {
				log.Print("Parse certificate failed: " + err.Error())
			}
			return nil, err
		}
	}

	// A reasonable chain will have a handful of certs, typically 3 or 4,
	// so limit to an arbitary 100 which would be quite unreasonable
	chainLen := len(chain)
	if chainLen > 100 || chainLen < 1 {
		return nil, fmt.Errorf("unreasonable number of certs (%d) in COSE_Sign1 document", chainLen)
	}

	// We need to split the certs into root, leaf and intermediate to use x509.Certificate.Verify(opts) below
	rootCerts := x509.NewCertPool()
	intermediateCerts := x509.NewCertPool()
	var leafCert *x509.Certificate
	var rootCert *x509.Certificate

	if verbose {
		log.Print("Certificate chain:")
	}
	// since the certs come from the ordered HeaderLabelX5Chain we can assume chain[0] is the leaf,
	// chain[len-1] is the root, and the rest are intermediates.
	for i, cert := range chain {
		if i == 0 {
			leafCert = cert
			if verbose {
				log.Println(x509ToPEM(cert))
			}
		} else if i == chainLen-1 {
			rootCert = cert
			rootCerts.AddCert(rootCert)
			if verbose {
				log.Println(x509ToPEM(cert))
			}
		} else {
			intermediateCerts.AddCert(cert)
			if verbose {
				log.Println(x509ToPEM(cert))
			}
		}
	}

	opts := x509.VerifyOptions{
		Intermediates: intermediateCerts,
		Roots:         rootCerts,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny}, // To be removed when I have a decent set of examples.
		// consider CurrentTime time.Time set to be the time the cose message was signed so we are checking the certs were valid then rather than now. Maybe not TBD
	}

	/*
		Until we have some production certs allow the certificate check to fail
	*/

	_, err = leafCert.Verify(opts)

	if err != nil {
		return nil, fmt.Errorf("certificate chain verification failed")
	}

	var leafCertBase64 = x509ToBase64(leafCert) // blob of the leaf x509 cert reformatted into pem (base64) style as per the fragment policy rules expect
	var leafPubKey = leafCert.PublicKey
	var leafPubKeyBase64 = keyToBase64(leafPubKey)

	var chainPEM string
	for i, c := range chain {
		if i > 0 {
			chainPEM += "\n"
		}
		chainPEM += x509ToPEM(c)
	}

	var results = UnpackedCoseSign1{
		Pubcert:     leafCertBase64,
		Feed:        feed,
		Issuer:      issuer,
		Pubkey:      leafPubKeyBase64,
		ChainPem:    chainPEM,
		ContentType: msg.Headers.Protected[cose.HeaderLabelContentType].(string),
		Payload:     msg.Payload,
		CertChain:   chain,
	}

	if err != nil {
		if verbose {
			log.Print("leafCert.Verify failed: " + err.Error())
		}
		return nil, err
	}

	// Use the supplied public key or the one we extracted from the leaf cert.
	var keyToCheck any
	if len(optionalPubKeyPEM) == 0 {
		keyToCheck = leafPubKey
	} else {
		var keyDer *pem.Block
		keyDer, _ = pem.Decode(optionalPubKeyPEM) // _ is the remaining. We only care about the first key.
		var keyBytes = keyDer.Bytes

		keyToCheck, err = x509.ParsePKCS1PublicKey(keyBytes)
		if err == nil {
			if verbose {
				log.Printf("parsed as PKCS1 public key %q\n", keyToCheck)
			}
		} else {
			keyToCheck, err = x509.ParsePKIXPublicKey(keyBytes)
			if err == nil {
				if verbose {
					log.Printf("parsed as PKIX key %q\n", keyToCheck)
				}
			} else {
				if verbose {
					log.Print("Failed to parse provided public key - Error = " + err.Error())
				}
				return nil, err
			}
		}
	}

	verifier, err := cose.NewVerifier(algo, keyToCheck)
	if err != nil {
		if verbose {
			log.Printf("cose.NewVerifier failed (algo %d): %s", algo, err.Error())
		}
		return nil, err
	}

	err = msg.Verify(nil, verifier)
	if err != nil {
		if verbose {
			log.Printf("msg.Verify failed: algo = %d err = %s", algo, err.Error())
		}
		return nil, err
	}

	return &results, nil
}

func PrintChain(inputFilename string) error {
	raw := ReadBlob(inputFilename)
	var msg cose.Sign1Message
	err := msg.UnmarshalCBOR(raw)
	if err != nil {
		return err
	}

	protected := msg.Headers.Protected

	chainDER, chainPresent := protected[cose.HeaderLabelX5Chain] // The spec says this is ordered - leaf, intermediates, root. X5Bag is unordered and woould need sorting
	if !chainPresent {
		return fmt.Errorf("x5Chain missing")
	}

	chainIA := chainDER.([]interface{})
	for _, c := range chainIA {
		cert, err := x509.ParseCertificate(c.([]byte))
		if err != nil {
			return err
		}
		fmt.Println(x509ToPEM(cert))
	}

	return nil
}
