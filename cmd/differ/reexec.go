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
}

func (c *reExecConfig) cmd(ctx *cli.Context) (reExec, func(), error) {
	if c.lpac {
		return c.cmdLPAC(ctx)
	}
	return c.cmdToken(ctx)
}

func (c *reExecConfig) cmdLPAC(ctx *cli.Context) (reExec, func(), error) {
	c.updateEnvWithTracing(ctx.Context)

	var caps []windows.SIDAndAttributes
	sid, err := winapi.CreateAppContainerProfile(ctx.App.Name, ctx.App.Name, ctx.App.Usage, caps)
	if err != nil {
		return nil, nil, fmt.Errorf("create app container profile: %w", err)
	}

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

	mitigation := winapi.PROCESS_CREATION_MITIGATION_POLICY_DEP_ENABLE |
		winapi.PROCESS_CREATION_MITIGATION_POLICY_SEHOP_ENABLE |
		winapi.PROCESS_CREATION_MITIGATION_POLICY_FORCE_RELOCATE_IMAGES_ALWAYS_ON_REQ_RELOCS |
		winapi.PROCESS_CREATION_MITIGATION_POLICY_HEAP_TERMINATE_ALWAYS_ON |
		winapi.PROCESS_CREATION_MITIGATION_POLICY_BOTTOM_UP_ASLR_ALWAYS_ON |
		winapi.PROCESS_CREATION_MITIGATION_POLICY_HIGH_ENTROPY_ASLR_ALWAYS_ON |
		winapi.PROCESS_CREATION_MITIGATION_POLICY_STRICT_HANDLE_CHECKS_ALWAYS_ON
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_MITIGATION_POLICY, unsafe.Pointer(&mitigation), unsafe.Sizeof(mitigation)); err != nil {
		return nil, nil, fmt.Errorf("updating process attributes with mitigation policy: %w", err)
	}

	child := winapi.PROCESS_CREATION_CHILD_PROCESS_RESTRICTED
	if err := attrs.Update(winapi.PROC_THREAD_ATTRIBUTE_CHILD_PROCESS_POLICY, unsafe.Pointer(&child), unsafe.Sizeof(child)); err != nil {
		return nil, nil, fmt.Errorf("updating process attributes with child process policy: %w", err)
	}

	lpac := winapi.PROCESS_CREATION_ALL_APPLICATION_PACKAGES_OPT_OUT
	if err := attrs.Update(winapi.PROC_THREAD_ATTRIBUTE_ALL_APPLICATION_PACKAGES_POLICY, unsafe.Pointer(&lpac), unsafe.Sizeof(lpac)); err != nil {
		return nil, nil, fmt.Errorf("updating process attributes with mitigation policy: %w", err)
	}

	path, args := c.cmdLine()
	opts := []exec.ExecOpts{
		exec.UsingStdio(os.Stdin, os.Stdout, os.Stderr),
		exec.WithEnv(c.env),
		exec.WithProcessAttributes(attrs),
	}
	cmd, err := exec.New(path, strings.Join(append([]string{path}, args...), " "), opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create hcsshim exec: %w", err)
	}

	log.G(ctx.Context).WithFields(logrus.Fields{
		"path": path,
		"args": args,
		"cmd":  cmd.String(),
		"env":  c.env,
	}).Debug("re-execing command")

	return cmd, func() {}, nil
}

func (c *reExecConfig) cmdToken(ctx *cli.Context) (reExec, func(), error) {
	c.updateEnvWithTracing(ctx.Context)
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
