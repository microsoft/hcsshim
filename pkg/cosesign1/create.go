package cosesign1

import (
	"crypto/rand"

	"crypto"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log"

	"github.com/veraison/go-cose"
)

// Header indices to match SCITT
// see https://ietf-scitt.github.io/draft-birkholz-scitt-architecture/draft-birkholz-scitt-architecture.html#name-envelope-and-claim-format

const (
	HeaderLabelIssuer int64 = 258
	HeaderLabelFeed   int64 = 259
)

func pem2der(chainPem []byte) []byte {
	block, rest := pem.Decode(chainPem)
	var r []byte = block.Bytes
	for rest != nil && len(rest) != 0 {
		r = append(r, block.Bytes...)
		block, rest = pem.Decode(rest)
	}
	return r
}

func CreateCoseSign1(payloadBlob []byte, issuer string, feed string, contentType string, chainPem []byte, keyPem []byte, saltType string, algo cose.Algorithm, verbose bool) ([]byte, error) {
	var err error

	var remaining []byte
	var result []byte

	var signingKey any
	var keyDer *pem.Block
	keyDer, remaining = pem.Decode(keyPem)
	_ = remaining
	var keyBytes = keyDer.Bytes

	signingKey, err = x509.ParseECPrivateKey(keyBytes)
	if err == nil {
		if verbose {
			log.Printf("parsed EC signing (private) key %q\n", signingKey)
		}
	} else {
		signingKey, err = x509.ParsePKCS8PrivateKey(keyBytes)
		if err == nil {
			if verbose {
				log.Printf("parsed PKCS8 signing (private) key %q\n", signingKey)
			}
		} else {
			signingKey, err = x509.ParsePKCS1PrivateKey(keyBytes)
			if err == nil {
				if verbose {
					log.Printf("parsed PKCS1 signing (private) key %q\n", signingKey)
				}
			} else {
				if verbose {
					log.Print("Error = " + err.Error())
				}
				return result, err
			}
		}
	}

	var chainCerts []*x509.Certificate
	chainDER := pem2der(chainPem)
	chainCerts, err = x509.ParseCertificates(chainDER)
	if err == nil {
		if verbose {
			log.Printf("parsed cert chain for leaf: %v\n", *chainCerts[0])
		}
	} else {
		if verbose {
			log.Print("cert parsing failed - " + err.Error())
		}
		return result, err
	}

	_ = remaining

	var saltReader io.Reader
	if saltType == "rand" {
		saltReader = rand.Reader
	} else {
		saltReader = NewFixedReader(0)
	}

	var cryptoSigner = signingKey.(crypto.Signer)

	var signer cose.Signer
	signer, err = cose.NewSigner(algo, cryptoSigner)
	if err != nil {
		if verbose {
			log.Print("cose.NewSigner err = " + err.Error())
		}
		return result, err
	} else {
		if verbose {
			log.Printf("cose signer %q\n", signer)
		}
	}

	var headers = cose.Headers{
		Protected: cose.ProtectedHeader{
			cose.HeaderLabelAlgorithm:   algo,
			cose.HeaderLabelContentType: contentType,
			cose.HeaderLabelX5Chain:     chainPem,
		},
	}

	// see https://ietf-scitt.github.io/draft-birkholz-scitt-architecture/draft-birkholz-scitt-architecture.html#name-envelope-and-claim-format
	// PRSS will be using string keys for these soon. Meanwhile I'll wrap it all in a json document

	if len(issuer) > 0 {
		headers.Protected[HeaderLabelIssuer] = issuer
	}
	if len(feed) > 0 {
		headers.Protected[HeaderLabelFeed] = feed
	}

	result, err = cose.Sign1(saltReader, signer, headers, payloadBlob, nil)
	if err != nil {
		if verbose {
			log.Print("cose.Sign1 failed\n" + err.Error())
		}
		return result, err
	}

	return result, nil
}
