package oci

import (
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
)

var DefaultUnixEnv = []string{
	"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
}

func LinuxImageConfig(entry, cmd []string, wd string) *imagespec.ImageConfig {
	return &imagespec.ImageConfig{
		WorkingDir: wd,
		Entrypoint: entry,
		Cmd:        cmd,
		User:       "",
		Env:        DefaultUnixEnv,
	}
}
