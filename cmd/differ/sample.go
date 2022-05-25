//go:build windows

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/urfave/cli/v2"
	"golang.org/x/sys/windows"
)

var testCommand = &cli.Command{
	Name:    "test",
	Aliases: []string{"t"},
	Usage:   "test command to check re-exec works and privileges are reduced",
	Action:  actionReExecWrapper(test, withPrivileges(privs)),
}

func test(c *cli.Context) error {
	log.G(c.Context).Warning("testing re-exec")
	f, err := os.Open(filename)
	if err == nil {
		if b, err := io.ReadAll(f); err == nil {
			fmt.Println("\nFile:\n" + string(b))
		}
		f.Close()
	} else {
		fmt.Printf("open error %v\n", err)
	}

	token := windows.GetCurrentProcessToken()
	// if err := winio.EnableProcessPrivileges(privs); err != nil {
	// 	return fmt.Errorf("enable process privileges: %w", err)
	// }

	fmt.Println("\nIs Elevated?", winapi.IsElevated())

	fmt.Println("\nPrivileges:")
	pv, err := winapi.GetTokenPrivileges(token)
	if err != nil {
		return fmt.Errorf("get token privileges: %w", err)
	}

	for _, o := range pv.AllPrivileges() {
		n, err := winapi.LookupPrivilegeName(o.Luid)
		if err != nil {
			continue
		}
		d, err := winapi.LookupPrivilegeDisplayName(n)
		if err != nil {
			continue
		}
		fmt.Printf("%-32s %-48s [%d]\n", n+":", d, o.Attributes)
	}

	fmt.Println("\nEnvironment:")
	for _, e := range os.Environ() {
		fmt.Println(e)
		// vs := strings.Split(e, "=")
		// fmt.Printf("%q,\n", vs[0])
	}

	// fmt.Println("\nSIDs:")

	// cap := winapi.SeChangeNotifyPrivilege // "lpacCom"
	// gs, ss, err := winapi.DeriveCapabilitySIDsFromName(cap)
	// fmt.Println(cap)
	// fmt.Printf("%v\n%v\n%v", gs, ss, err)

	log.G(c.Context).Info("successfully re-exec'ed")

	return nil
}
