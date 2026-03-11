package etw

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Microsoft/hcsshim/internal/log"
)

//go:embed etw-map.json default-logsources.json
var embeddedFiles embed.FS

const (
	EtwMapFileName        = "etw-map.json"
	DefaultLogSourcesFile = "default-logsources.json"
)

var (
	onceLists                sync.Once
	onceListMap              sync.Once
	defaultLogSources        LogSourcesInfo
	defaultLogSourcesWithMap LogSourcesInfo
)

var (
	onceProvider sync.Once
	nameToGUID   map[string]string // STATIC
	guidToName   map[string]string // STATIC
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

// ETW - Map JSON structure
type EtwInfo struct {
	EtwMap []EtwProviderMap `json:"EtwProviderMap"`
}

type EtwProviderMap struct {
	ProviderName string `json:"providerName"`
	ProviderGUID string `json:"providerGuid"`
}

// NormalizeGUID takes a GUID string in various formats and normalizes it to the standard 8-4-4-4-12 format with uppercase letters. It returns an error if the input string is not a valid GUID.
func NormalizeGUID(in string) (string, error) {
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

	compact = strings.ToUpper(compact)
	return compact[0:8] + "-" +
		compact[8:12] + "-" +
		compact[12:16] + "-" +
		compact[16:20] + "-" +
		compact[20:32], nil
}

// LoadEtwMap loads the ETW provider name to GUID mapping from the embedded JSON file. It returns two maps, one for name to GUID and another for GUID to name. If there is an error in loading or parsing the file, it returns empty maps and the error.
func LoadEtwMap(ctx context.Context) (map[string]string, map[string]string, error) {
	onceProvider.Do(func() {
		b, err := embeddedFiles.ReadFile(EtwMapFileName)
		if err != nil {
			log.G(ctx).Errorf("Error reading ETW map file: %v", err)
			return
		}

		var cfg EtwInfo
		if err := json.Unmarshal(b, &cfg); err != nil {
			log.G(ctx).Errorf("Error unmarshalling ETW map file: %v", err)
			return
		}

		n2g := make(map[string]string)
		g2n := make(map[string]string)

		for _, p := range cfg.EtwMap {
			name := strings.TrimSpace(p.ProviderName)
			guid, err := NormalizeGUID(p.ProviderGUID)
			if name == "" || err != nil {
				// skip invalid entries
				log.G(ctx).Warningf("Skipping invalid ETW map entry with name %q and GUID %q: %v", p.ProviderName, p.ProviderGUID, err)
				continue
			}

			// Duplicate check
			if _, ok := n2g[name]; ok {
				// skip if already exists
				log.G(ctx).Warningf("Skipping duplicate ETW provider name %q in ETW map", name)
				continue
			}
			if _, ok := g2n[guid]; ok {
				// skip if already exists
				log.G(ctx).Warningf("Skipping duplicate ETW provider GUID %q in ETW map", guid)
				continue
			}

			n2g[name] = guid
			g2n[guid] = name
		}

		nameToGUID = n2g
		guidToName = g2n

	})

	return nameToGUID, guidToName, nil
}

// GetDefaultLogSources returns the default log sources from the embedded JSON file. If there is an error in loading or parsing the file, it returns an empty LogSourcesInfo struct and the error.
// The default log sources are defined in the "default-logsources.json" file and are loaded only once using sync.Once to ensure thread safety and performance.
// The providers in the default-logsources.json file should only have Provider Names and must not contain GUIDs as the handling of GUIDs is based on the configuration and is done in the UpdateEncodedLogSources function where we
// check if we need to include GUIDs for the log sources based on the configuration and if needed, we map the provider names to their corresponding GUIDs using the ETW map loaded from the "etw-map.json" file.
// The only exception to this is if the provider does not have any name and only has a GUID.
func GetDefaultLogSources(ctx context.Context) (LogSourcesInfo, error) {
	onceLists.Do(func() {

		allList, err := embeddedFiles.ReadFile(DefaultLogSourcesFile)
		if err != nil {
			log.G(ctx).Errorf("Error reading default log sources file: %v", err)
			return
		}

		if err := json.Unmarshal(allList, &defaultLogSources); err != nil {
			log.G(ctx).Errorf("Error unmarshalling default log sources file: %v", err)
			return
		}

		// Check if the default log sources have provider names. If they do, do not include GUIDs in the
		// default log sources, because GUID handling is based on configuration and is done in the
		// UpdateEncodedLogSources function. There we check if GUIDs are needed for the log sources and,
		// if so, map provider names to their corresponding GUIDs using the ETW map from "etw-map.json".
		// The only exception is when a provider has no name and only a GUID.
		for i := range defaultLogSources.LogConfig.Sources {
			for j := range defaultLogSources.LogConfig.Sources[i].Providers {
				if defaultLogSources.LogConfig.Sources[i].Providers[j].ProviderName != "" &&
					defaultLogSources.LogConfig.Sources[i].Providers[j].ProviderGUID != "" {
					defaultLogSources.LogConfig.Sources[i].Providers[j].ProviderGUID = ""
				}
			}
		}
	})
	return defaultLogSources, nil
}

// GetDefaultLogSourcesWithMappedGUID returns the default log sources with provider GUIDs included in the providers. If there is an error in loading the default log sources or the ETW map, it returns the default log sources without GUIDs.
func GetDefaultLogSourcesWithMappedGUID(ctx context.Context) (LogSourcesInfo, error) {
	onceListMap.Do(func() {
		_, err := GetDefaultLogSources(ctx)
		if err != nil {
			log.G(ctx).Errorf("Error getting default log sources: %v", err)
			return
		}

		var logConfig LogConfig
		for _, src := range defaultLogSources.LogConfig.Sources {
			var source Source
			source.Type = src.Type
			for _, provider := range src.Providers {
				var etwProvider EtwProvider
				etwProvider.Keywords = provider.Keywords
				etwProvider.Level = provider.Level
				etwProvider.ProviderName = provider.ProviderName
				etwProvider.ProviderGUID = GetProviderGUIDFromName(ctx, provider.ProviderName)
				source.Providers = append(source.Providers, etwProvider)
			}

			logConfig.Sources = append(logConfig.Sources, source)
		}

		defaultLogSourcesWithMap.LogConfig = logConfig
	})
	return defaultLogSourcesWithMap, nil
}

// GetProviderGUIDFromName returns the provider GUID for a given provider name. If the provider name is not found in the map, it returns an empty string.
func GetProviderGUIDFromName(ctx context.Context, providerName string) string {
	if _, _, err := LoadEtwMap(ctx); err != nil {
		log.G(ctx).Errorf("Error loading ETW map: %v", err)
		return ""
	}
	return nameToGUID[providerName]
}

// GetProviderNameFromGUID returns the provider name for a given provider GUID. If the provider GUID is not found in the map, it returns an empty string.
func GetProviderNameFromGUID(ctx context.Context, providerGUID string) string {
	if _, _, err := LoadEtwMap(ctx); err != nil {
		log.G(ctx).Errorf("Error loading ETW map: %v", err)
		return ""
	}
	return guidToName[providerGUID]
}

// UpdateLogSources updates the user provided log sources with the default log sources based on the configuration and returns the updated log sources as a base64 encoded JSON string.
// If there is an error in the process, it returns the original user provided log sources string.
func UpdateLogSources(ctx context.Context, base64EncodedJSONLogConfig string, useDefaultLogSources bool, includeGUIDs bool) string {
	var resultLogCfg LogSourcesInfo
	if useDefaultLogSources {
		resultLogCfg, _ = GetDefaultLogSources(ctx)
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
						guid, err := NormalizeGUID(provider.ProviderGUID)
						if err != nil {
							log.G(ctx).Warningf("Skipping invalid GUID %q for provider %q: %v", provider.ProviderGUID, provider.ProviderName, err)
						}
						resultLogCfg.LogConfig.Sources[i].Providers[j].ProviderGUID = guid
					}
					if provider.ProviderName != "" && provider.ProviderGUID == "" {
						resultLogCfg.LogConfig.Sources[i].Providers[j].ProviderGUID = GetProviderGUIDFromName(ctx, provider.ProviderName)
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
						guid, err := NormalizeGUID(provider.ProviderGUID)
						if err != nil {
							log.G(ctx).Warningf("Skipping invalid GUID %q for provider %q: %v", provider.ProviderGUID, provider.ProviderName, err)
							continue
						}
						if strings.EqualFold(guid, GetProviderGUIDFromName(ctx, provider.ProviderName)) {
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
