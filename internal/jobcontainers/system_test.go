package jobcontainers

import (
	"testing"
)

func TestSystemInfo(t *testing.T) {
	_, err := systemProcessInformation()
	if err != nil {
		t.Fatal(err)
	}
}
