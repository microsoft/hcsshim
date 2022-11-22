package cosesign1

//	Little handy utilities that make logging and a command line tool easier.

import (
	"fmt"
	"os"

	"crypto/x509"
	"encoding/base64"
	"io"
	"log"

	"github.com/veraison/go-cose"
)

func ReadBlob(filename string) []byte {
	content, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	return content
}

func ReadString(filename string) string {
	content := ReadBlob(filename)
	str := string(content)
	return str
}

// Type to replace the rand.Reader with a source of fixed salt.
// Can be provided to the cose.Sign1 method instead of rand.Reader such that
// the signature is deterministic for testing and debugging purposes.
type fixedReader struct {
	valueToReturn byte
}

func (fr *fixedReader) Read(p []byte) (int, error) {
	if len(p) > 0 {
		p[0] = fr.valueToReturn
		return 1, nil
	}
	return 0, nil
}

func NewFixedReader(value byte) io.Reader {
	return &fixedReader{valueToReturn: value}
}

func WriteBlob(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func WriteString(path string, str string) error {
	var data = []byte(str)
	return WriteBlob(path, data)
}

func x509ToBase64(cert *x509.Certificate) string {
	base64Cert := base64.StdEncoding.EncodeToString(cert.Raw)

	return base64Cert
}

func keyToBase64(key any) string {
	derKey, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return ""
	}
	base64Key := base64.StdEncoding.EncodeToString(derKey)

	return base64Key
}

/*
	Create a pem of the form:

-----BEGIN CERTIFICATE-----
single line base64 standard encoded raw DER certificate
-----END CERTIFICATE-----

	Note that there are no extra line breaks added and that a string compare will need to accomodate that.
*/

func x509ToPEM(cert *x509.Certificate) string {
	base64Cert := x509ToBase64(cert)
	return base64CertToPEM(base64Cert)
}

func base64CertToPEM(base64Cert string) string {
	var begin = "-----BEGIN CERTIFICATE-----\n"
	var end = "\n-----END CERTIFICATE-----"

	pemData := begin + base64Cert + end

	return pemData
}

func keyToPEM(key any) string {
	base64Key := keyToBase64(key)
	return base64PublicKeyToPEM(base64Key)
}

func base64PublicKeyToPEM(base64Key string) string {
	var begin = "-----BEGIN PUBLIC KEY-----\n"
	var end = "\n-----END PUBLIC KEY-----"

	pemData := begin + base64Key + end
	return pemData
}

func PrintCert(name string, x509cert *x509.Certificate) {
	log.Printf("%s:\n", name)
	log.Printf("  Issuer = %s\n", x509cert.Issuer.String())
	log.Printf("  Subject = %s\n", x509cert.Subject.String())
	log.Printf("  AuthorityKeyId = %q\n", x509cert.AuthorityKeyId)
	log.Printf("  SubjectKeyId = %q\n", x509cert.SubjectKeyId)

	var pem = x509ToPEM(x509cert) // blob of the leaf x509 cert reformatted into pem (base64) style as per the fragment policy rules expect
	var pubKey = x509cert.PublicKey
	var pubKeyPem = keyToPEM(pubKey)

	log.Printf("  Cert PEM = \n%s\n", pem)
	log.Printf("  Public Key PEM = \n%s\n", pubKeyPem)
}

func StringToAlgorithm(algoType string) (cose.Algorithm, error) {
	var algo cose.Algorithm
	var err error = nil

	switch algoType {
	case "PS256":
		algo = cose.AlgorithmPS256
	case "PS384":
		algo = cose.AlgorithmPS384
	case "PS512":
		algo = cose.AlgorithmPS512
	case "ES256":
		algo = cose.AlgorithmES256
	case "ES384":
		algo = cose.AlgorithmES384
	case "ES512":
		algo = cose.AlgorithmES512
	case "EdDSA":
		algo = cose.AlgorithmEd25519
	default:
		algo = cose.AlgorithmPS384
		err = fmt.Errorf("unknown cose.Algorithm type %s", algoType)
	}
	return algo, err
}
