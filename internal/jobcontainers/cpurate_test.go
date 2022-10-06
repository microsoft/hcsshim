//go:build windows

package jobcontainers

import (
	"testing"
)

func assertEqual(t *testing.T, a uint32, b uint32) {
	t.Helper()
	if a != b {
		t.Fatalf("%d != %d", a, b)
	}
}

func TestJobCPURate(t *testing.T) {
	rate := calculateJobCPURate(10, 1)
	assertEqual(t, rate, 1000)

	rate = calculateJobCPURate(10, 5)
	assertEqual(t, rate, 5000)

	rate = calculateJobCPURate(20, 5)
	assertEqual(t, rate, 2500)

	rate = calculateJobCPURate(1, 1)
	assertEqual(t, rate, 10000)

	rate = calculateJobCPURate(1, 0)
	assertEqual(t, rate, 1)
}
