//go:build linux
// +build linux

package cosesign1

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/veraison/go-cose"
)

func readFileBytes(filename string) ([]byte, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		println("Error reading '" + filename + "': " + string(err.Error()))
		return nil, err
	}
	if len(content) == 0 {
		println("Warning: empty file '" + filename + "'")
	}
	return content, nil
}

func readFileBytesOrExit(filename string) []byte {
	val, err := readFileBytes(filename)
	if err != nil {
		println("failed to load from file '" + filename + "' with error " + string(err.Error()))
		os.Exit(1)
	}
	return val
}

func readFileStringOrExit(filename string) string {
	val := readFileBytesOrExit(filename)
	return string(val)
}

var fragmentRego string
var fragmentCose []byte
var leafPrivatePem string
var leafCertPEM string
var leafPubkeyPEM string
var certChainPEM string

func TestMain(m *testing.M) {
	fmt.Println("Generating files...")

	err := exec.Command("make", "chain.pem", "infra.rego.cose").Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build the required test files: %s", err)
		os.Exit(1)
	}

	fragmentRego = readFileStringOrExit("infra.rego.base64")
	fragmentCose = readFileBytesOrExit("infra.rego.cose")
	leafPrivatePem = readFileStringOrExit("leaf.private.pem")
	leafCertPEM = readFileStringOrExit("leaf.cert.pem")
	leafPubkeyPEM = readFileStringOrExit("leaf.public.pem")
	certChainPEM = readFileStringOrExit("chain.pem")

	os.Exit(m.Run())
}

func comparePEMs(pk1pem string, pk2pem string) bool {
	pk1der := pem2der([]byte(pk1pem))
	pk2der := pem2der([]byte(pk2pem))
	return bytes.Equal(pk1der, pk2der)
}

func base64PublicKeyToPEM(base64Key string) string {
	begin := "-----BEGIN PUBLIC KEY-----\n"
	end := "\n-----END PUBLIC KEY-----"

	pemData := begin + base64Key + end
	return pemData
}

// Decode a COSE_Sign1 document and check that we get the expected payload, issuer, keys, certs etc.
func Test_UnpackAndValidateCannedFragment(t *testing.T) {
	var unpacked *UnpackedCoseSign1
	unpacked, err := UnpackAndValidateCOSE1CertChain(fragmentCose)

	if err != nil {
		t.Errorf("UnpackAndValidateCOSE1CertChain failed: %s", err.Error())
	}

	iss := unpacked.Issuer
	feed := unpacked.Feed
	pubkey := base64PublicKeyToPEM(unpacked.Pubkey)
	pubcert := base64CertToPEM(unpacked.Pubcert)
	payload := string(unpacked.Payload[:])
	cty := unpacked.ContentType

	if !comparePEMs(pubkey, leafPubkeyPEM) {
		t.Fatal("pubkey did not match")
	}
	if !comparePEMs(pubcert, leafCertPEM) {
		t.Fatal("pubcert did not match")
	}
	if cty != "application/unknown+json" {
		t.Fatal("cty did not match")
	}
	if payload != fragmentRego {
		t.Fatal("payload did not match")
	}
	if iss != "TestIssuer" {
		t.Fatal("iss did not match")
	}
	if feed != "TestFeed" {
		t.Fatal("feed did not match")
	}
}

func Test_UnpackAndValidateCannedFragmentCorrupted(t *testing.T) {
	fragCose := make([]byte, len(fragmentCose))
	copy(fragCose, fragmentCose)

	offset := len(fragCose) / 2
	// corrupt the cose document (use the uncorrupted one as source in case we loop back to a good value)
	fragCose[offset] = fragmentCose[offset] + 1

	_, err := UnpackAndValidateCOSE1CertChain(fragCose)
	// expect it to fail
	if err == nil {
		t.Fatal("corrupted document passed validation")
	}
}

// Use CreateCoseSign1 to make a document that should match the one made by the makefile
func Test_CreateCoseSign1Fragment(t *testing.T) {
	var raw, err = CreateCoseSign1([]byte(fragmentRego), "TestIssuer", "TestFeed", "application/unknown+json", []byte(certChainPEM), []byte(leafPrivatePem), "zero", cose.AlgorithmES384)
	if err != nil {
		t.Fatalf("CreateCoseSign1 failed: %s", err)
	}

	if len(raw) != len(fragmentCose) {
		t.Fatal("created fragment length does not match expected")
	}

	for i := range raw {
		if raw[i] != fragmentCose[i] {
			t.Errorf("created fragment byte offset %d does not match expected", i)
		}
	}
}

func Test_OldCose(t *testing.T) {
	filename := "esrp.test.cose"
	cose, err := readFileBytes(filename)
	if err == nil {
		_, err = UnpackAndValidateCOSE1CertChain(cose)
	}
	if err != nil {
		t.Fatalf("validation of %s failed: %s", filename, err)
	}
}

func Test_DidX509(t *testing.T) {
	chainPEMBytes, err := os.ReadFile("chain.pem")
	if err != nil {
		t.Fatalf("failed to read PEM: %s", err)
	}
	chainPEM := string(chainPEMBytes)

	if _, err := MakeDidX509("sha256", 1, chainPEM, "subject:CN:Test Leaf (DO NOT TRUST)", true); err != nil {
		t.Fatalf("did:x509 creation failed: %s", err)
	}
}
