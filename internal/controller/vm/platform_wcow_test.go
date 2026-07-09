//go:build windows && wcow

package vm

func isLCOW() bool { return false }
