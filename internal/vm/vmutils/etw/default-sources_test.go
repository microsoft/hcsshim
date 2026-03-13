package etw

import (
	"strings"
	"testing"
)

func TestDefaultSources_ETWProvidersExistInETWMap(t *testing.T) {
	if len(etwNameToGuidMap) == 0 {
		t.Fatal("etwNameToGuidMap is empty")
	}

	for si, src := range defaultLogSourcesInfo.LogConfig.Sources {
		// Only ETW sources should be validated against etwNameToGuidMap.
		if !strings.EqualFold(src.Type, "ETW") {
			continue
		}

		for pi, p := range src.Providers {
			if p.ProviderName == "" {
				t.Fatalf("empty ProviderName at source index %d provider index %d", si, pi)
			}

			key := strings.ToLower(p.ProviderName)
			if _, ok := etwNameToGuidMap[key]; !ok {
				t.Fatalf(
					"provider not found in etwNameToGuidMap: source index=%d provider index=%d provider=%q lookup key=%q",
					si, pi, p.ProviderName, key,
				)
			}
		}
	}
}
func TestDefaultSources_NoDuplicateProviders(t *testing.T) {
	providerSet := make(map[string]struct{})

	for si, src := range defaultLogSourcesInfo.LogConfig.Sources {
		if !strings.EqualFold(src.Type, "ETW") {
			continue
		}
		for pi, p := range src.Providers {
			key := strings.ToLower(p.ProviderName)
			if _, exists := providerSet[key]; exists {
				t.Fatalf("duplicate provider found: source index=%d provider index=%d provider=%q", si, pi, p.ProviderName)
			}
			providerSet[key] = struct{}{}
		}
	}
}

func TestDefaultSources_NoProviderGUIDProvided(t *testing.T) {
	for si, src := range defaultLogSourcesInfo.LogConfig.Sources {
		if !strings.EqualFold(src.Type, "ETW") {
			continue
		}
		for pi, p := range src.Providers {
			if p.ProviderGUID != "" {
				t.Fatalf("ProviderGUID should not be provided: source index=%d provider index=%d provider=%q guid=%q", si, pi, p.ProviderName, p.ProviderGUID)
			}
		}
	}
}

func TestDefaultSources_AtLeastOneETWSource(t *testing.T) {
	found := false
	for _, src := range defaultLogSourcesInfo.LogConfig.Sources {
		if strings.EqualFold(src.Type, "ETW") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no ETW source found in defaultLogSourcesInfo")
	}
}
