// +build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guest/storage/overlay"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	lcowGlobalDriversFormat = "/run/drivers/%s"

	moduleExtension = ".ko"
)

func install(ctx context.Context) error {
	args := []string(os.Args[1:])

	if len(args) == 0 {
		return errors.New("no driver paths provided for install")
	}

	for _, driver := range args {
		modules := []string{}

		driverGUID, err := uuid.NewRandom()
		if err != nil {
			return err
		}

		// create an overlay mount from the driver's UVM path so we can write to the
		// mount path in the UVM despite having mounted in the driver originally as
		// readonly
		runDriverPath := fmt.Sprintf(lcowGlobalDriversFormat, driverGUID.String())
		upperPath := filepath.Join(runDriverPath, "upper")
		workPath := filepath.Join(runDriverPath, "work")
		rootPath := filepath.Join(runDriverPath, "content")
		if err := overlay.Mount(ctx, []string{driver}, upperPath, workPath, rootPath, false); err != nil {
			return err
		}

		// find all module files, which end with ".ko" extension, and remove extension
		// for use when calling `modprobe` below.
		if walkErr := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return errors.Wrap(err, "failed to read directory while walking dir")
			}
			if !info.IsDir() && filepath.Ext(info.Name()) == moduleExtension {
				moduleName := strings.TrimSuffix(info.Name(), moduleExtension)
				modules = append(modules, moduleName)
			}
			return nil
		}); walkErr != nil {
			return walkErr
		}

		// create a new module dependency map database for the driver
		depmodArgs := []string{"-b", rootPath}
		cmd := exec.Command("depmod", depmodArgs...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return errors.Wrapf(err, "failed to run depmod with args %v: %s", depmodArgs, out)
		}

		// run modprobe for every module name found
		modprobeArgs := append([]string{"-d", rootPath, "-a"}, modules...)
		cmd = exec.Command(
			"modprobe",
			modprobeArgs...,
		)

		out, err = cmd.CombinedOutput()
		if err != nil {
			return errors.Wrapf(err, "failed to run modporbe with args %v: %s", modprobeArgs, out)
		}
	}

	return nil
}

func installDriversMain() {
	ctx := context.Background()
	logrus.SetOutput(os.Stderr)
	if err := install(ctx); err != nil {
		logrus.Fatalf("error in install drivers: %s", err)
	}
}
