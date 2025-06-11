//go:build windows
// +build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Microsoft/hcsshim/pkg/extractuvm"
)

func run() error {
	var (
		layerTar string
		destPath string
	)

	flag.StringVar(&layerTar, "layer", "", "Path to the gzipped layer tar")
	flag.StringVar(&destPath, "dest", "", "Path to the destination directory")
	flag.Parse()

	if layerTar == "" || destPath == "" {
		flag.Usage()
		return fmt.Errorf("both -layer and -dest flags are required")
	}

	// 5 minutes should be more than enough to extract all the files
	ctx, cancelFunc := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancelFunc()

	_, err := extractuvm.MakeUtilityVMCIMFromTar(ctx, layerTar, destPath)
	if err != nil {
		return fmt.Errorf("failed to create UVM CIM: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
