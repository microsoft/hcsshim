package cosesign1

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/veraison/go-cose"
)

func readFileBytes(filename string) []byte {
	content, err := os.ReadFile(filename)
	if err != nil {
		println("Error reading '" + filename + "': " + string(err.Error()))
	}
	if len(content) == 0 {
		println("Warning: empty file '" + filename + "'")
	}
	return content
}

func readFileString(filename string) string {
	return string(readFileBytes(filename))
}

var FragmentRego string
var FragmentCose []byte
var LeafPrivatePem string
var LeafCertPEM string
var LeafPubkeyPEM string
var CertChainPEM string

func comparePEMs(pk1pem string, pk2pem string) bool {
	pk1der := pem2der([]byte(pk1pem))
	pk2der := pem2der([]byte(pk2pem))
	return bytes.Equal(pk1der, pk2der)
}

/*
	Decode a COSE_Sign1 document and check that we get the expected payload, issuer, keys, certs etc.
*/

func Test_UnpackAndValidateCannedFragment(t *testing.T) {
	var unpacked UnpackedCoseSign1
	unpacked, err := UnpackAndValidateCOSE1CertChain(FragmentCose, nil, nil, false, false)

	if err != nil {
		t.Errorf("UnpackAndValidateCOSE1CertChain failed: %s", err.Error())
	}

	var iss = unpacked.Issuer
	var feed = unpacked.Feed
	var pubkey = base64PublicKeyToPEM(unpacked.Pubkey)
	var pubcert = base64CertToPEM(unpacked.Pubcert)
	var payload = string(unpacked.Payload[:])
	var cty = unpacked.ContentType

	if !comparePEMs(pubkey, LeafPubkeyPEM) {
		t.Error("pubkey did not match")
	}
	if !comparePEMs(pubcert, LeafCertPEM) {
		t.Error("pubcert did not match")
	}
	if cty != "application/unknown+json" {
		t.Error("cty did not match")
	}
	if payload != FragmentRego {
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
	FragmentCose2 := make([]byte, len(FragmentCose))

	for i := range FragmentCose2 {
		FragmentCose2[i] = FragmentCose[i]
	}

	var offset = len(FragmentCose2) / 2
	FragmentCose2[offset] = FragmentCose[offset] + 1 // corrupt the cose document (use the uncorrupted one as source in case we loop back to a good value)
	var _, err = UnpackAndValidateCOSE1CertChain(FragmentCose2, nil, nil, false, false)

	// expect it to fail
	if err == nil {
		t.Error("corrupted document passed validation")
	}
}

/*
	Use CreateCoseSign1 to make a document that should match the one made by the makefile
*/

func Test_CreateCoseSign1Fragment(t *testing.T) {
	var raw, err = CreateCoseSign1([]byte(FragmentRego), "TestIssuer", "TestFeed", "application/unknown+json", []byte(CertChainPEM), []byte(LeafPrivatePem), "zero", cose.AlgorithmES384, false)
	if err != nil {
		t.Errorf("CreateCoseSign1 failed: %s", err.Error())
	}

	if len(raw) != len(FragmentCose) {
		t.Error("created fragment length does not match expected")
	}

	for which := range raw {
		if raw[which] != FragmentCose[which] {
			t.Errorf("created fragment byte offset %d does not match expected", which)
		}
	}
}

func Test_OldCose(t *testing.T) {
	filename := "old.1.cose"
	cose := readFileBytes(filename)
	_, err := UnpackAndValidateCOSE1CertChain(cose, nil, nil, false, false)
	if err != nil {
		t.Errorf("validation of %s failed", filename)
	}
}

func Test_DidX509(t *testing.T) {
	didx509, err := MakeDidX509("sha256", 1, "chain.pem", "subject:CN:Test Leaf (DO NOT TRUST)", true)
	if err != nil {
		t.Errorf("did:x509 creation failed: %s", err)
	}
	print(didx509)
}

func TestMain(m *testing.M) {
	println("Generating files...")

	err := exec.Command("make", "chain.pem", "infra.rego.cose").Run()
	if err != nil {
		os.Exit(1)
	}

	FragmentRego = readFileString("infra.rego.base64")
	FragmentCose = readFileBytes("infra.rego.cose")
	LeafPrivatePem = readFileString("leaf.private.pem")
	LeafCertPEM = readFileString("leaf.cert.pem")
	LeafPubkeyPEM = readFileString("leaf.public.pem")
	CertChainPEM = readFileString("chain.pem")

	os.Exit(m.Run())
}
