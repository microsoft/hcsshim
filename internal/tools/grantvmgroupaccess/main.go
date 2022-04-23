//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Microsoft/hcsshim/internal/security"
)

func main() {
	readFlag := flag.Bool("read", false, "Grant GENERIC_READ permission (DEFAULT)")
	writeFlag := flag.Bool("write", false, "Grant GENERIC_WRITE permission")
	executeFlag := flag.Bool("execute", false, "Grant GENERIC_EXECUTE permission")
	allFlag := flag.Bool("all", false, "Grant GENERIC_ALL permission")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\nUsage of %s:\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    %s [flags] file\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "    %s --read myfile.txt\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    %s --read --execute myfile.txt\n", os.Args[0])
	}

	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(-1)
	}

	desiredAccess := security.AccessMaskNone
	if flag.NFlag() == 0 {
		desiredAccess = security.AccessMaskRead
	}

	if *readFlag {
		desiredAccess |= security.AccessMaskRead
	}
	if *writeFlag {
		desiredAccess |= security.AccessMaskWrite
	}
	if *executeFlag {
		desiredAccess |= security.AccessMaskExecute
	}
	if *allFlag {
		desiredAccess |= security.AccessMaskAll
	}

	if err := security.GrantVmGroupAccessWithMask(flag.Arg(0), desiredAccess); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
}
