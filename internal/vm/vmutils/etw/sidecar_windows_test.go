//go:build windows

package etw

import (
	"testing"

	winioetw "github.com/Microsoft/go-winio/pkg/etw"
)

func TestSidecarProviderIdentity(t *testing.T) {
	provider, err := winioetw.NewProvider(SidecarProviderName, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := provider.Close(); err != nil {
			t.Error(err)
		}
	})
	if got, want := provider.ID.String(), GetProviderGUIDFromName(SidecarProviderName); got != want {
		t.Fatalf("registered provider GUID = %s, trusted GUID = %s", got, want)
	}
}
