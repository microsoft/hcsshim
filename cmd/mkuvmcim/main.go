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

func main() {
	var (
		layerTar string
		destPath string
	)

	flag.StringVar(&layerTar, "layer", "", "Path to the source layer tar")
	flag.StringVar(&destPath, "dest", "", "Path to the destination directory")
	flag.Parse()

	if layerTar == "" || destPath == "" {
		fmt.Println("Error: Both -layer and -dest flags are required")
		flag.Usage()
		os.Exit(1)
	}

	// 5 minutes should be more than enough to extract all the files
	ctx, cancelFunc := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancelFunc()

	if _, err := extractuvm.MakeUtilityVMCIMFromTar(ctx, layerTar, destPath); err != nil {
		fmt.Printf("failed to create UVM CIM: %s", err)
		os.Exit(1)
	}
}
