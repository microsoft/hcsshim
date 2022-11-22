package cosesign1

import (
	"bytes"
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

func readFileString(filename string) (string, error) {
	data, err := readFileBytes(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

var fragmentRego string
var fragmentCose []byte
var leafPrivatePem string
var leafCertPEM string
var leafPubkeyPEM string
var certChainPEM string

func comparePEMs(pk1pem string, pk2pem string) bool {
	pk1der := pem2der([]byte(pk1pem))
	pk2der := pem2der([]byte(pk2pem))
	return bytes.Equal(pk1der, pk2der)
}

// Decode a COSE_Sign1 document and check that we get the expected payload, issuer, keys, certs etc.
func Test_UnpackAndValidateCannedFragment(t *testing.T) {
	var unpacked *UnpackedCoseSign1
	unpacked, err := UnpackAndValidateCOSE1CertChain(fragmentCose, nil, nil, false)

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
		t.Error("pubkey did not match")
	}
	if !comparePEMs(pubcert, leafCertPEM) {
		t.Error("pubcert did not match")
	}
	if cty != "application/unknown+json" {
		t.Error("cty did not match")
	}
	if payload != fragmentRego {
		t.Error("payload did not match")
	}
	if iss != "TestIssuer" {
		t.Error("iss did not match")
	}
	if feed != "TestFeed" {
		t.Error("feed did not match")
	}
}

func Test_UnpackAndValidateCannedFragmentCorrupted(t *testing.T) {
	fragCose := make([]byte, len(fragmentCose))
	copy(fragCose, fragmentCose)

	var offset = len(fragCose) / 2
	// corrupt the cose document (use the uncorrupted one as source in case we loop back to a good value)
	fragCose[offset] = fragmentCose[offset] + 1
	var _, err = UnpackAndValidateCOSE1CertChain(fragCose, nil, nil, false)

	// expect it to fail
	if err == nil {
		t.Error("corrupted document passed validation")
	}
}

// Use CreateCoseSign1 to make a document that should match the one made by the makefile
func Test_CreateCoseSign1Fragment(t *testing.T) {
	var raw, err = CreateCoseSign1([]byte(fragmentRego), "TestIssuer", "TestFeed", "application/unknown+json", []byte(certChainPEM), []byte(leafPrivatePem), "zero", cose.AlgorithmES384, false)
	if err != nil {
		t.Errorf("CreateCoseSign1 failed: %s", err.Error())
	}

	if len(raw) != len(fragmentCose) {
		t.Error("created fragment length does not match expected")
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
		_, err = UnpackAndValidateCOSE1CertChain(cose, nil, nil, false)
	}
	if err != nil {
		t.Errorf("validation of %s failed", filename)
	}
}

func Test_DidX509(t *testing.T) {
	_, err := MakeDidX509("sha256", 1, "chain.pem", "subject:CN:Test Leaf (DO NOT TRUST)", true)
	if err != nil {
		t.Errorf("did:x509 creation failed: %s", err)
	}
}

func TestMain(m *testing.M) {
	println("Generating files...")

	err := exec.Command("make", "chain.pem", "infra.rego.cose").Run()
	if err != nil {
		os.Exit(1)
	}

	fragmentRego, err = readFileString("infra.rego.base64")
	if err != nil {
		os.Exit(1)
	}
	fragmentCose, err = readFileBytes("infra.rego.cose")
	if err != nil {
		os.Exit(1)
	}
	leafPrivatePem, err = readFileString("leaf.private.pem")
	if err != nil {
		os.Exit(1)
	}
	leafCertPEM, err = readFileString("leaf.cert.pem")
	if err != nil {
		os.Exit(1)
	}
	leafPubkeyPEM, err = readFileString("leaf.public.pem")
	if err != nil {
		os.Exit(1)
	}
	certChainPEM, err = readFileString("chain.pem")
	if err != nil {
		os.Exit(1)
	}

	os.Exit(m.Run())
}
