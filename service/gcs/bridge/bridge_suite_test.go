package bridge

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestBridge(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bridge Suite")
}
