package cosesign1

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log"
	"net/url"
	"strings"

	didx509resolver "github.com/Microsoft/hcsshim/pkg/did-x509-resolver"
)

func parsePemChain(chainPem string) ([]*x509.Certificate, error) {
	var chain = []*x509.Certificate{}

	bs := []byte(chainPem)
	for block, rest := pem.Decode(bs); block != nil; block, rest = pem.Decode(rest) {
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return []*x509.Certificate{}, fmt.Errorf("certificate parser failed: %s", err)
			}
			chain = append(chain, cert)
		}
	}

	return chain, nil
}

func MakeDidX509(fingerprintAlgorithm string, fingerprintIndex int, chainFilename string, didPolicy string, verbose bool) (string, error) {
	if fingerprintAlgorithm != "sha256" {
		return "", fmt.Errorf("unsupported hash algorithm")
	}

	if fingerprintIndex == 0 {
		return "", fmt.Errorf("fingerprint index must be >= 1")
	}

	var chainPEM = string(ReadBlob(chainFilename))
	chain, err := parsePemChain(chainPEM)

	if err != nil {
		return "", err
	}

	if fingerprintIndex > len(chain) {
		return "", fmt.Errorf("signer index out of bounds")
	}

	signerCert := chain[fingerprintIndex]
	var hash = sha256.Sum256(signerCert.Raw)
	fingerprint := base64.RawURLEncoding.EncodeToString(hash[:])

	policyTokens := strings.Split(didPolicy, ":")

	if len(policyTokens) == 0 {
		return "", fmt.Errorf("invalid policy")
	}

	for i := 0; i < len(policyTokens); i++ {
		policyName := policyTokens[i]
		switch policyName {
		case "subject":
			{
				i += 2
				if i >= len(policyTokens) {
					return "", fmt.Errorf("invalid '%s' policy", policyName)
				}
				policyTokens[i] = url.QueryEscape(policyTokens[i])
			}
		default:
			{
			}
		}

	}

	r := "did:x509:0:" + fingerprintAlgorithm + ":" + fingerprint + "::" + strings.Join(policyTokens, ":")

	_, err = didx509resolver.Resolve(chainPEM, r, true)

	if err != nil {
		return "", err
	} else if verbose {
		log.Println("did:x509 resolved correctly")
	}

	return r, nil
}
