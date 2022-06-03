//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"unsafe"

	"github.com/urfave/cli/v2"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/winapi"
)

var testCommand = &cli.Command{
	Name:    "test",
	Aliases: []string{"t"},
	Usage:   "test command to check re-exec works and privileges are reduced",
	Before:  createCommandBeforeFunc(withPrivileges(privs)),
	Action:  test,
}

func test(c *cli.Context) error {
	log.G(c.Context).Warning("testing re-exec")
	f, err := os.Create(filename)
	if err == nil {
		f.Write([]byte("howdy yall"))
		f.Seek(0, 0)

		if b, err := io.ReadAll(f); err == nil {
			fmt.Println("\nFile:\n" + string(b))
		}
		f.Close()
	} else {
		fmt.Printf("open error %v\n", err)
	}

	token := windows.GetCurrentProcessToken()
	for i := range privs {
		p := privs[i : i+1]
		if err := winio.EnableProcessPrivileges(p); err != nil {
			fmt.Printf("count not enable enable process privileges %q: %v\n", privs[i], err)
		}
	}

	fmt.Println("\nIs Elevated?", winapi.IsElevated())
	b, err := winapi.IsAppContainerToken(token)
	fmt.Println("\nIs AC?", b, err)

	fmt.Println("\nPrivileges:")
	pv, err := winapi.GetTokenPrivileges(token)
	if err != nil {
		return fmt.Errorf("get token privileges: %w", err)
	}

	for _, o := range pv.AllPrivileges() {
		n, err := winapi.LookupPrivilegeName(o.Luid)
		if err != nil {
			fmt.Printf("failed to lookup %v\n", o.Luid)
			continue
		}
		d, err := winapi.LookupPrivilegeDisplayName(n)
		if err != nil {
			fmt.Printf("failed to lookup name %q\n", n)
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

const filename = `C:\Users\hamzaelsaawy\Downloads\t\temp.txt`
const dirname = `C:\Users\hamzaelsaawy\Downloads\t`
const filename2 = `C:\Users\hamzaelsaawy\Downloads\temp.txt`

func createTemp() (windows.Handle, error) {
	if err := os.Remove(filename); err != nil {
		fmt.Println("could not delete temp file", err)
	}
	u16, err := windows.UTF16PtrFromString(filename)
	if err != nil {
		return 0, err
	}
	sa := &windows.SecurityAttributes{
		SecurityDescriptor: nil,
		InheritHandle:      0,
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
	}
	h, err := windows.CreateFile(
		u16,
		windows.READ_CONTROL|windows.GENERIC_READ|windows.GENERIC_WRITE|windows.WRITE_DAC,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		sa,
		windows.CREATE_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return 0, err
	}
	f := os.NewFile(uintptr(h), "t")
	fmt.Fprintf(f, "test test ?")
	readSD(filename)
	return h, nil
}

func openTemph() (windows.Handle, error) {
	u16, err := windows.UTF16PtrFromString(filename)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(
		u16,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return 0, err
	}
	return h, nil
}

func readHSD(h windows.Handle, t windows.SE_OBJECT_TYPE) {
	sd, err := winapi.GetHandleSD(h, t)
	if err != nil {
		fmt.Printf("could not get security description: %v\n", err)
	} else {
		fmt.Printf("security descriptor: %v\n", sd)
	}
}

func readSD(s string) {
	sd, err := winapi.GetFileSD(s)
	if err != nil {
		fmt.Printf("could not get %q security description: %v\n", s, err)
	} else {
		fmt.Printf("security descriptor: %v\n", sd)
	}
}
