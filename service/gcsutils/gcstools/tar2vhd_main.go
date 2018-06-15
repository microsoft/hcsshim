package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Microsoft/opengcs/service/gcsutils/gcstools/commoncli"
	"github.com/Microsoft/opengcs/service/gcsutils/libtar2vhd"
	"github.com/sirupsen/logrus"
)

func tar2vhd() error {
	tar2vhdArgs := commoncli.SetFlagsForTar2VHDLib()
	logArgs := commoncli.SetFlagsForLogging()
	flag.Parse()

	options, err := commoncli.SetupTar2VHDLibOptions(tar2vhdArgs...)
	if err != nil {
		logrus.Infof("error: %s. Please use -h for params", err)
		return err
	}

	err = commoncli.SetupLogging(logArgs...)
	if err != nil {
		logrus.Infof("error: %s. Please useu-h for params", err)
		return err
	}

	_, err = libtar2vhd.Tar2VHD(os.Stdin, os.Stdout, options)
	if err != nil {
		logrus.Infof("svmutilsMain failed with %s", err)
		return err
	}
	return nil
}

func tar2vhdMain() {
	if err := tar2vhd(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}
