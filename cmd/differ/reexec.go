//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"syscall"

	exec "golang.org/x/sys/execabs"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/sirupsen/logrus"
)

type reExec interface {
	Start() error
	Wait() error
	Run() error
}

// reExecOpts are options to change how a subcommand is re-exec'ed
type reExecConfig struct {
	privs []string
	env   []string
}

func (c *reExecConfig) cmd(ctx context.Context) (reExec, func(), error) {
	if sc, ok := spanContextToString(ctx); ok {
		c.env = append(c.env, spanContextEnvVar+"="+sc)
	}
	path, args := c.cmdLine()

	log.G(ctx).WithFields(logrus.Fields{
		"path":       path,
		"args":       args,
		"env":        c.env,
		"privileges": c.privs,
	}).Debug("re-execing command")

	token, err := c.restrictedToken()
	if err != nil {
		return nil, nil, err
	}
	f := func() {
		token.Close()
	}

	cmd := exec.CommandContext(ctx, path, args...)
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
	// args = strings.Join(a, " ")
	return
}

func (c *reExecConfig) restrictedToken() (t windows.Token, err error) {
	var etoken windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_DUPLICATE|windows.TOKEN_ASSIGN_PRIMARY|windows.TOKEN_QUERY|windows.TOKEN_WRITE,
		&etoken,
	); err != nil {
		return t, fmt.Errorf("could not open process token: %w", err)
	}
	defer etoken.Close()

	deleteLUIDs, err := privilegesToDelete(etoken, c.privs)
	if err != nil {
		return t, fmt.Errorf("could not get privileges to delete: %w", err)
	}

	if err := winapi.CreateRestrictedToken(
		etoken,
		0,   // flags
		nil, // SIDs to disable
		deleteLUIDs,
		nil, // SIDs to restrict
		&t,
	); err != nil {
		return t, fmt.Errorf("could not create restricted token: %w", err)
	}

	return t, nil
}

// privilegesToDelete returns a list of all the privleges a token has, except for those
// specified in keep.
//
// The return is a pointer to the first element of a []
func privilegesToDelete(token windows.Token, keep []string) ([]windows.LUIDAndAttributes, error) {
	keepLUIDs := make([]windows.LUID, 0, len(keep))
	for _, p := range keep {
		l, err := winapi.LookupPrivilegeValue(p)
		if err != nil {
			return nil, fmt.Errorf("could not lookup privilege %q: %w", p, err)
		}
		keepLUIDs = append(keepLUIDs, l)
	}

	pv, err := winapi.GetTokenPrivileges(token)
	if err != nil {
		return nil, fmt.Errorf("could not get token privileges: %w", err)
	}

	privs := pv.AllPrivileges()
	privDel := make([]windows.LUIDAndAttributes, 0, len(privs))

	for _, a := range privs {
		if deletePriv(&a, keepLUIDs) {
			privDel = append(privDel, a)
		}
	}

	return privDel, nil
}

func deletePriv(p *windows.LUIDAndAttributes, keep []windows.LUID) bool {
	for _, l := range keep {
		if p.Luid == l {
			return false
		}
	}
	return true
}
