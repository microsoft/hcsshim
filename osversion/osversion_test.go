package osversion

import (
	"testing"
)

func TestOSVersionString(t *testing.T) {
	v := OSVersion{
		Version:      809042555,
		MajorVersion: 123,
		MinorVersion: 2,
		Build:        12345,
	}
	expected := "123.2.12345"
	actual := v.String()
	if actual != expected {
		t.Errorf("expected: %q, got: %q", expected, actual)
	}

	t.Run("parse back", func(t *testing.T) {
		parsed, err := Parse(actual)
		if err != nil {
			t.Errorf("failed to parse back: %q", err)
		}
		if parsed != v {
			t.Errorf("parsed version is not the same, original: %+v (%d) parsed: %+v (%d)", v, v.Version, parsed, parsed.Version)
		}
	})
}
