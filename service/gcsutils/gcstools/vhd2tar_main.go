package main

import (
	"flag"
	"os"

	"github.com/Microsoft/opengcs/service/gcsutils/gcstools/commoncli"
	"github.com/Microsoft/opengcs/service/gcsutils/libtar2vhd"
	"github.com/Sirupsen/logrus"
)

func vhd2tar() error {
	tar2vhdArgs := commoncli.SetFlagsForTar2VHDLib()
	logArgs := commoncli.SetFlagsForLogging()
	flag.Parse()

	options, err := commoncli.SetupTar2VHDLibOptions(tar2vhdArgs...)
	if err != nil {
		logrus.Infof("error: %s. Please use -h for params\n", err)
		return err
	}

	if err = commoncli.SetupLogging(logArgs...); err != nil {
		logrus.Infof("error: %s. Please use -h for params\n", err)
		return err
	}

	if _, err = libtar2vhd.VHD2Tar(os.Stdin, os.Stdout, options); err != nil {
		logrus.Infof("svmutilsMain failed with %s\n", err)
		return err
	}
	return nil
}

func vhd2tarMain() {
	if err := vhd2tar(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
