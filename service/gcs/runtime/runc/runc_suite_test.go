package runc

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRunc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RunC Suite")
}
