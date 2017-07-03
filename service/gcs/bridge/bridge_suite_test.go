package bridge

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestBridge(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bridge Suite")
}
