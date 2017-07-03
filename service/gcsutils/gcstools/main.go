package main

import (
	"fmt"
	"os"
	"path/filepath"
)

var commands = map[string]func(){
	"tar2vhd":       tar2vhd_main,
	"vhd2tar":       vhd2tar_main,
	"createSandbox": createSandbox_main,
	"exportSandbox": exportSandbox_main,
}

func main() {
	cmd := filepath.Base(os.Args[0])
	if mainFunc, ok := commands[cmd]; ok {
		mainFunc()

		// The called program might have exited to return a custom return code.
		// If it returned, then assume success.
		os.Exit(0)
	}

	// Unknown command
	fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
	fmt.Fprintf(os.Stderr, "known commands:\n")
	for k, _ := range commands {
		fmt.Fprintf(os.Stderr, "\t%s\n", k)
	}
	os.Exit(127)
}
