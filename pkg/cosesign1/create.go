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

func pem2der(chainPem []byte) []byte {
	block, rest := pem.Decode(chainPem)
	var r = []byte{}
	if block.Bytes != nil {
		r = block.Bytes
	}
	for len(rest) != 0 {
		block, rest = pem.Decode(rest)
		if block.Bytes != nil {
			r = append(r, block.Bytes...)
		}
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
	var chainDERArray [][]byte
	for cert := range(chainCerts) {
		chainDERArray = append(chainDERArray, chainCerts[cert].Raw)
	}


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

	// See https://www.iana.org/assignments/cose/cose.xhtml#:~:text=COSE%20Header%20Parameters%20%20%20%20Name%20,algorithm%20to%20use%20%2019%20more%20rows

	var headers = cose.Headers{
		Protected: cose.ProtectedHeader{
			cose.HeaderLabelAlgorithm:   algo,
			cose.HeaderLabelContentType: contentType,
			cose.HeaderLabelX5Chain:     chainDERArray,
		},
	}

	// see https://ietf-scitt.github.io/draft-birkholz-scitt-architecture/draft-birkholz-scitt-architecture.html#name-envelope-and-claim-format
	// Use of strings here to match PRSS COSE Sign1 service

	if len(issuer) > 0 {
		headers.Protected["iss"] = issuer
	}
	if len(feed) > 0 {
		headers.Protected["feed"] = feed
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
