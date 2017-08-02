package runc

import (
	"io/ioutil"
	"testing"

	"github.com/sirupsen/logrus"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRunc(t *testing.T) {
	// Turn off logging so as not to spam output.
	logrus.SetOutput(ioutil.Discard)

	RegisterFailHandler(Fail)
	RunSpecs(t, "RunC Suite")
}
