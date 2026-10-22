package etw

import (
	"strings"
	"testing"
)

func TestSidecarDefaultLogSource(t *testing.T) {
	// Updating sources mutates the shared defaults; restore them after the test.
	original := cloneLogSourcesInfo(defaultLogSourcesInfo)
	t.Cleanup(func() { defaultLogSourcesInfo = original })
	for _, level := range []string{"Information", "Verbose"} {
		var user LogSourcesInfo
		if level == "Verbose" {
			user.LogConfig.Sources = []Source{{Type: "ETW", Providers: []EtwProvider{
				{ProviderName: SidecarProviderName, Level: level},
			}}}
		}
		encoded, err := UpdateLogSourcesFromInfo(user, true, true)
		if err != nil {
			t.Fatal(err)
		}
		sources, err := DecodeAndUnmarshalLogSources(encoded)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, source := range sources.LogConfig.Sources {
			for _, provider := range source.Providers {
				if !strings.EqualFold(provider.ProviderName, SidecarProviderName) {
					continue
				}
				count++
				if provider.Level != level || provider.ProviderGUID != GetProviderGUIDFromName(SidecarProviderName) {
					t.Fatalf("unexpected sidecar configuration: %+v", provider)
				}
			}
		}
		if count != 1 {
			t.Fatalf("sidecar provider count = %d, want 1", count)
		}
		// Keep the next configuration independent.
		defaultLogSourcesInfo = cloneLogSourcesInfo(original)
	}
}
