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

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/exec"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/security"
	"github.com/Microsoft/hcsshim/internal/winapi"
)

type reExec interface {
	Start() error
	Wait() error
	Run() error
}

// reExecOpts are options to change how a subcommand is re-exec'ed
type reExecConfig struct {
	lpac  bool
	privs []string
	env   []string
	pipes *ioPipes
}

func (c *reExecConfig) cmd(ctx *cli.Context) (reExec, func(), error) {
	if c.lpac {
		return c.cmdLPAC(ctx)
	}
	return c.cmdToken(ctx)
}

func (c *reExecConfig) cmdLPAC(ctx *cli.Context) (reExec, func(), error) {
	c.updateEnvWithTracing(ctx.Context)

	g, err := guid.NewV5(guid.FromArray([16]byte{}), []byte(appName))
	ba := g.ToArray()
	b := unsafe.Slice((*uint32)(unsafe.Pointer(&ba[0])), 4)
	if err != nil {
		return nil, nil, fmt.Errorf("creat capability guid: %w", err)
	}
	fmt.Println("guid:", g)
	appCapSID := &windows.SID{}
	if err := windows.AllocateAndInitializeSid(
		&winapi.SECURITY_APP_PACKAGE_AUTHORITY,
		5,
		winapi.SECURITY_CAPABILITY_BASE_RID,
		b[0],
		b[1],
		b[2],
		b[3],
		0, 0, 0,
		&appCapSID,
	); err != nil {
		return nil, nil, fmt.Errorf("allocate capability SID: %w", err)
	}
	fmt.Println("cap sid is:", appCapSID.String())

	caps := make([]windows.SIDAndAttributes, 0, len(c.privs)+1)
	for _, p := range c.privs {
		_, ss, err := winapi.DeriveCapabilitySIDsFromName(p)
		// l, err := winapi.LookupPrivilegeValue(p)
		if err != nil {
			log.G(ctx.Context).WithError(err).Warningf("could not lookup privilege %q", p)
			continue
		}
		if len(ss) == 0 {
			log.G(ctx.Context).Warningf("did not receive a SID for privilege %q", p)
			continue
		}

		s := windows.SIDAndAttributes{
			Sid:        ss[0],
			Attributes: winapi.SE_GROUP_ENABLED_BY_DEFAULT | winapi.SE_GROUP_ENABLED,
		}
		caps = append(caps, s)
	}
	caps = append(caps, windows.SIDAndAttributes{
		Sid:        appCapSID,
		Attributes: winapi.SE_GROUP_ENABLED_BY_DEFAULT | winapi.SE_GROUP_ENABLED,
	})

	if err := winapi.DeleteAppContainerProfile(ctx.App.Name); err != nil {
		return nil, nil, fmt.Errorf("delete prior app container profile: %w", err)
	}
	sid, err := winapi.CreateAppContainerProfile(ctx.App.Name, ctx.App.Name, ctx.App.Usage, caps)
	if err != nil {
		return nil, nil, fmt.Errorf("create app container profile: %w", err)
	}
	fmt.Println("sid is:", sid.String())

	// sidc, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	// sidc, err := windows.CreateWellKnownSid(windows.WinCreatorOwnerRightsSid)
	// if err != nil {
	// 	return nil, nil, fmt.Errorf("create well know SID for creator group: %w", err)
	// }

	h, err := createTemp()
	if err != nil {
		fmt.Println("could not create temp", err)
	}
	err = security.UpdateHandleDACL(h,
		[]windows.EXPLICIT_ACCESS{
			security.AllowAccessForSID(sid, windows.GENERIC_ALL, windows.NO_INHERITANCE),
			// security.AllowAccessForSID(appCapSID, windows.GENERIC_ALL, windows.NO_INHERITANCE),
		},
		windows.SE_FILE_OBJECT)
	fmt.Println("update acls", err)
	readSD(h)
	windows.Close(h)
	// c.pipes, err = newIOPipes(sd)
	// if err != nil {
	// 	return nil, nil, fmt.Errorf("could not create io pipes: %w", err)
	// }

	attrs, err := windows.NewProcThreadAttributeList(7)
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
	if err := attrs.Update(winapi.PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES, unsafe.Pointer(&sc), unsafe.Sizeof(sc)); err != nil {
		return nil, nil, fmt.Errorf("updating process attributes with security capabilities: %w", err)
	}

	mitigation := winapi.PROCESS_CREATION_MITIGATION_POLICY_DEP_ENABLE | // enable data execution prevention (DEP)
		winapi.PROCESS_CREATION_MITIGATION_POLICY_SEHOP_ENABLE | // block exploits that use the structured exception handler (SEH) overwrite techniques
		winapi.PROCESS_CREATION_MITIGATION_POLICY_FORCE_RELOCATE_IMAGES_ALWAYS_ON_REQ_RELOCS | // images that do not have a base relocation section will not be loaded
		winapi.PROCESS_CREATION_MITIGATION_POLICY_HEAP_TERMINATE_ALWAYS_ON | // heap will terminate if it becomes corrupt
		winapi.PROCESS_CREATION_MITIGATION_POLICY_BOTTOM_UP_ASLR_ALWAYS_ON | // bottom-up randomization policy
		winapi.PROCESS_CREATION_MITIGATION_POLICY_HIGH_ENTROPY_ASLR_ALWAYS_ON | // up to 1TB of bottom-up variance to be used
		winapi.PROCESS_CREATION_MITIGATION_POLICY_STRICT_HANDLE_CHECKS_ALWAYS_ON //  exception raised immediately on a bad handle reference
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_MITIGATION_POLICY, unsafe.Pointer(&mitigation), unsafe.Sizeof(mitigation)); err != nil {
		return nil, nil, fmt.Errorf("updating process attributes with mitigation policy: %w", err)
	}

	// prevent from creating child processes
	child := winapi.PROCESS_CREATION_CHILD_PROCESS_RESTRICTED
	if err := attrs.Update(winapi.PROC_THREAD_ATTRIBUTE_CHILD_PROCESS_POLICY, unsafe.Pointer(&child), unsafe.Sizeof(child)); err != nil {
		return nil, nil, fmt.Errorf("updating process attributes with child process policy: %w", err)
	}

	// enable less privileged app container
	lpac := winapi.PROCESS_CREATION_ALL_APPLICATION_PACKAGES_OPT_OUT
	if err := attrs.Update(winapi.PROC_THREAD_ATTRIBUTE_ALL_APPLICATION_PACKAGES_POLICY, unsafe.Pointer(&lpac), unsafe.Sizeof(lpac)); err != nil {
		return nil, nil, fmt.Errorf("updating process attributes with mitigation policy: %w", err)
	}

	// sd, err := windows.GetNamedSecurityInfo("stdout", windows.SE_OBJECT_TYPE(windows.SE_FILE_OBJECT), windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION))
	// sd, err := windows.GetSecurityInfo(windows.Handle(c.pipes.proc[1].Fd()), windows.SE_OBJECT_TYPE(windows.SE_FILE_OBJECT), windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION))
	// if err != nil {
	// 	fmt.Printf("could not get security description: %v\n", err)
	// } else {
	// 	fmt.Printf("%v", sd)
	// }

	path, args := c.cmdLine()
	// the command line is passed directly to create process, so (unlike "os/exec"), the path
	// is not automatically appended as the first argument
	args = append([]string{path}, args...)
	opts := []exec.ExecOpts{
		// exec.UsingStdio(c.pipes.proc[0], c.pipes.proc[1], c.pipes.proc[2]),
		exec.UsingStdio(os.Stdin, os.Stdout, os.Stderr),
		// exec.WithStdio(true, true, true),
		exec.WithEnv(c.env),
		exec.WithProcessAttributes(attrs),
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
	cleanup := func() {
		// c.pipes.Close()
		// close stdin to stop IO pipe copy from Stdin (which may be blocked on read)
		// os.Stdin.Close()
	}

	return cmd, cleanup, nil
}

const filename = `C:\Users\hamzaelsaawy\Downloads\temp.txt`

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
	readSD(h)
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
func openTemp() (*os.File, error) {
	h, err := openTemph()
	return os.NewFile(uintptr(h), filename), err
}

func readSD(h windows.Handle) {
	sd, err := windows.GetSecurityInfo(h,
		windows.SE_OBJECT_TYPE(windows.SE_FILE_OBJECT),
		windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION))
	if err != nil {
		fmt.Printf("could not get security description: %v\n", err)
	} else {
		fmt.Printf("security descriptor: %v\n", sd)
	}
}

func (c *reExecConfig) cmdToken(ctx *cli.Context) (reExec, func(), error) {
	c.updateEnvWithTracing(ctx.Context)
	createTemp()
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
