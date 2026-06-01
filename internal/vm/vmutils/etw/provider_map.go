package etw

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Microsoft/go-winio/pkg/guid"
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

// GetDefaultLogSources returns the default log sources configuration.
func GetDefaultLogSources() LogSourcesInfo {
	return defaultLogSourcesInfo
}

// GetProviderGUIDFromName returns the provider GUID for a given provider name. If the provider name is not found in the map, it returns an empty string.
func getProviderGUIDFromName(providerName string) string {
	if guid, ok := etwNameToGUIDMap[strings.ToLower(providerName)]; ok {
		return guid
	}
	return ""
}

// providerKey returns a unique key for an EtwProvider, used for deduplication during merge.
// If both Name and GUID are present, key is "Name|GUID". If only GUID, key is GUID. Otherwise, key is Name.
func providerKey(provider EtwProvider) string {
	if provider.ProviderGUID != "" {
		if provider.ProviderName != "" {
			return provider.ProviderName + "|" + provider.ProviderGUID
		}
		return provider.ProviderGUID
	}
	return provider.ProviderName
}

// mergeProviders merges two slices of EtwProvider, with userProviders taking precedence over defaultProviders
// on key conflicts (same name, same GUID, or same name|GUID combination).
func mergeProviders(defaultProviders, userProviders []EtwProvider) []EtwProvider {
	providerMap := make(map[string]EtwProvider)
	for _, provider := range defaultProviders {
		providerMap[providerKey(provider)] = provider
	}
	for _, provider := range userProviders {
		providerMap[providerKey(provider)] = provider
	}

	merged := make([]EtwProvider, 0, len(providerMap))
	for _, provider := range providerMap {
		merged = append(merged, provider)
	}
	return merged
}

// mergeLogSources merges userSources into resultSources. Sources with matching types have their
// providers merged; unmatched user sources are appended as new entries.
func mergeLogSources(resultSources []Source, userSources []Source) []Source {
	for _, userSrc := range userSources {
		merged := false
		for i, resSrc := range resultSources {
			if userSrc.Type == resSrc.Type {
				resultSources[i].Providers = mergeProviders(resSrc.Providers, userSrc.Providers)
				merged = true
				break
			}
		}
		if !merged {
			resultSources = append(resultSources, userSrc)
		}
	}
	return resultSources
}

// DecodeAndUnmarshalLogSources decodes a base64-encoded JSON string and unmarshals it into a LogSourcesInfo.
func DecodeAndUnmarshalLogSources(base64EncodedJSONLogConfig string) (LogSourcesInfo, error) {
	jsonBytes, err := base64.StdEncoding.DecodeString(base64EncodedJSONLogConfig)
	if err != nil {
		return LogSourcesInfo{}, fmt.Errorf("error decoding base64 log config: %w", err)
	}

	var userLogSources LogSourcesInfo
	if err := json.Unmarshal(jsonBytes, &userLogSources); err != nil {
		return LogSourcesInfo{}, fmt.Errorf("error unmarshalling user log config: %w", err)
	}
	return userLogSources, nil
}

func trimGUID(in string) string {
	s := strings.TrimSpace(in)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	s = strings.TrimSpace(s)
	return s
}

// resolveGUIDsWithLookup normalizes and fills in provider GUIDs from the well-known ETW map
// for all providers across all sources. Providers with an invalid GUID are warned and skipped.
func resolveGUIDsWithLookup(sources []Source) ([]Source, error) {
	for i, src := range sources {
		for j, provider := range src.Providers {
			if provider.ProviderGUID != "" {
				guid, err := guid.FromString(trimGUID(provider.ProviderGUID))
				if err != nil {
					return nil, fmt.Errorf("invalid GUID %q for provider %q: %w", provider.ProviderGUID, provider.ProviderName, err)
				}
				sources[i].Providers[j].ProviderGUID = strings.ToLower(guid.String())
			}
			if provider.ProviderName != "" && provider.ProviderGUID == "" {
				sources[i].Providers[j].ProviderGUID = getProviderGUIDFromName(provider.ProviderName)
			}
		}
	}
	return sources, nil
}

// stripRedundantGUIDs removes the GUID from providers where both Name and GUID are present and
// the GUID matches the well-known lookup by name. This ensures sidecar-GCS prefers name-based
// policy verification. Invalid GUIDs are errored out.
func stripRedundantGUIDs(sources []Source) ([]Source, error) {
	for i, src := range sources {
		for j, provider := range src.Providers {
			if provider.ProviderName == "" || provider.ProviderGUID == "" {
				continue
			}
			guid, err := guid.FromString(trimGUID(provider.ProviderGUID))
			if err != nil {
				return nil, fmt.Errorf("invalid GUID %q for provider %q: %w", provider.ProviderGUID, provider.ProviderName, err)
			}
			if strings.EqualFold(guid.String(), getProviderGUIDFromName(provider.ProviderName)) {
				sources[i].Providers[j].ProviderGUID = ""
			} else {
				// If the GUID doesn't match the well-known GUID for the provider name,
				// we keep it but ensure it's normalized to lowercase without braces.
				// However, we remove the provider name to avoid incorrect policy matches
				// in sidecar-GCS, since the GUID is the source of truth in this case.
				sources[i].Providers[j].ProviderName = ""
				sources[i].Providers[j].ProviderGUID = strings.ToLower(guid.String())
			}
		}
	}
	return sources, nil
}

// applyGUIDPolicy applies GUID resolution or stripping to all sources depending on the includeGUIDs flag.
// See resolveGUIDsWithLookup and stripRedundantGUIDs for the respective behaviors.
func applyGUIDPolicy(sources []Source, includeGUIDs bool) ([]Source, error) {
	if len(sources) == 0 {
		return sources, nil
	}
	if includeGUIDs {
		return resolveGUIDsWithLookup(sources)
	}
	return stripRedundantGUIDs(sources)
}

// marshalAndEncodeLogSources marshals the given LogSourcesInfo to JSON and encodes it as a base64 string.
// On error, it logs and returns the original fallback string.
func marshalAndEncodeLogSources(logCfg LogSourcesInfo) (string, error) {
	jsonBytes, err := json.Marshal(logCfg)
	if err != nil {
		return "", fmt.Errorf("error marshalling log config: %w", err)
	}
	return base64.StdEncoding.EncodeToString(jsonBytes), nil
}

// UpdateLogSources updates the user provided log sources with the default log sources based on the
// configuration and returns the updated log sources as a base64 encoded JSON string.
// If there is an error in the process, it returns the original user provided log sources string.
func UpdateLogSources(base64EncodedJSONLogConfig string, useDefaultLogSources bool, includeGUIDs bool) (string, error) {
	var resultLogCfg LogSourcesInfo
	if useDefaultLogSources {
		resultLogCfg = defaultLogSourcesInfo
	}

	if base64EncodedJSONLogConfig != "" {
		userLogSources, err := DecodeAndUnmarshalLogSources(base64EncodedJSONLogConfig)
		if err != nil {
			return "", fmt.Errorf("failed to decode and unmarshal user log sources: %w", err)
		}
		resultLogCfg.LogConfig.Sources = mergeLogSources(resultLogCfg.LogConfig.Sources, userLogSources.LogConfig.Sources)

	}

	var err error
	resultLogCfg.LogConfig.Sources, err = applyGUIDPolicy(resultLogCfg.LogConfig.Sources, includeGUIDs)
	if err != nil {
		return "", fmt.Errorf("failed to apply GUID policy: %w", err)
	}

	result, err := marshalAndEncodeLogSources(resultLogCfg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal and encode log sources: %w", err)
	}
	return result, nil
}
