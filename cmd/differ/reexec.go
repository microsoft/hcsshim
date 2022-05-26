//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
	"golang.org/x/sys/execabs"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/exec"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/security"
	"github.com/Microsoft/hcsshim/internal/winapi"
)

// add privliges via restricted token?

type reExec interface {
	Start() error
	Wait() error
	Run() error
}

// reExecOpts are options to change how a subcommand is re-exec'ed
type reExecConfig struct {
	ac    bool
	lpac  bool
	caps  []string
	privs []string
	env   []string
}

func (c *reExecConfig) cmd(ctx *cli.Context) (reExec, func(), error) {
	h, err := createTemp()
	if err != nil {
		fmt.Println("could not create temp", err)
	}
	defer windows.Close(h)

	if c.ac {
		return c.cmdAppContainer(ctx)
	}
	return c.cmdToken(ctx)
}

func (c *reExecConfig) cmdAppContainer(ctx *cli.Context) (reExec, func(), error) {
	log.G(ctx.Context).WithField("LPAC", c.lpac).Info("using app containers")

	caps := make([]windows.SIDAndAttributes, 1, len(c.caps)+1)
	capAttribs := winapi.SE_GROUP_ENABLED_BY_DEFAULT | winapi.SE_GROUP_ENABLED

	appCapSID, err := winapi.GetAppAuthoritySIDFromName(appCapabilityName)
	if err != nil {
		return nil, nil, err
	}
	log.G(ctx.Context).WithField("sid", appCapSID.String()).Debug("created app container capability SID")
	// appCapNTSID, err := winapi.GetGroupSIDFromName(appCapabilityName)
	// if err != nil {
	// 	return nil, nil, err
	// }

	caps[0] = windows.SIDAndAttributes{
		Sid:        appCapSID,
		Attributes: capAttribs,
	}
	log.G(ctx.Context).WithField("capabilities", c.caps).Debug("adding other app container capabilities")
	for _, c := range c.caps {
		ss, err := winapi.GetAppAuthoritySIDFromName(c)
		if err != nil {
			log.G(ctx.Context).WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"capability":    c,
			}).Warning("could not get capability SID")
			continue
		}

		sa := windows.SIDAndAttributes{
			Sid:        ss,
			Attributes: capAttribs,
		}
		caps = append(caps, sa)
	}

	if err := winapi.DeleteAppContainerProfile(ctx.App.Name); err != nil {
		return nil, nil, fmt.Errorf("delete prior app container profile: %w", err)
	}
	sid, err := winapi.CreateAppContainerProfile(ctx.App.Name, ctx.App.Name, ctx.App.Usage, caps)
	if err != nil {
		return nil, nil, fmt.Errorf("create app container profile: %w", err)
	}
	fmt.Println("sid is:", sid.String())

	// lsaH, err := winapi.LSAOpenPolicy(winapi.LSA_POLICY_READ | winapi.LSA_POLICY_WRITE)
	// if err != nil {
	// 	fmt.Println("could not open lsa h", err)
	// }
	// defer winapi.LSAClose(lsaH)

	// fmt.Println("")
	// for _, p := range c.privs {
	// 	ps := []string{p}
	// 	if err = winapi.LSAAddAccountRightsString(lsaH, sid, ps); err != nil {
	// 		fmt.Printf("could not add privileges %q: %v\n", p, err)
	// 	} else {
	// 		fmt.Println("added priv", p)
	// 	}
	// }
	// fmt.Println("")

	err = security.GrantSIDFileAccess(filename, appCapSID, windows.GENERIC_ALL)
	fmt.Println("update acls", err)
	readSD(filename)

	attrs, err := windows.NewProcThreadAttributeList(10)
	if err != nil {
		return nil, nil, fmt.Errorf("create process attribute list: %w", err)
	}

	sc := winapi.SecurityCapabilities{
		AppContainerSid: sid,
		CapabilityCount: uint32(len(caps)),
	}
	if len(caps) > 0 {
		sc.Capabilities = &caps[0]
	}
	if err := attrs.Update(
		winapi.PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES,
		unsafe.Pointer(&sc),
		unsafe.Sizeof(sc),
	); err != nil {
		return nil, nil, fmt.Errorf("updating process attributes with security capabilities: %w", err)
	}

	mitigation := winapi.PROCESS_CREATION_MITIGATION_POLICY_DEP_ENABLE | // enable data execution prevention (DEP)
		winapi.PROCESS_CREATION_MITIGATION_POLICY_SEHOP_ENABLE | // block exploits that use the structured exception handler (SEH) overwrite techniques
		winapi.PROCESS_CREATION_MITIGATION_POLICY_FORCE_RELOCATE_IMAGES_ALWAYS_ON_REQ_RELOCS | // images that do not have a base relocation section will not be loaded
		winapi.PROCESS_CREATION_MITIGATION_POLICY_HEAP_TERMINATE_ALWAYS_ON | // heap will terminate if it becomes corrupt
		winapi.PROCESS_CREATION_MITIGATION_POLICY_BOTTOM_UP_ASLR_ALWAYS_ON | // bottom-up randomization policy
		winapi.PROCESS_CREATION_MITIGATION_POLICY_HIGH_ENTROPY_ASLR_ALWAYS_ON | // up to 1TB of bottom-up variance to be used
		winapi.PROCESS_CREATION_MITIGATION_POLICY_STRICT_HANDLE_CHECKS_ALWAYS_ON //  exception raised immediately on a bad handle reference
	if err := attrs.Update(
		windows.PROC_THREAD_ATTRIBUTE_MITIGATION_POLICY,
		unsafe.Pointer(&mitigation),
		unsafe.Sizeof(mitigation),
	); err != nil {
		return nil, nil, fmt.Errorf("updating process attributes with mitigation policy: %w", err)
	}

	// prevent from creating child processes
	child := winapi.PROCESS_CREATION_CHILD_PROCESS_RESTRICTED
	if err := attrs.Update(
		winapi.PROC_THREAD_ATTRIBUTE_CHILD_PROCESS_POLICY,
		unsafe.Pointer(&child),
		unsafe.Sizeof(child),
	); err != nil {
		return nil, nil, fmt.Errorf("updating process attributes with child process policy: %w", err)
	}

	// create process outside desktop environment
	desktop := uint32(winapi.PROCESS_CREATION_DESKTOP_APP_BREAKAWAY_ENABLE_PROCESS_TREE)
	if err := attrs.Update(
		winapi.PROC_THREAD_ATTRIBUTE_DESKTOP_APP_POLICY,
		unsafe.Pointer(&desktop),
		unsafe.Sizeof(desktop),
	); err != nil {
		return nil, nil, fmt.Errorf("updating process attributes with desktop policy: %w", err)
	}

	// enable less privileged app container
	if c.lpac {
		lpac := winapi.PROCESS_CREATION_ALL_APPLICATION_PACKAGES_OPT_OUT
		if err := attrs.Update(
			winapi.PROC_THREAD_ATTRIBUTE_ALL_APPLICATION_PACKAGES_POLICY,
			unsafe.Pointer(&lpac),
			unsafe.Sizeof(lpac),
		); err != nil {
			return nil, nil, fmt.Errorf("updating process attributes with mitigation policy: %w", err)
		}
	}

	token := windows.GetCurrentProcessToken()

	// token, err := restrictedToken(c.privs)
	// if err != nil {
	// 	return nil, nil, err
	// }
	pv, err := winapi.GetTokenPrivileges(token)
	if err != nil {
		return nil, nil, fmt.Errorf("get token privileges: %w", err)
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
		fmt.Printf("rt -> %-32s %-48s [%d]\n", n+":", d, o.Attributes)
	}

	path, args := c.cmdLine()
	// the command line is passed directly to create process, so (unlike "os/exec"), the path
	// is not automatically appended as the first argument
	args = append([]string{path}, args...)
	opts := []exec.ExecOpts{
		exec.UsingStdio(os.Stdin, os.Stdout, os.Stderr),
		exec.WithEnv(c.env),
		exec.WithProcessAttributes(attrs),
		exec.WithToken(token),
	}

	cmd, err := exec.New(path, strings.Join(args, " "), opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create hcsshim exec: %w", err)
	}

	log.G(ctx.Context).WithFields(logrus.Fields{
		"path": path,
		"args": args,
		"env":  c.env,
	}).Debug("re-execing command")
	cleanup := func() {}

	return cmd, cleanup, nil
}

const filename = `C:\Users\hamzaelsaawy\Downloads\temp.txt`
const filename2 = `C:\Users\hamzaelsaawy\Downloads\t\temp.txt`
const dirname = `C:\Users\hamzaelsaawy\Downloads\t`

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

func readSD(s string) {
	sd, err := security.GetFileSD(s)
	if err != nil {
		fmt.Printf("could not get %q security description: %v\n", s, err)
	} else {
		fmt.Printf("security descriptor: %v\n", sd)
	}
}

func (c *reExecConfig) cmdToken(ctx *cli.Context) (reExec, func(), error) {
	path, args := c.cmdLine()

	log.G(ctx.Context).WithFields(logrus.Fields{
		"path":       path,
		"args":       args,
		"env":        c.env,
		"privileges": c.privs,
	}).Debug("re-execing command")

	token, err := restrictedToken(c.privs)
	if err != nil {
		return nil, nil, err
	}
	f := func() {
		token.Close()
	}

	cmd := execabs.CommandContext(ctx.Context, path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = c.env
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		Token:      syscall.Token(token),
	}

	return cmd, f, nil
}

func (c *reExecConfig) cmdLine() (path string, args []string) {
	path = os.Args[0]
	args = append([]string{"-" + reExecFlagName}, os.Args[1:]...)
	return
}

func (c *reExecConfig) updateEnvWithTracing(ctx context.Context) {
	if sc, ok := spanContextToString(ctx); ok {
		c.env = append(c.env, spanContextEnvVar+"="+sc)
	}
}
