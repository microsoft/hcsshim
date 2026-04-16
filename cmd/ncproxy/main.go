//go:build windows

package main

import (
	"os"

	"github.com/sirupsen/logrus"
)

//go:generate pwsh -Command "../../scripts/New-ResourceObjectFile.ps1 -ErrorAction 'Stop' -Destination '.' -Name 'ncproxy' -UseVersionFile -Architecture 'all'"

func main() {
	if err := app().Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
