package cosesign1

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"

	didx509resolver "github.com/Microsoft/didx509go/pkg/did-x509-resolver"
	"github.com/sirupsen/logrus"
)

func parsePemChain(chainPem string) ([]*x509.Certificate, error) {
	var chain = []*x509.Certificate{}

	bs := []byte(chainPem)
	for block, rest := pem.Decode(bs); block != nil; block, rest = pem.Decode(rest) {
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("certificate parser failed: %w", err)
			}
			chain = append(chain, cert)
		}
	}

	return chain, nil
}

func MakeDidX509(fingerprintAlgorithm string, fingerprintIndex int, chainPEM string, didPolicy string, verbose bool) (string, error) {
	if fingerprintAlgorithm != "sha256" {
		return "", fmt.Errorf("unsupported fingerprint hash algorithm %q", fingerprintAlgorithm)
	}

	if fingerprintIndex < 1 {
		return "", fmt.Errorf("fingerprint index must be >= 1")
	}

	chain, err := parsePemChain(chainPEM)
	if err != nil {
		return "", err
	}

	if len(chain) < 1 {
		return "", fmt.Errorf("chain must not be empty")
	}

	if fingerprintIndex > len(chain)-1 {
		return "", fmt.Errorf("signer index out of bounds")
	}

	signerCert := chain[fingerprintIndex]
	hash := sha256.Sum256(signerCert.Raw)
	fingerprint := base64.RawURLEncoding.EncodeToString(hash[:])

	var policyTokens []string
	didPolicyUpper := strings.ToUpper(didPolicy)
	switch didPolicyUpper {
	case "CN":
		policyTokens = append(policyTokens, "subject", "CN", chain[0].Subject.CommonName)
	case "EKU":
		// Note: In general there may be many predefined and not predefined key usages.
		// We pick the first non-predefined one, or, if there are none, the first predefined one.
		// Others must be specified manually (as in custom, next branch).

		// non-predefined
		if len(chain[0].UnknownExtKeyUsage) > 0 {
			policyTokens = append(policyTokens, "eku", chain[0].UnknownExtKeyUsage[0].String())
		} else if len(chain[0].ExtKeyUsage) > 0 {
			extendedKeyUsage := chain[0].ExtKeyUsage[0]
			keyUsageOid, ok := didx509resolver.OidFromExtKeyUsage(extendedKeyUsage)
			// predefined
			if ok {
				policyTokens = append(policyTokens, "eku", keyUsageOid.String())
			}
		}
	default:
		// Custom policies
		policyTokens = strings.Split(didPolicy, ":")
	}

	if len(policyTokens) == 0 {
		return "", errors.New("invalid policy")
	}

	for i := 0; i < len(policyTokens); i++ {
		policyName := policyTokens[i]
		switch policyName {
		case "subject":
			i += 2
			if i >= len(policyTokens) {
				return "", fmt.Errorf("invalid %q policy", policyName)
			}
			policyTokens[i] = url.PathEscape(policyTokens[i])
		default:
		}
	}

	r := "did:x509:0:" + fingerprintAlgorithm + ":" + fingerprint + "::" + strings.Join(policyTokens, ":")
	_, err = didx509resolver.Resolve(chainPEM, r, true)
	if err != nil {
		return "", err
	}

	if verbose {
		logrus.Debug("did:x509 resolved correctly")
	}

	return r, nil
}
