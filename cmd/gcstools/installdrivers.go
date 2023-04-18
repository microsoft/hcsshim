//go:build linux
// +build linux

package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/guest/storage/overlay"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/pkg/errors"
)

const moduleExtension = ".ko"

func install(ctx context.Context) error {
	args := []string(os.Args[1:])

	if len(args) != 2 {
		return fmt.Errorf("expected two args, instead got %v", len(args))
	}
	targetOverlayPath := args[0]
	driver := args[1]

	if _, err := os.Lstat(targetOverlayPath); err == nil {
		// We assume the overlay path to be unique per set of drivers. Thus, if the path
		// exists already, we have already installed these drivers, and can quit early.
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to stat overlay dir: %s: %w", targetOverlayPath, err)
	}

	// create an overlay mount from the driver's UVM path so we can write to the
	// mount path in the UVM despite having mounted in the driver originally as
	// readonly
	upperPath := filepath.Join(targetOverlayPath, "upper")
	workPath := filepath.Join(targetOverlayPath, "work")
	rootPath := filepath.Join(targetOverlayPath, "content")
	if err := overlay.Mount(ctx, []string{driver}, upperPath, workPath, rootPath, false); err != nil {
		return err
	}

	// find all module files, which end with ".ko" extension, and remove extension
	// for use when calling `modprobe` below.
	modules := []string{}
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
		return errors.Wrapf(err, "failed to run modprobe with args %v: %s", modprobeArgs, out)
	}

	return nil
}

func installDriversMain() {
	ctx := context.Background()
	log.G(ctx).Logger.SetOutput(os.Stderr)
	if err := install(ctx); err != nil {
		log.G(ctx).Fatalf("error while installing drivers: %s", err)
	}
}
