package cosesign1

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"

	"github.com/veraison/go-cose"
)

type UnpackedCoseSign1 struct {
	Issuer      string
	Feed        string
	ContentType string
	Pubkey      string
	Pubcert     string
	Payload     []byte
	CertChain   []*x509.Certificate
}

func UnpackAndValidateCOSE1CertChain(raw []byte, optionaPubKeyPEM []byte, optionalRootCAPEM []byte, requireKNownAuthority bool, verbose bool) (UnpackedCoseSign1, error) {
	var msg cose.Sign1Message
	err := msg.UnmarshalCBOR(raw)
	if err != nil {
		return UnpackedCoseSign1{}, err
	}

	protected := msg.Headers.Protected
	algo := protected[cose.HeaderLabelAlgorithm]

	if verbose {
		log.Printf("algo %d aka %s", algo.(cose.Algorithm), algo.(cose.Algorithm))
	}

	chainPEM, chainPresent := protected[cose.HeaderLabelX5Chain] // The spec says this is ordered - leaf, intermediates, root. X5Bag is unordered and woould need sorting
	if !chainPresent {
		return UnpackedCoseSign1{}, fmt.Errorf("x5Chain missing")
	}

	var issuer string
	val, valPresent := protected[HeaderLabelIssuer]
	if valPresent {
		issuer = val.(string)
	}

	var feed string
	val, valPresent = protected[HeaderLabelFeed]
	if valPresent {
		feed = val.(string)
	}

	// The HeaderLabelX5Chain entry in the cose header may be a blob (single cert) or an array of blobs (a chain) see https://datatracker.ietf.org/doc/draft-ietf-cose-x509/08/

	chainDER := pem2der(chainPEM.([]byte))
	chain, err := x509.ParseCertificates(chainDER)
	if err != nil {
		if verbose {
			log.Print("Parse certificate failed: " + err.Error())
		}
		return UnpackedCoseSign1{}, err
	}


	// A reasonable chain will have 2-100 elements
	chainLen := len(chain)
	if chainLen > 100 || chainLen < 1 {
		return UnpackedCoseSign1{}, fmt.Errorf("unreasonable number of certs (%d) in COSE_Sign1 document", chainLen)
	}

	// We need to split the certs into root, leaf and intermediate to use x509.Certificate.Verify(opts) below

	rootCerts := x509.NewCertPool()
	intermediateCerts := x509.NewCertPool()
	var leafCert *x509.Certificate // x509 leaf cert
	var rootCert *x509.Certificate // x509 root cert

	for which, cert := range chain {
		if which == 0 {
			leafCert = cert
		} else if which == chainLen-1 {
			// is this the root cert? (NOTE may be absent as per https://microsoft.sharepoint.com/teams/prss/Codesign/SitePages/COSESignOperationsReference.aspx TBC)
			// cwinter: I think intermediates may be absent, but the root should always be present.
			rootCert = cert
			rootCerts.AddCert(rootCert)
		} else {
			intermediateCerts.AddCert(cert)
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

	var results = UnpackedCoseSign1{
		Pubcert:     leafCertBase64,
		Feed:        feed,
		Issuer:      issuer,
		Pubkey:      leafPubKeyBase64,
		ContentType: msg.Headers.Protected[cose.HeaderLabelContentType].(string),
		Payload:     msg.Payload,
		CertChain:   chain,
	}

	if err != nil {
		if verbose {
			log.Print("leafCert.Verify failed: " + err.Error())
		}
		// self signed gives "x509: certificate signed by unknown authority"
		if requireKNownAuthority {
			return results, err
		}
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

	verifier, err := cose.NewVerifier(algo.(cose.Algorithm), keyToCheck)
	if err != nil {
		if verbose {
			log.Printf("cose.NewVerifier failed (algo %s): %s", algo.(cose.Algorithm), err.Error())
		}
		return results, err
	}

	err = msg.Verify(nil, verifier)
	if err != nil {
		if verbose {
			log.Printf("msg.Verify failed: algo = %s err = %s", algo.(cose.Algorithm), err.Error())
		}
		return results, err
	}

	return results, err
}
