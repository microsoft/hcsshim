package privileged

import (
	"strconv"
	"testing"
)

func TestBitmask(t *testing.T) {
	mask := int32ToBitmask(5)
	str := strconv.FormatInt(int64(mask), 2)
	if str != "11111" {
		t.Fatalf("expected '11111' but got: %s", str)
	}

	mask = int32ToBitmask(0)
	str = strconv.FormatInt(int64(mask), 2)
	if str != "0" {
		t.Fatalf("expected '0' but got: %s", str)
	}

	mask = int32ToBitmask(2)
	str = strconv.FormatInt(int64(mask), 2)
	if str != "11" {
		t.Fatalf("expected '0' but got: %s", str)
	}
}
