package runc

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestRunc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RunC Suite")
}
