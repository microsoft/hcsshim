package did_x509_resolver

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

func verify_certificate_chain(chain []*x509.Certificate, trusted_roots []*x509.Certificate, ignore_time bool) ([][]*x509.Certificate, error) {
	roots := x509.NewCertPool()
	for _, cert := range trusted_roots {
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

func check_fingerprint(chain []*x509.Certificate, ca_fingerprint_alg, ca_fingerprint string) error {

	expected_ca_fingerprints := make(map[string]struct{})
	for _, cert := range chain[1:] {
		var n struct{}
		var hash []byte
		switch ca_fingerprint_alg {
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
		hash_str := base64.RawURLEncoding.EncodeToString(hash)
		expected_ca_fingerprints[hash_str] = n
	}

	if _, found := expected_ca_fingerprints[ca_fingerprint]; !found {
		return errors.New("unexpected certificate fingerprint")
	}

	return nil
}

func url_unescape(urls []string) ([]string, error) {
	r := make([]string, 0)
	for _, p := range urls {
		d, err := url.QueryUnescape(p)
		if err != nil {
			return r, errors.New("URL unescape failed")
		}
		r = append(r, d)
	}
	return r, nil
}

func oid_from_string(s string) (asn1.ObjectIdentifier, error) {
	tokens := strings.Split(s, ".")
	var ints []int
	for _, x := range tokens {
		i, err := strconv.Atoi(x)
		if err != nil {
			return asn1.ObjectIdentifier{}, errors.New("invalid OID")
		}
		ints = append(ints, i)
	}
	return asn1.ObjectIdentifier(ints), nil
}

func check_has_san(san_type string, value string, cert *x509.Certificate) error {
	switch san_type {
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
		return fmt.Errorf("unknown SAN type: %s", san_type)
	}
	return fmt.Errorf("SAN not found: %s", value)
}

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

func oidFromExtKeyUsage(eku x509.ExtKeyUsage) (oid asn1.ObjectIdentifier, ok bool) {
	for _, pair := range extKeyUsageOIDs {
		if eku == pair.extKeyUsage {
			return pair.oid, true
		}
	}
	return
}

func verify_did(chain []*x509.Certificate, did string) error {
	var top_tokens []string = strings.Split(did, "::")

	if len(top_tokens) <= 1 {
		return errors.New("invalid DID string")
	}

	var pretokens []string = strings.Split(top_tokens[0], ":")

	if len(pretokens) < 3 || pretokens[0] != "did" || pretokens[1] != "x509" {
		return errors.New("unsupported method/prefix")
	}

	if pretokens[2] != "0" {
		return errors.New("unsupported did:x509 version")
	}

	ca_fingerprint_alg := pretokens[3]
	ca_fingerprint := pretokens[4]

	policies := top_tokens[1:]

	if len(chain) < 2 {
		return errors.New("certificate chain too short")
	}

	err := check_fingerprint(chain, ca_fingerprint_alg, ca_fingerprint)
	if err != nil {
		return err
	}

	for _, policy := range policies {
		parts := strings.Split(policy, ":")

		if len(parts) < 2 {
			return errors.New("invalid policy")
		}

		policy_name, args := parts[0], parts[1:]
		switch policy_name {
		case "subject":
			if len(args) == 0 || len(args)%2 != 0 {
				return errors.New("key-value pairs required")
			}

			if len(args) < 2 {
				return errors.New("at least one key-value pair is required")
			}

			var seen_fields []string
			for i := 0; i < len(args); i += 2 {
				k := args[i]
				v, err := url.QueryUnescape(args[i+1])

				if err != nil {
					return fmt.Errorf("url_unescape failed: %s", err)
				}

				for _, sk := range seen_fields {
					if sk == k {
						return fmt.Errorf("duplicate field '%s'", k)
					}
				}
				seen_fields = append(seen_fields, k)

				leaf := chain[0]
				var field_values []string
				switch k {
				case "C":
					field_values = leaf.Subject.Country
				case "O":
					field_values = leaf.Subject.Organization
				case "OU":
					field_values = leaf.Subject.OrganizationalUnit
				case "L":
					field_values = leaf.Subject.Locality
				case "S":
					field_values = leaf.Subject.Province
				case "STREET":
					field_values = leaf.Subject.StreetAddress
				case "POSTALCODE":
					field_values = leaf.Subject.PostalCode
				case "SERIALNUMBER":
					field_values = []string{leaf.Subject.SerialNumber}
				case "CN":
					field_values = []string{leaf.Subject.CommonName}
				default:
					for _, aav := range leaf.Subject.Names {
						if aav.Type.String() == k {
							field_values = []string{aav.Value.(string)}
							break
						}
					}
					if len(field_values) == 0 {
						return fmt.Errorf("unsupported subject key: %s", k)
					}
				}
				found := false
				for _, fv := range field_values {
					if strings.Contains(fv, v) {
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
			san_type := args[0]
			san_value, err := url.QueryUnescape(args[1])
			if err != nil {
				return fmt.Errorf("url.QueryUnescape failed: %s", err)
			}
			err = check_has_san(san_type, san_value, chain[0])
			if err != nil {
				return err
			}
		case "eku":
			if len(args) != 1 {
				return errors.New("exactly one EKU required")
			}

			eku_oid, err := oid_from_string(args[0])
			if err != nil {
				return fmt.Errorf("oid_from_string failed: %s", err)
			}

			if len(chain[0].UnknownExtKeyUsage) == 0 {
				return errors.New("no EKU extension in certificate")
			}

			found_eku := false
			for _, cert_eku := range chain[0].ExtKeyUsage {
				cert_eku_oid, ok := oidFromExtKeyUsage(cert_eku)
				if ok && cert_eku_oid.Equal(eku_oid) {
					found_eku = true
					break
				}
			}
			for _, cert_eku_oid := range chain[0].UnknownExtKeyUsage {
				if cert_eku_oid.Equal(eku_oid) {
					found_eku = true
					break
				}
			}

			if !found_eku {
				return fmt.Errorf("EKU not found: %s", eku_oid)
			}
		case "fulcio-issuer":
			if len(args) != 1 {
				return errors.New("excessive arguments to fulcio-issuer")
			}
			decoded_arg, err := url.QueryUnescape(args[0])
			if err != nil {
				return fmt.Errorf("url_unescape failed: %s", err)
			}
			fulcio_issuer := "https://" + decoded_arg
			fulcio_issuer_oid, err := oid_from_string("1.3.6.1.4.1.57264.1.1")
			if err != nil {
				return fmt.Errorf("oid_from_string failed: %s", err)
			}
			found := false
			for _, ext := range chain[0].Extensions {
				if ext.Id.Equal(fulcio_issuer_oid) {
					if string(ext.Value) == fulcio_issuer {
						found = true
						break
					}
				}
			}
			if !found {
				return fmt.Errorf("invalid fulcio-issuer: %s", fulcio_issuer)
			}
		default:
			return fmt.Errorf("unsupported did:x509 policy name '%s'", policy_name)
		}
	}

	return nil
}

func create_did_document(did string, chain []*x509.Certificate) (string, error) {
	format := `
{
	"@context": "https://www.w3.org/ns/did/v1",
	"id": "%s",
	"verificationMethod": [{
		"id": "%s#key-1",
		"type": "JsonWebKey2020",
		"controller": "%s",
		"publicKeyJwk": %s,
	}]
	%s
	%s
}`

	include_assertion_method := chain[0].KeyUsage == 0 || (chain[0].KeyUsage&x509.KeyUsageDigitalSignature) != 0
	include_key_agreement := chain[0].KeyUsage == 0 || (chain[0].KeyUsage&x509.KeyUsageKeyAgreement) != 0

	if !include_assertion_method && !include_key_agreement {
		return "", errors.New("leaf certificate key usage must include digital signature or key agreement")
	}

	am := ""
	ka := ""
	if include_assertion_method {
		am = fmt.Sprintf(",\"assertionMethod\": \"%s#key-1\"", did)
	}
	if include_key_agreement {
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
	doc := fmt.Sprintf(format, did, did, did, jleaf, am, ka)
	return doc, nil
}

func parse_pem_chain(chain_pem string) ([]*x509.Certificate, error) {
	var chain []*x509.Certificate = []*x509.Certificate{}

	bs := []byte(chain_pem)
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

func resolve(chain_pem string, did string, ignore_time bool) (string, error) {
	chain, err := parse_pem_chain(chain_pem)

	if err != nil {
		return "", err
	}

	if len(chain) == 0 {
		return "", errors.New("no certificate chain")
	}

	// The last certificate in the chain is assumed to be the trusted root.
	roots := []*x509.Certificate{chain[len(chain)-1]}

	chains, err := verify_certificate_chain(chain, roots, ignore_time)

	if err != nil {
		return "", fmt.Errorf("certificate chain verification failed: %s", err)
	}

	for _, chain := range chains {
		err = verify_did(chain, did)
		if err != nil {
			return "", fmt.Errorf("DID verification failed: %s", err)
		}
	}

	doc, err := create_did_document(did, chain)

	if err != nil {
		return "", fmt.Errorf("DID document creation failed: %s", err)
	}

	return doc, nil
}
