package cosesign1

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"reflect"

	"github.com/veraison/go-cose"
)

// object to convey results to the caller.

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
	return reflect.TypeOf(val).String() == "string"
}

// a cose algorithm is really an int64
func isAnAlgorithm(val interface{}) bool {
	var algo cose.Algorithm
	return reflect.TypeOf(val) == reflect.TypeOf(algo)
}

// See https://datatracker.ietf.org/doc/draft-ietf-cose-x509/09/ x5chain section,
// The "chain" can be an array of arrays of bytes or just a single array of bytes
// in the single cert case.

// a DER chain is an array of arrays of bytes
func isADERChain(val interface{}) bool {
	var derArray []interface{}
	var der []byte
	if val == nil || reflect.TypeOf(val) != reflect.TypeOf(derArray) {
		return false
	}

	var valArray = val.([]interface{})
	if len(valArray) < 1 {
		return false
	}

	return reflect.TypeOf(valArray[0]) == reflect.TypeOf(der)
}

// a DER is an array of bytes
func isADEROnly(val interface{}) bool {
	var der []byte
	if val == nil || reflect.TypeOf(val) != reflect.TypeOf(der) {
		return false
	}

	return true
}

func UnpackAndValidateCOSE1CertChain(raw []byte, optionaPubKeyPEM []byte, optionalRootCAPEM []byte, verbose bool) (UnpackedCoseSign1, error) {
	var msg cose.Sign1Message
	err := msg.UnmarshalCBOR(raw)
	if err != nil {
		return UnpackedCoseSign1{}, err
	}

	protected := msg.Headers.Protected
	val, valPresent := protected[cose.HeaderLabelAlgorithm]
	if !valPresent || !isAnAlgorithm(val) {
		return UnpackedCoseSign1{}, fmt.Errorf("algorithm missing")
	}

	algo := val.(cose.Algorithm)

	if verbose {
		log.Printf("algo %d aka %s", algo, algo)
	}

	chainDER, chainPresent := protected[cose.HeaderLabelX5Chain] // The spec says this is ordered - leaf, intermediates, root. X5Bag is unordered and woould need sorting
	log.Println(reflect.TypeOf(chainDER).String())

	if !chainPresent {
		return UnpackedCoseSign1{}, fmt.Errorf("x5Chain missing")
	}

	if !isADERChain(chainDER) && !isADEROnly(chainDER) {
		return UnpackedCoseSign1{}, fmt.Errorf("x5Chain wrong type")
	}

	var issuer string
	val, valPresent = protected["iss" /* HeaderLabelIssuer */]
	if valPresent && isAString(val) {
		issuer = val.(string)
	}

	var feed string
	val, valPresent = protected["feed" /* HeaderLabelFeed */]
	if valPresent && isAString(val) {
		feed = val.(string)
	}

	// The HeaderLabelX5Chain entry in the cose header may be a blob (single cert) or an array of blobs (a chain) see https://datatracker.ietf.org/doc/draft-ietf-cose-x509/08/

	var chain []*x509.Certificate
	var chainIA []interface{}
	if isADERChain(chainDER) {
		chainIA = chainDER.([]interface{})
	} else {
		chainIA = append(chainIA, chainDER)
	}

	for i := range chainIA {
		cert, err := x509.ParseCertificate(chainIA[i].([]byte))
		chain = append(chain, cert)
		if err != nil {
			if verbose {
				log.Print("Parse certificate failed: " + err.Error())
			}
			return UnpackedCoseSign1{}, err
		}
	}
	

	// A reasonable chain will have 1-100 elements
	chainLen := len(chain)
	if chainLen > 100 || chainLen < 1 {
		return UnpackedCoseSign1{}, fmt.Errorf("unreasonable number of certs (%d) in COSE_Sign1 document", chainLen)
	}

	// We need to split the certs into root, leaf and intermediate to use x509.Certificate.Verify(opts) below

	rootCerts := x509.NewCertPool()
	intermediateCerts := x509.NewCertPool()
	var leafCert *x509.Certificate // x509 leaf cert
	var rootCert *x509.Certificate // x509 root cert

	if verbose {
		log.Print("Certificate chain:")
	}
	for which, cert := range chain {
		if which == 0 {
			leafCert = cert
			if verbose {
				log.Println(x509ToPEM(cert))
			}
		} else if which == chainLen-1 {
			// is this the root cert? (NOTE may be absent as per https://microsoft.sharepoint.com/teams/prss/Codesign/SitePages/COSESignOperationsReference.aspx TBC)
			// cwinter: I think intermediates may be absent, but the root should always be present.
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
		return UnpackedCoseSign1{}, fmt.Errorf("certificate chain verification failed")
	}

	var leafCertBase64 = x509ToBase64(leafCert) // blob of the leaf x509 cert reformatted into pem (base64) style as per the fragment policy rules expect
	var leafPubKey = leafCert.PublicKey
	var leafPubKeyBase64 = keyToBase64(leafPubKey)

	var chainPEM string
	for i := range chain {
		chainPEM += x509ToPEM(chain[i])
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
		return results, err
	}

	// Use the supplied public key or the one we extracted from the leaf cert.
	var keyToCheck any
	if len(optionaPubKeyPEM) == 0 {
		keyToCheck = leafPubKey
	} else {
		var keyDer *pem.Block
		keyDer, _ = pem.Decode(optionaPubKeyPEM)
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
				return results, err
			}
		}
	}

	verifier, err := cose.NewVerifier(algo, keyToCheck)
	if err != nil {
		if verbose {
			log.Printf("cose.NewVerifier failed (algo %d): %s", algo, err.Error())
		}
		return results, err
	}

	err = msg.Verify(nil, verifier)
	if err != nil {
		if verbose {
			log.Printf("msg.Verify failed: algo = %d err = %s", algo, err.Error())
		}
		return results, err
	}

	return results, err
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
	for i := range chainIA {
		cert, err := x509.ParseCertificate(chainIA[i].([]byte))
		if err != nil {
			return err
		}
		fmt.Println(x509ToPEM(cert))
	}

	return nil
}
