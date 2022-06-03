//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unsafe"

	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/exec"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/winapi"
)

// add privliges via restricted token?

// reExecOpts are options to change how a subcommand is re-exec'ed
type reExecConfig struct {
	ac    bool
	lpac  bool
	caps  []string
	privs []string
	env   []string
}

func (c *reExecConfig) cmd(ctx *cli.Context, sids []*windows.SID, defaultOpts ...exec.ExecOpts) (cmd *exec.Exec, cleanup func(), err error) {
	var opts []exec.ExecOpts
	if c.ac {
		opts, cleanup, err = c.cmdAppContainer(ctx, sids)
	} else {
		opts, cleanup, err = c.cmdToken(ctx, sids)
	}
	if cleanup == nil {
		cleanup = func() {}
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()
	if err != nil {
		return nil, cleanup, err
	}

	// add default StdIO and environment opts
	opts = append(defaultOpts, opts...)
	path, args := c.cmdLine()
	cmd, err = exec.New(path, strings.Join(args, " "), opts...)
	if err != nil {
		return nil, cleanup, err
	}

	log.G(ctx.Context).WithFields(logrus.Fields{
		"path": path,
		"args": args,
		"env":  c.env,
	}).Info("Created re-exec command")
	return cmd, cleanup, nil
}

func (c *reExecConfig) cmdAppContainer(ctx *cli.Context, acSIDs []*windows.SID) ([]exec.ExecOpts, func(), error) {
	log.G(ctx.Context).WithField("LPAC", c.lpac).Info("Using AppContainers")

	caps := make([]windows.SIDAndAttributes, 0, len(c.caps)+len(acSIDs))
	capAttribs := winapi.SE_GROUP_ENABLED_BY_DEFAULT | winapi.SE_GROUP_ENABLED
	for _, sid := range acSIDs {
		caps = append(caps, windows.SIDAndAttributes{
			Sid:        sid,
			Attributes: capAttribs,
		})
	}
	log.G(ctx.Context).WithField("capabilities", c.caps).Debug("Adding AppContainer capabilities")
	for _, c := range c.caps {
		ss, err := winapi.GetAppAuthoritySIDFromName(c)
		if err != nil {
			log.G(ctx.Context).WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"capability":    c,
			}).Warning("Could not get capability SID")
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
	log.G(ctx.Context).WithFields(logrus.Fields{
		"sid": sid.String(),
	}).Debug("Created AppContainer SID")

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

	opts := []exec.ExecOpts{
		exec.WithProcessAttributes(attrs),
	}
	return opts, nil, nil
}

func (c *reExecConfig) cmdToken(ctx *cli.Context, sids []*windows.SID) (_ []exec.ExecOpts, cleanup func(), err error) {
	pToken, err := winapi.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_DUPLICATE|windows.TOKEN_ASSIGN_PRIMARY|windows.TOKEN_QUERY)
	if err != nil {
		return nil, nil, fmt.Errorf("open process token: %w", err)
	}
	defer pToken.Close()

	deleteLUIDs, err := privilegesToDelete(pToken, c.privs)
	if err != nil {
		return nil, nil, fmt.Errorf("get privileges to delete: %w", err)
	}

	restrictSIDs := make([]windows.SIDAndAttributes, 0, len(sids))
	for _, sid := range sids {
		restrictSIDs = append(restrictSIDs, windows.SIDAndAttributes{
			Sid: sid,
		})
	}
	// sidc, err := windows.CreateWellKnownSid(windows.WinLocalSid)
	// if err != nil {
	// 	return nil, nil, fmt.Errorf("create well know SID for creator group: %w", err)
	// }
	// fmt.Println("csid", sidc)
	// restrictSIDs = append(restrictSIDs, windows.SIDAndAttributes{
	// 	Sid: sidc,
	// })
	token, err := winapi.CreateRestrictedToken(
		pToken,
		winapi.TOKEN_WRITE_RESTRICTED,
		nil, // SIDs to disable
		deleteLUIDs,
		// nil,
		restrictSIDs,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create restricted token: %w", err)
	}
	cleanup = func() {
		_ = token.Close()
	}

	opts := []exec.ExecOpts{
		exec.WithToken(token),
	}
	return opts, cleanup, nil
}

func (c *reExecConfig) pipeSecurityDescriptor(sids []*windows.SID, access windows.ACCESS_MASK) (*windows.SECURITY_DESCRIPTOR, error) {
	// AppContainers can inherit the pipe handles propely, but the restricting SID in the restricted token
	// must be explicitly added to the pipes
	if c.ac {
		return nil, nil
	}

	sd, err := windows.NewSecurityDescriptor()
	if err != nil {
		return nil, err
	}
	sidc, err := windows.CreateWellKnownSid(windows.WinCreatorOwnerRightsSid)
	if err != nil {
		return nil, fmt.Errorf("create well know SID for creator group: %w", err)
	}

	eas := make([]windows.EXPLICIT_ACCESS, 0, len(sids)+1)
	// var dacl *windows.ACL
	for _, sid := range append(sids, sidc) {
		eas = append(eas, winapi.AllowAccessForSID(sid, access, windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT))
	}

	dacl, err := windows.ACLFromEntries(eas, nil)
	if err != nil {
		return nil, fmt.Errorf("create DACL from explicit accesses: %w", err)
	}
	err = sd.SetDACL(dacl, true, false)
	return sd, err
}

func (c *reExecConfig) capabilitySIDs() ([]*windows.SID, error) {
	sids, acSIDs, err := winapi.DeriveCapabilitySIDsFromName(appCapabilityName)
	if c.ac {
		sids = acSIDs
	}
	return sids, err
}

// cmdLine adds the re-exec flag to the command line arguments
func (c *reExecConfig) cmdLine() (string, []string) {
	path := os.Args[0]
	args := os.Args[1:]
	// the command line is passed directly to create process, so (unlike "os/exec"), the path
	// is not automatically appended as the first argument
	args = append([]string{path, "-" + reExecFlagName}, args...)
	return path, args
}

func (c *reExecConfig) updateEnvWithTracing(ctx context.Context) {
	if sc, ok := spanContextToString(ctx); ok {
		c.env = append(c.env, spanContextEnvVar+"="+sc)
	}
}
