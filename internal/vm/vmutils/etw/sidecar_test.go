package etw

import (
	"strings"
	"testing"
)

func TestSidecarDefaultLogSource(t *testing.T) {
	// Updating sources mutates the shared defaults; restore them after the test.
	original := cloneLogSourcesInfo(defaultLogSourcesInfo)
	t.Cleanup(func() { defaultLogSourcesInfo = original })
	for _, tc := range []struct {
		name  string
		level string
	}{
		{SidecarProviderName, "Information"},
		{SidecarProviderName, "Verbose"},
		{strings.ToLower(SidecarProviderName), "Verbose"},
		{strings.ToUpper(SidecarProviderName), "Verbose"},
	} {
		var user LogSourcesInfo
		if tc.level == "Verbose" {
			user.LogConfig.Sources = []Source{{Type: "ETW", Providers: []EtwProvider{
				{ProviderName: tc.name, Level: tc.level},
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
				if provider.Level != tc.level || provider.ProviderGUID != GetProviderGUIDFromName(SidecarProviderName) {
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

func TestMergeProvidersCaseInsensitive(t *testing.T) {
	const id = "5400e7dc-0f0f-553d-19b5-3672635b5f7b"
	for _, withName := range []bool{false, true} {
		defaultProvider := EtwProvider{ProviderGUID: id, Level: "Information"}
		userProvider := EtwProvider{ProviderGUID: strings.ToUpper(id), Level: "Verbose"}
		if withName {
			defaultProvider.ProviderName = SidecarProviderName
			userProvider.ProviderName = strings.ToLower(SidecarProviderName)
		}
		merged := mergeProviders([]EtwProvider{defaultProvider}, []EtwProvider{userProvider})
		if len(merged) != 1 || merged[0] != userProvider {
			t.Fatalf("case-insensitive override failed: %+v", merged)
		}
	}
}
