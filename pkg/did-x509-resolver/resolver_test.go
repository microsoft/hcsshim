package did_x509_resolver

import (
	"os"
	"testing"
)


func check_failed(t *testing.T, err error) {
	if (err == nil) { t.Errorf("error: should have failed") }
}

func check_ok(t *testing.T, err error) {
	if (err != nil) { t.Errorf("error: rejected valid DID: %s", err) }
}

func load_certificate_chain(t *testing.T, path string) string {
	chain, err := os.ReadFile(path)
	if err != nil { t.Errorf("error: can't read file"); }
	return string(chain)
}

func TestWrongPrefix(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/ms-code-signing.pem")
	_, err := resolve(chain, "djd:y508:1:abcd::", true)
	check_failed(t, err)
}

func TestRootCA(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/ms-code-signing.pem")
	_, err := resolve(chain, "did:x509:0:sha256:hH32p4SXlD8n_HLrk_mmNzIKArVh0KkbCeh6eAftfGE::subject:CN:Microsoft%20Corporation", true)
	check_ok(t, err)
}

func TestIntermediateCA(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/ms-code-signing.pem")
	_, err := resolve(chain, "did:x509:0:sha256:VtqHIq_ZQGb_4eRZVHOkhUiSuEOggn1T-32PSu7R4Ys::subject:CN:Microsoft%20Corporation", true)
	check_ok(t, err)
}

func TestInvalidLeafCA(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/ms-code-signing.pem")
	_, err := resolve(chain, "did:x509:0:sha256:h::subject:CN:Microsoft%20Corporation", true)
	check_failed(t, err)
}

func TestInvalidCA(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/ms-code-signing.pem")
	_, err := resolve(chain, "did:x509:0:sha256:abc::CN:Microsoft%20Corporation", true)
	check_failed(t, err)
}

func TestMultiplePolicies(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/ms-code-signing.pem")
	_, err := resolve(chain, "did:x509:0:sha256:hH32p4SXlD8n_HLrk_mmNzIKArVh0KkbCeh6eAftfGE::eku:1.3.6.1.5.5.7.3.3::eku:1.3.6.1.4.1.311.10.3.21", true)
	check_ok(t, err)
}

func TestSubject(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/ms-code-signing.pem")
	_, err := resolve(chain, "did:x509:0:sha256:hH32p4SXlD8n_HLrk_mmNzIKArVh0KkbCeh6eAftfGE::subject:CN:Microsoft%20Corporation", true)
	check_ok(t, err)
}

func TestSubjectInvalidName(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/ms-code-signing.pem")
	_, err := resolve(chain, "did:x509:0:sha256:hH32p4SXlD8n_HLrk_mmNzIKArVh0KkbCeh6eAftfGE::subject:CN:MicrosoftCorporation", true)
	check_failed(t, err)
}

func TestSubjectDuplicateField(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/ms-code-signing.pem")
	_, err := resolve(chain, "did:x509:0:sha256:hH32p4SXlD8n_HLrk_mmNzIKArVh0KkbCeh6eAftfGE::subject:CN:Microsoft%20Corporation:CN:Microsoft%20Corporation", true)
	check_failed(t, err)
}

func TestSAN(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/fulcio-email.pem")
	_, err := resolve(chain, "did:x509:0:sha256:O6e2zE6VRp1NM0tJyyV62FNwdvqEsMqH_07P5qVGgME::san:email:igarcia%40suse.com", true)
	check_ok(t, err)
}

func TestSANInvalidType(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/fulcio-email.pem")
	_, err := resolve(chain, "did:x509:0:sha256:O6e2zE6VRp1NM0tJyyV62FNwdvqEsMqH_07P5qVGgME::san:uri:igarcia%40suse.com", true)
	check_failed(t, err)
}

func TestSANInvalidValue(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/fulcio-email.pem")
	_, err := resolve(chain, "did:x509:0:sha256:O6e2zE6VRp1NM0tJyyV62FNwdvqEsMqH_07P5qVGgME::email:bob%40example.com", true)
	check_failed(t, err)
}

func TestBadEKU(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/ms-code-signing.pem")
	_, err := resolve(chain, "did:x509:0:sha256:hH32p4SXlD8n_HLrk_mmNzIKArVh0KkbCeh6eAftfGE::eku:1.3.6.1.5.5.7.3.12", true)
	check_failed(t, err)
}

func TestGoodEKU(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/ms-code-signing.pem")
	_, err := resolve(chain, "did:x509:0:sha256:hH32p4SXlD8n_HLrk_mmNzIKArVh0KkbCeh6eAftfGE::eku:1.3.6.1.4.1.311.10.3.21", true)
	check_ok(t, err)
}

func TestEKUInvalidValue(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/ms-code-signing.pem")
	_, err := resolve(chain, "did:x509:0:sha256:hH32p4SXlD8n_HLrk_mmNzIKArVh0KkbCeh6eAftfGE::eku:1.2.3", true)
	check_failed(t, err)
}

func TestFulcioIssuerWithEmailSAN(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/fulcio-email.pem")
	_, err := resolve(chain, "did:x509:0:sha256:O6e2zE6VRp1NM0tJyyV62FNwdvqEsMqH_07P5qVGgME::fulcio-issuer:github.com%2Flogin%2Foauth::san:email:igarcia%40suse.com", true)
	check_ok(t, err)
}

func TestFulcioIssuerWithURISAN(t *testing.T) {
	chain := load_certificate_chain(t, "test-data/fulcio-github-actions.pem")
	_, err := resolve(chain, "did:x509:0:sha256:O6e2zE6VRp1NM0tJyyV62FNwdvqEsMqH_07P5qVGgME::fulcio-issuer:token.actions.githubusercontent.com::san:uri:https%3A%2F%2Fgithub.com%2Fbrendancassells%2Fmcw-continuous-delivery-lab-files%2F.github%2Fworkflows%2Ffabrikam-web.yml%40refs%2Fheads%2Fmain", true)
	check_ok(t, err)
}
