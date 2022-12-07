package didx509resolver

import (
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/lestrrat-go/jwx/jwk"
)

func VerifyCertificateChain(chain []*x509.Certificate, trustedRoots []*x509.Certificate, ignoreTime bool) ([][]*x509.Certificate, error) {
	roots := x509.NewCertPool()
	for _, cert := range trustedRoots {
		roots.AddCert(cert)
	}

	intermediates := x509.NewCertPool()

	for _, c := range chain[1 : len(chain)-1] {
		intermediates.AddCert(c)
	}

	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		CurrentTime:   chain[0].NotAfter, // TODO: Figure out how to disable expiry checks?
	}

	return chain[0].Verify(opts)
}

func checkFingerprint(chain []*x509.Certificate, caFingerprintAlg, caFingerprint string) error {

	expectedCaFingerprints := make(map[string]struct{})
	for _, cert := range chain[1:] {
		var n struct{}
		var hash []byte
		switch caFingerprintAlg {
		case "sha256":
			x := sha256.Sum256(cert.Raw)
			hash = x[:]
		case "sha384":
			x := sha512.Sum384(cert.Raw)
			hash = x[:]
		case "sha512":
			x := sha512.Sum512(cert.Raw)
			hash = x[:]
		default:
			return errors.New("unsupported hash algorithm")
		}
		hashStr := base64.RawURLEncoding.EncodeToString(hash)
		expectedCaFingerprints[hashStr] = n
	}

	if _, found := expectedCaFingerprints[caFingerprint]; !found {
		return errors.New("unexpected certificate fingerprint")
	}

	return nil
}

func oidFromString(s string) (*asn1.ObjectIdentifier, error) {
	tokens := strings.Split(s, ".")
	var ints []int
	for _, x := range tokens {
		i, err := strconv.Atoi(x)
		if err != nil {
			return nil, errors.New("invalid OID")
		}
		ints = append(ints, i)
	}
	result := asn1.ObjectIdentifier(ints)
	return &result, nil
}

func checkHasSan(sanType string, value string, cert *x509.Certificate) error {
	switch sanType {
	case "dns":
		for _, name := range cert.DNSNames {
			if name == value {
				return nil
			}
		}
	case "email":
		for _, email := range cert.EmailAddresses {
			if email == value {
				return nil
			}
		}
	case "ipaddress":
		for _, ip := range cert.IPAddresses {
			if ip.String() == value {
				return nil
			}
		}
	case "uri":
		for _, uri := range cert.URIs {
			if uri.String() == value {
				return nil
			}
		}
	default:
		return fmt.Errorf("unknown SAN type: %s", sanType)
	}
	return fmt.Errorf("SAN not found: %s", value)
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

func OidFromExtKeyUsage(eku x509.ExtKeyUsage) (oid asn1.ObjectIdentifier, ok bool) {
	for _, pair := range extKeyUsageOIDs {
		if eku == pair.extKeyUsage {
			return pair.oid, true
		}
	}
	return
}

// did:x509 examples
//
//      did:x509:0:sha256:WE4P5dd8DnLHSkyHaIjhp4udlkF9LqoKwCvu9gl38jk::subject:C:US:ST:California:L:San%20Francisco:O:GitHub%2C%20Inc.
//      did:x509:0:sha256:I5ni_nuWegx4NiLaeGabiz36bDUhDDiHEFl8HXMA_4o::subject:CN:Test%20Leaf%20%28DO%20NOT%20TRUST%20
//
// see https://github.com/microsoft/did-x509/blob/main/specification.md and https://www.w3.org/TR/2022/REC-did-core-20220719/

// check that the did "did" matches the public cert chain "chain"
func verifyDid(chain []*x509.Certificate, did string) error {
	var topTokens = strings.Split(did, "::")

	if len(topTokens) <= 1 {
		return errors.New("invalid DID string")
	}

	var pretokens = strings.Split(topTokens[0], ":")

	if len(pretokens) < 5 || pretokens[0] != "did" || pretokens[1] != "x509" {
		return errors.New("unsupported method/prefix")
	}

	if pretokens[2] != "0" {
		return errors.New("unsupported did:x509 version")
	}

	caFingerprintAlg := pretokens[3]
	caFingerprint := pretokens[4]

	policies := topTokens[1:]

	if len(chain) < 2 {
		return errors.New("certificate chain too short")
	}

	err := checkFingerprint(chain, caFingerprintAlg, caFingerprint)
	if err != nil {
		return err
	}

	for _, policy := range policies {
		parts := strings.Split(policy, ":")

		if len(parts) < 2 {
			return errors.New("invalid policy")
		}

		policyName, args := parts[0], parts[1:]
		switch policyName {
		case "subject":
			if len(args) == 0 || len(args)%2 != 0 {
				return errors.New("key-value pairs required")
			}

			if len(args) < 2 {
				return errors.New("at least one key-value pair is required")
			}

			// Walk the x509 subject description (a list of key:value pairs like "CN:ContainerPlat" saying the subject common
			// name is ContainerPlat) extracting the various fields and checking they do not occur more than once.
			var seenFields []string
			for i := 0; i < len(args); i += 2 {
				k := strings.ToUpper(args[i])
				v, err := url.QueryUnescape(args[i+1])

				if err != nil {
					return fmt.Errorf("urlUnescape failed: %w", err)
				}

				for _, sk := range seenFields {
					if sk == k {
						return fmt.Errorf("duplicate field '%s'", k)
					}
				}
				seenFields = append(seenFields, k)

				leaf := chain[0]
				var fieldValues []string
				switch k {
				case "C":
					fieldValues = leaf.Subject.Country
				case "O":
					fieldValues = leaf.Subject.Organization
				case "OU":
					fieldValues = leaf.Subject.OrganizationalUnit
				case "L":
					fieldValues = leaf.Subject.Locality
				case "S":
					fieldValues = leaf.Subject.Province
				case "STREET":
					fieldValues = leaf.Subject.StreetAddress
				case "POSTALCODE":
					fieldValues = leaf.Subject.PostalCode
				case "SERIALNUMBER":
					fieldValues = []string{leaf.Subject.SerialNumber}
				case "CN":
					fieldValues = []string{leaf.Subject.CommonName}
				default:
					for _, aav := range leaf.Subject.Names {
						if aav.Type.String() == k {
							fieldValues = []string{aav.Value.(string)}
							break
						}
					}
					if len(fieldValues) == 0 {
						return fmt.Errorf("unsupported subject key: %s", k)
					}
				}
				found := false
				for _, fv := range fieldValues {
					if fv == v {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("invalid subject value: %s=%s", k, v)
				}
			}
		case "san":
			if len(args) != 2 {
				return fmt.Errorf("exactly one SAN type and value required")
			}
			sanType := args[0]
			sanValue, err := url.QueryUnescape(args[1])
			if err != nil {
				return fmt.Errorf("url.QueryUnescape failed: %w", err)
			}
			err = checkHasSan(sanType, sanValue, chain[0])
			if err != nil {
				return err
			}
		case "eku":
			if len(args) != 1 {
				return errors.New("exactly one EKU required")
			}

			ekuOid, err := oidFromString(args[0])
			if err != nil {
				return fmt.Errorf("oidFromString failed: %w", err)
			}

			if len(chain[0].UnknownExtKeyUsage) == 0 {
				return errors.New("no EKU extension in certificate")
			}

			foundEku := false
			for _, certEku := range chain[0].ExtKeyUsage {
				certEkuOid, ok := OidFromExtKeyUsage(certEku)
				if ok && certEkuOid.Equal(*ekuOid) {
					foundEku = true
					break
				}
			}
			for _, certEkuOid := range chain[0].UnknownExtKeyUsage {
				if certEkuOid.Equal(*ekuOid) {
					foundEku = true
					break
				}
			}

			if !foundEku {
				return fmt.Errorf("EKU not found: %s", ekuOid)
			}
		case "fulcio-issuer":
			if len(args) != 1 {
				return errors.New("excessive arguments to fulcio-issuer")
			}
			decodedArg, err := url.QueryUnescape(args[0])
			if err != nil {
				return fmt.Errorf("urlUnescape failed: %w", err)
			}
			fulcioIssuer := "https://" + decodedArg
			fulcioIssuerOid, err := oidFromString("1.3.6.1.4.1.57264.1.1")
			if err != nil {
				return fmt.Errorf("oidFromString failed: %w", err)
			}
			found := false
			for _, ext := range chain[0].Extensions {
				if ext.Id.Equal(*fulcioIssuerOid) {
					if string(ext.Value) == fulcioIssuer {
						found = true
						break
					}
				}
			}
			if !found {
				return fmt.Errorf("invalid fulcio-issuer: %s", fulcioIssuer)
			}
		default:
			return fmt.Errorf("unsupported did:x509 policy name '%s'", policyName)
		}
	}

	return nil
}

func createDidDocument(did string, chain []*x509.Certificate) (string, error) {
	format := `
{
	"@context": "https://www.w3.org/ns/did/v1",
	"id": "%s",
	"verificationMethod": [{
		"id": "%[1]s#key-1",
		"type": "JsonWebKey2020",
		"controller": "%[1]s",
		"publicKeyJwk": %s,
	}]
	%s
	%s
}`

	includeAssertionMethod := chain[0].KeyUsage == 0 || (chain[0].KeyUsage&x509.KeyUsageDigitalSignature) != 0
	includeKeyAgreement := chain[0].KeyUsage == 0 || (chain[0].KeyUsage&x509.KeyUsageKeyAgreement) != 0

	if !includeAssertionMethod && !includeKeyAgreement {
		return "", errors.New("leaf certificate key usage must include digital signature or key agreement")
	}

	am := ""
	ka := ""
	if includeAssertionMethod {
		am = fmt.Sprintf(",\"assertionMethod\": \"%s#key-1\"", did)
	}
	if includeKeyAgreement {
		ka = fmt.Sprintf(",\"keyAgreement\": \"%s#key-1\"", did)
	}

	leaf, err := jwk.New(chain[0].PublicKey)
	if err != nil {
		return "", err
	}
	jleaf, err := json.Marshal(leaf)
	if err != nil {
		return "", err
	}
	doc := fmt.Sprintf(format, did, jleaf, am, ka)
	return doc, nil
}

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

func Resolve(chainPem string, did string, ignoreTime bool) (string, error) {
	chain, err := parsePemChain(chainPem)

	if err != nil {
		return "", err
	}

	if len(chain) == 0 {
		return "", errors.New("no certificate chain")
	}

	// The last certificate in the chain is assumed to be the trusted root.
	roots := []*x509.Certificate{chain[len(chain)-1]}

	chains, err := VerifyCertificateChain(chain, roots, ignoreTime)

	if err != nil {
		return "", fmt.Errorf("certificate chain verification failed: %w", err)
	}

	for _, chain := range chains {
		err = verifyDid(chain, did)
		if err != nil {
			return "", fmt.Errorf("DID verification failed: %w", err)
		}
	}

	doc, err := createDidDocument(did, chain)

	if err != nil {
		return "", fmt.Errorf("DID document creation failed: %w", err)
	}

	return doc, nil
}
