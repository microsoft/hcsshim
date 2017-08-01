package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Microsoft/opengcs/service/gcsutils/remotefs"
)

// ErrUnknown is returned for an unknown remotefs command
var ErrUnknown = errors.New("unkown command")

func remotefsHandler() error {
	if len(os.Args) < 2 {
		return ErrUnknown
	}

	command := os.Args[1]
	if cmd, ok := remotefs.Commands[command]; ok {
		cmdErr := cmd(os.Stdin, os.Stdout, os.Args[2:])

		// Write the cmdErr to stderr, so that the client can handle it.
		if err := remotefs.WriteError(cmdErr, os.Stderr); err != nil {
			return err
		}

		return nil
	}

	fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
	fmt.Fprintf(os.Stderr, "known commands:\n")
	for k := range remotefs.Commands {
		fmt.Fprintf(os.Stderr, "\t%s\n", k)
	}
	return ErrUnknown
}

func remotefsMain() {
	if err := remotefsHandler(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
