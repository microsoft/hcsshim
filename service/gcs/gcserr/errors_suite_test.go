package gcserr

import (
	"io/ioutil"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

func TestErrors(t *testing.T) {
	// Turn off logging so as not to spam output.
	logrus.SetOutput(ioutil.Discard)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Errors Suite")
}
