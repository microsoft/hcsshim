package etw

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed etw-map.json
//go:embed default-logsources.json

var etwFS embed.FS
var listFS embed.FS

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
	ProviderGuid string `json:"providerGuid,omitempty"`
	Level        string `json:"level,omitempty"`
	Keywords     string `json:"keywords,omitempty"`
}

// ETW - Map JSON structure
type EtwInfo struct {
	EtwMap []EtwProviderMap `json:"EtwProviderMap"`
}

type EtwProviderMap struct {
	ProviderName string `json:"providerName"`
	ProviderGuid string `json:"providerGuid"`
}

// NormalizeGuid takes a GUID string in various formats and normalizes it to the standard 8-4-4-4-12 format with uppercase letters. It returns an error if the input string is not a valid GUID.
func NormalizeGuid(in string) (string, error) {
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
func LoadEtwMap() (map[string]string, map[string]string, error) {
	onceProvider.Do(func() {
		b, err := etwFS.ReadFile(EtwMapFileName)
		if err != nil {
			return
		}

		var cfg EtwInfo
		if err := json.Unmarshal(b, &cfg); err != nil {
			return
		}

		n2g := make(map[string]string)
		g2n := make(map[string]string)

		for _, p := range cfg.EtwMap {
			name := strings.TrimSpace(p.ProviderName)
			guid, err := NormalizeGuid(p.ProviderGuid)
			if name == "" || err != nil {
				// skip invalid entries
				continue
			}

			// Duplicate check
			if _, ok := n2g[name]; ok {
				// skip if already exists
				continue
			}
			if _, ok := g2n[guid]; ok {
				// skip if already exists
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

// GetDefaultLogSources returns the default log sources from the embedded json file. If there is an error in loading or parsing the file, it returns an empty LogSourcesInfo struct and the error.
func GetDefaultLogSources() (LogSourcesInfo, error) {
	onceLists.Do(func() {

		allList, err := listFS.ReadFile(DefaultLogSourcesFile)
		if err != nil {
			return
		}

		if err := json.Unmarshal(allList, &defaultLogSources); err != nil {
			return
		}
	})
	return defaultLogSources, nil
}

// GetDefaultLogSourcesWithMappedGuid returns the default log sources with provider GUIDs included in the providers. If there is an error in loading the default log sources or the ETW map, it returns the default log sources without GUIDs.
func GetDefaultLogSourcesWithMappedGuid() (LogSourcesInfo, error) {
	onceListMap.Do(func() {
		_, err := GetDefaultLogSources()
		if err != nil {
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
				etwProvider.ProviderGuid = GetProviderGuidFromName(provider.ProviderName)
				source.Providers = append(src.Providers, etwProvider)
			}

			logConfig.Sources = append(logConfig.Sources, source)
		}

		defaultLogSourcesWithMap.LogConfig = logConfig
	})
	return defaultLogSourcesWithMap, nil
}

// GetProviderGuidFromName returns the provider guid for a given provider name. If the provider name is not found in the map, it returns an empty string.
func GetProviderGuidFromName(providerName string) string {
	LoadEtwMap()
	return nameToGUID[providerName]
}

// GetProviderNameFromGuid returns the provider name for a given provider guid. If the provider guid is not found in the map, it returns an empty string.
func GetProviderNameFromGuid(providerGuid string) string {
	LoadEtwMap()
	return guidToName[providerGuid]
}

// Updates the user provided log sources with the default log sources based on the configuration and returns the updated log sources as a base64 encoded json string. If there is an error in the process, it returns the original user provided log sources string.
func UpdateEncodedLogSources(base64EncodedJsonLogConfig string, useDefaultLogSources bool, includeGuids bool) string {

	var resultLogCfg LogSourcesInfo
	if useDefaultLogSources {
		if includeGuids {
			resultLogCfg, _ = GetDefaultLogSourcesWithMappedGuid()
		} else {
			resultLogCfg, _ = GetDefaultLogSources()
		}
	}

	if base64EncodedJsonLogConfig != "" {
		jsonBytes, err := base64.StdEncoding.DecodeString(base64EncodedJsonLogConfig)
		if err == nil {
			var userLogConfig LogSourcesInfo
			if err := json.Unmarshal(jsonBytes, &userLogConfig); err == nil {

				resultSrcMap := make(map[string]Source)

				// Add all defaults in map
				for _, source := range resultLogCfg.LogConfig.Sources {
					resultSrcMap[source.Type] = source
				}

				for _, source := range userLogConfig.LogConfig.Sources {
					if destSrc, ok := resultSrcMap[source.Type]; ok {
						// then update the source's providers
						for _, srcProvider := range source.Providers {
							if includeGuids {
								if srcProvider.ProviderGuid == "" {
									srcProvider.ProviderGuid = GetProviderGuidFromName(srcProvider.ProviderName)
								}
							} else {
								// If Include GUIDs is false, then
								// We still include GUIDs if that is the only identity present. Only when both Name and GUID is provided for a ETW provider, we
								// check if the provided GUID is valid and remove it if we can fetch the same from our well known list of guids by using the name
								// This is because the sidecar-GCS prefers verification of log providers by name against the policy.
								if srcProvider.ProviderName != "" && srcProvider.ProviderGuid != "" {
									guid, _ := NormalizeGuid(srcProvider.ProviderGuid)
									if strings.EqualFold(guid, GetProviderGuidFromName(srcProvider.ProviderName)) {
										srcProvider.ProviderGuid = ""
									} else {
										srcProvider.ProviderGuid = guid
									}
								}
							}
							destSrc.Providers = append(destSrc.Providers, srcProvider)

						}
						resultSrcMap[source.Type] = destSrc

					} else {
						resultSrcMap[source.Type] = source
					}

				}

				var logSources []Source
				for _, src := range resultSrcMap {
					logSources = append(logSources, src)
				}

				resultLogCfg.LogConfig.Sources = logSources
			}
		}
	}

	if len(resultLogCfg.LogConfig.Sources) == 0 {
		return ""
	}

	jsonBytes, err := json.Marshal(resultLogCfg)
	if err != nil {
		return base64EncodedJsonLogConfig
	}

	encodedCfg := base64.StdEncoding.EncodeToString(jsonBytes)
	return encodedCfg
}
