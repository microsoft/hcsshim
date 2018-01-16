package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

var commands = map[string]func(){
	"tar2vhd":       tar2vhdMain,
	"vhd2tar":       vhd2tarMain,
	"exportSandbox": exportSandboxMain,
	"netnscfg":      netnsConfigMain,
	"remotefs":      remotefsMain,
}

func main() {
	cmd := filepath.Base(os.Args[0])
	if mainFunc, ok := commands[cmd]; ok {
		defer writePanicLog()
		mainFunc()

		// The called program might have exited to return a custom return code.
		// If it returned, then assume success.
		os.Exit(0)
	}

	// Unknown command
	fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
	fmt.Fprintf(os.Stderr, "known commands:\n")
	for k := range commands {
		fmt.Fprintf(os.Stderr, "\t%s\n", k)
	}
	os.Exit(127)
}

// Very basic panic log writer that dumps the stack to file
func writePanicLog() {
	recover()
	var logger *log.Logger
	f, _ := os.Create(fmt.Sprintf("/tmp/paniclog.%s.%d", filepath.Base(os.Args[0]), os.Getpid()))
	logger = log.New(f, "", log.LstdFlags)
	buf := make([]byte, 1<<16)
	runtime.Stack(buf, true)
	logger.Printf("%s", buf)
	os.Exit(1)
}
