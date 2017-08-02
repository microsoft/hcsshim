package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/opengcs/service/gcsutils/gcstools/commoncli"
	"github.com/Microsoft/opengcs/service/gcsutils/libtar2vhd"
	"github.com/sirupsen/logrus"
)

func exportSandbox() error {
	tar2vhdArgs := commoncli.SetFlagsForTar2VHDLib()
	logArgs := commoncli.SetFlagsForLogging()
	mntPath := flag.String("path", "", "path to layer")
	flag.Parse()

	if err := commoncli.SetupLogging(logArgs...); err != nil {
		return err
	}

	options, err := commoncli.SetupTar2VHDLibOptions(tar2vhdArgs...)
	if err != nil {
		logrus.Infof("error: %s. Please use -h for params\n", err)
		return err
	}

	if *mntPath == "" {
		err = fmt.Errorf("path is required")
		logrus.Infof("error: %s. Please use -h for params\n", err)
		return err
	}

	absPath, err := filepath.Abs(*mntPath)
	if err != nil {
		logrus.Infof("error: %s. Could not get abs\n", err)
		return err
	}

	logrus.Infof("converted: Packing %s\n", absPath)
	if _, err = libtar2vhd.VHDX2Tar(absPath, os.Stdout, options); err != nil {
		logrus.Infof("failed to pack files: %s\n", err)
		return err
	}
	return nil
}

func exportSandboxMain() {
	if err := exportSandbox(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
