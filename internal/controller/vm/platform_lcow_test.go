//go:build windows && lcow

package vm

func isLCOW() bool { return true }
