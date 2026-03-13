package etw

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Microsoft/hcsshim/internal/log"
)

// Log Sources JSON structure
type LogSourcesInfo struct {
	LogConfig LogConfig `json:"LogConfig"`
}

type LogConfig struct {
	Sources []Source `json:"sources"`
}

type Source struct {
	Type      string        `json:"type"`
	Providers []EtwProvider `json:"providers"`
}

type EtwProvider struct {
	ProviderName string `json:"providerName,omitempty"`
	ProviderGUID string `json:"providerGuid,omitempty"`
	Level        string `json:"level,omitempty"`
	Keywords     string `json:"keywords,omitempty"`
}

// NormalizeGUID takes a GUID string in various formats and normalizes it to the standard 8-4-4-4-12 format with uppercase letters. It returns an error if the input string is not a valid GUID.
func normalizeGUID(in string) (string, error) {
	s := strings.TrimSpace(in)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	s = strings.TrimSpace(s)

	compact := strings.ReplaceAll(s, "-", "")
	if len(compact) != 32 {
		return "", fmt.Errorf("GUID %q has invalid length after normalization (%d, want 32 hex chars)", in, len(compact))
	}

	for i := 0; i < len(compact); i++ {
		c := compact[i]
		isHex := (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'f') ||
			(c >= 'A' && c <= 'F')
		if !isHex {
			return "", fmt.Errorf("GUID %q contains non-hex character %q", in, c)
		}
	}

	compact = strings.ToLower(compact)
	return compact[0:8] + "-" +
		compact[8:12] + "-" +
		compact[12:16] + "-" +
		compact[16:20] + "-" +
		compact[20:32], nil
}

// GetDefaultLogSources returns the default log sources configuration.
func GetDefaultLogSources() LogSourcesInfo {
	return defaultLogSourcesInfo
}

// GetProviderGUIDFromName returns the provider GUID for a given provider name. If the provider name is not found in the map, it returns an empty string.
func getProviderGUIDFromName(providerName string) string {
	if guid, ok := etwNameToGuidMap[strings.ToLower(providerName)]; ok {
		return guid
	}
	return ""
}

// UpdateLogSources updates the user provided log sources with the default log sources based on the configuration and returns the updated log sources as a base64 encoded JSON string.
// If there is an error in the process, it returns the original user provided log sources string.
func UpdateLogSources(ctx context.Context, base64EncodedJSONLogConfig string, useDefaultLogSources bool, includeGUIDs bool) string {
	var resultLogCfg LogSourcesInfo
	if useDefaultLogSources {
		resultLogCfg = defaultLogSourcesInfo
	}

	if base64EncodedJSONLogConfig != "" {
		jsonBytes, err := base64.StdEncoding.DecodeString(base64EncodedJSONLogConfig)
		if err != nil {
			log.G(ctx).Errorf("Error decoding base64 log config: %v", err)
		} else {
			var userLogSources LogSourcesInfo
			if err := json.Unmarshal(jsonBytes, &userLogSources); err != nil {
				log.G(ctx).Errorf("Error unmarshalling user log config: %v", err)
			} else {
				// Merge user log sources with default log sources based on the type. If the type matches,
				// we merge the providers. If there is a conflict in providers, we append them.
				// If the type does not match, we add the user log source as a new source.
				for _, userSrc := range userLogSources.LogConfig.Sources {
					found := false
					for i, defSrc := range resultLogCfg.LogConfig.Sources {
						if userSrc.Type == defSrc.Type {
							found = true
							// Merge providers
							providerMap := make(map[string]EtwProvider)
							for _, provider := range defSrc.Providers {
								key := provider.ProviderName
								if provider.ProviderGUID != "" {
									if key != "" {
										key = provider.ProviderName + "|" + provider.ProviderGUID
									} else {
										key = provider.ProviderGUID
									}
								}
								providerMap[key] = provider
							}
							for _, provider := range userSrc.Providers {
								key := provider.ProviderName
								if provider.ProviderGUID != "" {
									if key != "" {
										key = provider.ProviderName + "|" + provider.ProviderGUID
									} else {
										key = provider.ProviderGUID
									}
								}
								providerMap[key] = provider
							}
							etwProviders := make([]EtwProvider, 0, len(providerMap))
							for _, provider := range providerMap {
								etwProviders = append(etwProviders, provider)
							}
							resultLogCfg.LogConfig.Sources[i].Providers = etwProviders
							break
						}
					}
					if !found {
						resultLogCfg.LogConfig.Sources = append(resultLogCfg.LogConfig.Sources, userSrc)
					}
				}
			}
		}
	}

	// Append GUIDs to the providers if includeGUIDs is true. We get the GUIDs from the ETW map based on the provider names.
	// If a provider does not have a name and only has a GUID, we keep it as is.
	if len(resultLogCfg.LogConfig.Sources) > 0 {
		if includeGUIDs {
			for i, src := range resultLogCfg.LogConfig.Sources {
				for j, provider := range src.Providers {
					if provider.ProviderGUID != "" {
						guid, err := normalizeGUID(provider.ProviderGUID)
						if err != nil {
							log.G(ctx).Warningf("Skipping invalid GUID %q for provider %q: %v", provider.ProviderGUID, provider.ProviderName, err)
						}
						resultLogCfg.LogConfig.Sources[i].Providers[j].ProviderGUID = guid
					}
					if provider.ProviderName != "" && provider.ProviderGUID == "" {
						resultLogCfg.LogConfig.Sources[i].Providers[j].ProviderGUID = getProviderGUIDFromName(provider.ProviderName)
					}
				}
			}
		} else {
			// If includeGUIDs is false, we still want to include GUIDs if that is the only identity present for a provider.
			// Only when both Name and GUID is provided for a ETW provider, we check if the provided GUID is valid and remove
			// it if we can fetch the same from our well known list of guids by using the name. This is because the sidecar-GCS
			// prefers verification of log providers by name against the policy.
			for i, src := range resultLogCfg.LogConfig.Sources {
				for j, provider := range src.Providers {
					if provider.ProviderName != "" && provider.ProviderGUID != "" {
						guid, err := normalizeGUID(provider.ProviderGUID)
						if err != nil {
							log.G(ctx).Warningf("Skipping invalid GUID %q for provider %q: %v", provider.ProviderGUID, provider.ProviderName, err)
							continue
						}
						if strings.EqualFold(guid, getProviderGUIDFromName(provider.ProviderName)) {
							resultLogCfg.LogConfig.Sources[i].Providers[j].ProviderGUID = ""
						} else {
							resultLogCfg.LogConfig.Sources[i].Providers[j].ProviderGUID = guid
						}
					}
				}
			}
		}

	}

	jsonBytes, err := json.Marshal(resultLogCfg)
	if err != nil {
		log.G(ctx).Errorf("Error marshalling log config: %v", err)
		return base64EncodedJSONLogConfig
	}

	encodedCfg := base64.StdEncoding.EncodeToString(jsonBytes)
	return encodedCfg
}
