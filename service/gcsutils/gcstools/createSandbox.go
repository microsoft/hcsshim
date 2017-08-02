package main

import (
	"flag"
	"io"
	"os"

	"github.com/Microsoft/opengcs/service/gcsutils/gcstools/commoncli"
	"github.com/sirupsen/logrus"
)

// PreBuiltSandboxFile is the default location for the sandbox file.
const PreBuiltSandboxFile = "/root/integration/prebuildSandbox.vhdx"

func createSandbox() error {
	logArgs := commoncli.SetFlagsForLogging()
	sandboxLocation := flag.String("file", PreBuiltSandboxFile, "Sandbox file location")
	size := flag.Int64("size", 20*1024*1024*1024, "20GB in bytes")
	flag.Parse()

	if err := commoncli.SetupLogging(logArgs...); err != nil {
		return err
	}

	logrus.Infof("got location=%s and size=%d\n", sandboxLocation, *size)
	file, err := os.Open(*sandboxLocation)
	if err != nil {
		logrus.Infof("error opening %s: %s\n", *sandboxLocation, err)
		return err
	}
	defer file.Close()

	if _, err = io.Copy(os.Stdout, file); err != nil {
		logrus.Infof("error copying %s: %s", *sandboxLocation, err)
		return err
	}

	return nil
}

func createSandboxMain() {
	if err := createSandbox(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
