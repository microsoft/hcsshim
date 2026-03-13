package etw

import (
	"strings"
	"testing"
)

func TestETWNameToGuidMap_AllKeysAndValuesAreLowercase(t *testing.T) {
	if len(etwNameToGuidMap) == 0 {
		t.Fatal("etwNameToGuidMap is empty")
	}

	for key, value := range etwNameToGuidMap {
		if key != strings.ToLower(key) {
			t.Fatalf("map key is not lowercase: key=%q value=%q", key, value)
		}
		if value != strings.ToLower(value) {
			t.Fatalf("map value is not lowercase: key=%q value=%q", key, value)
		}
	}
}

func isValidGuid(guid string) bool {
	// GUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (8-4-4-4-12 hex digits)
	if len(guid) != 36 {
		return false
	}
	for i, c := range guid {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return false
			}
		}
	}
	return true
}

func TestETWNameToGuidMap_AllGuidsAreValid(t *testing.T) {
	for key, guid := range etwNameToGuidMap {
		if !isValidGuid(guid) {
			t.Fatalf("invalid GUID format: key=%q guid=%q", key, guid)
		}
	}
}

func TestETWNameToGuidMap_KeysAreNonEmpty(t *testing.T) {
	for key := range etwNameToGuidMap {
		if strings.TrimSpace(key) == "" {
			t.Fatal("found empty key in etwNameToGuidMap")
		}
	}
}

func TestETWNameToGuidMap_ValuesAreNonEmpty(t *testing.T) {
	for key, value := range etwNameToGuidMap {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("found empty value for key=%q", key)
		}
	}
}
