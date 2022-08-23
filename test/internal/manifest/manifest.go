//go:build windows

// This package allows tests can include the .syso to manifest them to pick up the right Windows build
package manifest

//go:generate go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo -platform-specific
