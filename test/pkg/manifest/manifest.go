//go:build windows

// This package allows tests can include the .syso to manifest them to pick up the right Windows build
package manifest

//go:generate pwsh -Command "../../../scripts/New-ResourceObjectFile.ps1 -ErrorAction 'Stop' -Destination '.' -Name 'hcsshim-test' -UseVersionFile -Architecture 'all'"
