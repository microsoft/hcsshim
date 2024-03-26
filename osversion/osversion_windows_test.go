package osversion

import (
	"testing"
)

func TestOSVersionParseGet(t *testing.T) {
	v := Get()
	parsed, err := Parse(v.String())
	if err != nil {
		t.Errorf("unexpected parse error: %q", err)
	}

	if parsed != v {
		t.Errorf("unable to reparse into the same version, original: %q, parsed: %q", v, parsed)
	}
}
