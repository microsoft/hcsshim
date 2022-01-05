package testutilities

import (
	"fmt"
	"time"
)

// TODO: move these somewhere common for all tests (functional, cri-containerd, etc.)

const (
	connectTimeout = time.Second * 10

	PlatformWindows    = "windows"
	PlatformLinux      = "linux"
	SnapshotterWindows = "windows"
	SnapshotterLinux   = "windows-lcow"

	McrWindowsImageRepo = "mcr.microsoft.com/windows"

	ImageLinuxAlpineLatest = "docker.io/library/alpine:latest"
)

var (
	ImageWindowsNanoserver1809     = NanoserverImage("1809")
	ImageWindowsNanoserver1903     = NanoserverImage("1903")
	ImageWindowsNanoserver1909     = NanoserverImage("1909")
	ImageWindowsNanoserver2004     = NanoserverImage("2004")
	ImageWindowsNanoserver2009     = NanoserverImage("2009")
	ImageWindowsNanoserverLtsc2022 = NanoserverImage("ltsc2022")

	ImageWindowsServercore1809     = ServercoreImage("1809")
	ImageWindowsServercore1903     = ServercoreImage("1903")
	ImageWindowsServercore1909     = ServercoreImage("1909")
	ImageWindowsServercore2004     = ServercoreImage("2004")
	ImageWindowsServercore2009     = ServercoreImage("2009")
	ImageWindowsServercoreLtsc2022 = ServercoreImage("ltsc2022")
)

// all inputs should be predefined and vetted
// may not be formatted correctly for arbitrary inputs
func makeImageURL(repo, image, tag string) string {
	r := fmt.Sprintf("%s/%s", repo, image)
	if tag != "" {
		r = fmt.Sprintf("%s:%s", r, tag)
	}
	return r
}

func NanoserverImage(tag string) string {
	return makeImageURL(McrWindowsImageRepo, "nanoserver", tag)
}

func ServercoreImage(tag string) string {
	return makeImageURL(McrWindowsImageRepo, "servercore", tag)
}
