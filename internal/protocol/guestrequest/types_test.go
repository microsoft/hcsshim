package guestrequest

import (
	"github.com/Microsoft/go-winio/pkg/guid"
	"testing"
)

func TestGuidValidity(t *testing.T) {
	for _, g := range ScsiControllerGuids {
		_, err := guid.FromString(g)
		if err != nil {
			t.Fatalf("GUID parsing failed: %s", err)
		}
	}
}
