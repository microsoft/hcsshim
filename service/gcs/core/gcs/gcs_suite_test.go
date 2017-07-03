package gcs

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestGCS(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GCS Suite")
}
