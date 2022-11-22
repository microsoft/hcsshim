package cosesign1

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log"
	"net/url"
	"strings"

	didx509resolver "github.com/Microsoft/hcsshim/internal/did-x509-resolver"
)

func parsePemChain(chainPem string) ([]*x509.Certificate, error) {
	var chain = []*x509.Certificate{}

	bs := []byte(chainPem)
	for block, rest := pem.Decode(bs); block != nil; block, rest = pem.Decode(rest) {
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return []*x509.Certificate{}, fmt.Errorf("certificate parser failed: %w", err)
			}
			chain = append(chain, cert)
		}
	}

	return chain, nil
}

// The x509 package/module doesn't export these.
// they are derived from https://www.rfc-editor.org/rfc/rfc5280 RFC 5280, 4.2.1.12  Extended Key Usage
// and can never change
var (
	oidExtKeyUsageAny                            = asn1.ObjectIdentifier{2, 5, 29, 37, 0}
	oidExtKeyUsageServerAuth                     = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 1}
	oidExtKeyUsageClientAuth                     = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 2}
	oidExtKeyUsageCodeSigning                    = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 3}
	oidExtKeyUsageEmailProtection                = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 4}
	oidExtKeyUsageIPSECEndSystem                 = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 5}
	oidExtKeyUsageIPSECTunnel                    = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 6}
	oidExtKeyUsageIPSECUser                      = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 7}
	oidExtKeyUsageTimeStamping                   = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 8}
	oidExtKeyUsageOCSPSigning                    = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 9}
	oidExtKeyUsageMicrosoftServerGatedCrypto     = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 10, 3, 3}
	oidExtKeyUsageNetscapeServerGatedCrypto      = asn1.ObjectIdentifier{2, 16, 840, 1, 113730, 4, 1}
	oidExtKeyUsageMicrosoftCommercialCodeSigning = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 2, 1, 22}
	oidExtKeyUsageMicrosoftKernelCodeSigning     = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 61, 1, 1}
)

var extKeyUsageOIDs = []struct {
	extKeyUsage x509.ExtKeyUsage
	oid         asn1.ObjectIdentifier
}{
	{x509.ExtKeyUsageAny, oidExtKeyUsageAny},
	{x509.ExtKeyUsageServerAuth, oidExtKeyUsageServerAuth},
	{x509.ExtKeyUsageClientAuth, oidExtKeyUsageClientAuth},
	{x509.ExtKeyUsageCodeSigning, oidExtKeyUsageCodeSigning},
	{x509.ExtKeyUsageEmailProtection, oidExtKeyUsageEmailProtection},
	{x509.ExtKeyUsageIPSECEndSystem, oidExtKeyUsageIPSECEndSystem},
	{x509.ExtKeyUsageIPSECTunnel, oidExtKeyUsageIPSECTunnel},
	{x509.ExtKeyUsageIPSECUser, oidExtKeyUsageIPSECUser},
	{x509.ExtKeyUsageTimeStamping, oidExtKeyUsageTimeStamping},
	{x509.ExtKeyUsageOCSPSigning, oidExtKeyUsageOCSPSigning},
	{x509.ExtKeyUsageMicrosoftServerGatedCrypto, oidExtKeyUsageMicrosoftServerGatedCrypto},
	{x509.ExtKeyUsageNetscapeServerGatedCrypto, oidExtKeyUsageNetscapeServerGatedCrypto},
	{x509.ExtKeyUsageMicrosoftCommercialCodeSigning, oidExtKeyUsageMicrosoftCommercialCodeSigning},
	{x509.ExtKeyUsageMicrosoftKernelCodeSigning, oidExtKeyUsageMicrosoftKernelCodeSigning},
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

	var policyTokens []string
	didPolicyUpper := strings.ToUpper(didPolicy)
	switch didPolicyUpper {
	case "CN":
		{
			policyTokens = append(policyTokens, "subject", "CN", chain[0].Subject.CommonName)
		}
	case "EKU":
		{
			// Note: In general there may be many predefined and not predefined key usages.
			// We pick the first non-predefined one, or, if there are none, the first predefined one.
			// Others must be specified manually (as in custom, next branch).

			// non-predefined
			if len(chain[0].UnknownExtKeyUsage) > 0 {
				policyTokens = append(policyTokens, "eku", chain[0].UnknownExtKeyUsage[0].String())
			} else if len(chain[0].ExtKeyUsage) > 0 {
				// predefined
				policyTokens = append(policyTokens, "eku", extKeyUsageOIDs[chain[0].ExtKeyUsage[0]].oid.String())
			}
		}
	default:
		{
			// Custom policies
			policyTokens = strings.Split(didPolicy, ":")
		}
	}

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
				policyTokens[i] = url.PathEscape(policyTokens[i])
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
	}

	if verbose {
		log.Println("did:x509 resolved correctly")
	}

	return r, nil
}
