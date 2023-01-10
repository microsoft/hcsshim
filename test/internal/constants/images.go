package constants

// not technically constants, but close enough ...

import (
	"errors"
	"fmt"

	"github.com/Microsoft/hcsshim/osversion"
)

const (
	DockerImageRepo     = "docker.io/library"
	MCRWindowsImageRepo = "mcr.microsoft.com/windows"

	ImageLinuxAlpineLatest = "docker.io/library/alpine:latest"
	ImageLinuxPause31      = "k8s.gcr.io/pause:3.1"
	ImageMCRLinuxPause     = "mcr.microsoft.com/oss/kubernetes/pause:3.1"
)

var ErrUnsupportedBuild = errors.New("unsupported build")

var (
	ImageWindowsNanoserver1709 = NanoserverImage("1709")
	ImageWindowsNanoserver1803 = NanoserverImage("1803")
	ImageWindowsNanoserver1809 = NanoserverImage("1809")
	ImageWindowsNanoserver1903 = NanoserverImage("1903")
	ImageWindowsNanoserver1909 = NanoserverImage("1909")
	ImageWindowsNanoserver2004 = NanoserverImage("2004")
	ImageWindowsNanoserver2009 = NanoserverImage("2009")

	ImageWindowsNanoserverRS3  = ImageWindowsNanoserver1709
	ImageWindowsNanoserverRS4  = ImageWindowsNanoserver1803
	ImageWindowsNanoserverRS5  = ImageWindowsNanoserver1809
	ImageWindowsNanoserver19H1 = ImageWindowsNanoserver1903
	ImageWindowsNanoserver19H2 = ImageWindowsNanoserver1909
	ImageWindowsNanoserver20H1 = ImageWindowsNanoserver2004
	ImageWindowsNanoserver20H2 = NanoserverImage("20H2")

	ImageWindowsNanoserverLTSC2019 = ImageWindowsNanoserver1809
	ImageWindowsNanoserverLTSC2022 = NanoserverImage("ltsc2022")

	ImageWindowsServercore1709 = ServercoreImage("1709")
	ImageWindowsServercore1803 = ServercoreImage("1803")
	ImageWindowsServercore1809 = ServercoreImage("1809")
	ImageWindowsServercore1903 = ServercoreImage("1903")
	ImageWindowsServercore1909 = ServercoreImage("1909")
	ImageWindowsServercore2004 = ServercoreImage("2004")
	ImageWindowsServercore2009 = ServercoreImage("2009")

	ImageWindowsServercoreRS3  = ImageWindowsServercore1709
	ImageWindowsServercoreRS4  = ImageWindowsServercore1803
	ImageWindowsServercoreRS5  = ImageWindowsServercore1809
	ImageWindowsServercore19H1 = ImageWindowsServercore1903
	ImageWindowsServercore19H2 = ImageWindowsServercore1909
	ImageWindowsServercore20H1 = ImageWindowsServercore2004
	ImageWindowsServercore20H2 = ServercoreImage("20H2")

	ImageWindowsServercoreLTSC2019 = ImageWindowsServercore1809
	ImageWindowsServercoreLTSC2022 = ServercoreImage("ltsc2022")
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
	return makeImageURL(MCRWindowsImageRepo, "nanoserver", tag)
}

func ServercoreImage(tag string) string {
	return makeImageURL(MCRWindowsImageRepo, "servercore", tag)
}

var _buildToTag = map[uint16]string{
	osversion.RS1:      "1607",
	osversion.RS2:      "1703",
	osversion.RS3:      "1709",
	osversion.RS4:      "1803",
	osversion.RS5:      "1809",
	osversion.V19H1:    "1903",
	osversion.V19H2:    "1909",
	osversion.V20H1:    "2004",
	osversion.V20H2:    "20H2",
	osversion.LTSC2022: "ltsc2022",
}

func ImageFromBuild(build uint16) (string, error) {
	if tag, ok := _buildToTag[build]; ok {
		return tag, nil
	}

	// Due to some efforts in improving down-level compatibility for Windows containers (see
	// https://techcommunity.microsoft.com/t5/containers/windows-server-2022-and-beyond-for-containers/ba-p/2712487)
	// the ltsc2022 image should continue to work on builds ws2022 and onwards. With this in mind,
	// if there's no mapping for the host build, just use the Windows Server 2022 image.
	if build > osversion.LTSC2022 {
		return "ltsc2022", nil
	}
	return "", ErrUnsupportedBuild
}
