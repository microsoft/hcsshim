//go:build windows

package devguard

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func setGuard(t *testing.T, name string, value uint32) {
	t.Helper()
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE,
		`Software\Microsoft\HCS\Dev\Reboot`, registry.WRITE)
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	defer k.Close()
	if err := k.SetDWordValue(name, value); err != nil {
		t.Fatalf("SetDWordValue: %v", err)
	}
}

func clearGuard(t *testing.T, name string) {
	t.Helper()
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`Software\Microsoft\HCS\Dev\Reboot`, registry.WRITE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.DeleteValue(name)
}

func TestIsEnabled_MissingKey_ReturnsFalse(t *testing.T) {
	clearGuard(t, "TestGuardA")
	if IsEnabled("TestGuardA") {
		t.Fatal("expected false for missing key")
	}
}

func TestIsEnabled_ZeroValue_ReturnsFalse(t *testing.T) {
	setGuard(t, "TestGuardB", 0)
	defer clearGuard(t, "TestGuardB")
	if IsEnabled("TestGuardB") {
		t.Fatal("expected false for value=0")
	}
}

func TestIsEnabled_NonZeroValue_ReturnsTrue(t *testing.T) {
	setGuard(t, "TestGuardC", 1)
	defer clearGuard(t, "TestGuardC")
	if !IsEnabled("TestGuardC") {
		t.Fatal("expected true for value=1")
	}
}
