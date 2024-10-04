//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/Microsoft/go-winio"
)

func main() {
	if err := logContainerStdoutToFile(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func logContainerStdoutToFile() (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var sout, wait net.Conn

	soutPipe := os.Getenv("CONTAINER_STDOUT")
	waitPipe := os.Getenv("CONTAINER_WAIT")

	if sout, err = winio.DialPipeContext(ctx, soutPipe); err != nil {
		return fmt.Errorf("couldn't open stdout pipe: %w", err)
	}
	defer sout.Close()

	// The only expected argument should be output file path
	if len(os.Args[1:]) != 1 {
		return fmt.Errorf("Expected exactly 1 argument, got: %d", len(os.Args[1:]))
	}

	var dest *os.File
	destPath := os.Args[1]
	if dest, err = os.Create(destPath); err != nil {
		return fmt.Errorf("couldn't open destination file: %w", err)
	}
	defer dest.Close()

	if wait, err = winio.DialPipeContext(ctx, waitPipe); err != nil {
		return fmt.Errorf("couldn't open wait pipe: %w", err)
	}
	// Indicate that logging binary is ready to receive output
	wait.Close()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err = io.Copy(dest, sout)
	}()
	wg.Wait()
	return
}
