//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/jobcontainers"
	"github.com/Microsoft/hcsshim/internal/searchexe"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
)

const hpcCmdName = "hpc"

var hpcCommand = cli.Command{
	Name:           hpcCmdName,
	Hidden:         true,
	SkipArgReorder: true,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "hpc-pipe",
			Usage: "named pipe to grab information about the process to launch from",
		},
	},
	Action: func(clictx *cli.Context) (err error) {
		// This command is to handle a special case for HostProcess containers because of some silo and bind mount interactions. For HostProcess
		// containers the containers rootfs is bind mounted to a static path in the container (default of C:\hpc) and is unique per container.
		// The contents of this path aren't viewable from outside of the container, and as we launch the containers processes via CreateProcess
		// on the host, we can't actually launch C:\hpc\path\to\binary.exe as we'll fail to find the file. Once a process is associated with a
		// job it can't be disassociated (at least in usermode) so temporarily joining, launching the process, and then leaving isn't feasible.
		//
		// An alternative to this command would be invoking 'cmd /c' or the powershell equivalent and having them launch the process, but this
		// gives us finer grained control over how the process is launched and managed.
		p, err := winio.DialPipe(clictx.String("hpc-pipe"), nil)
		if err != nil {
			return err
		}

		cmd, err := containerSetup(p)
		if err != nil {
			return err
		}

		err = cmd.Run()
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		// Exit with the exit code of the containers process we launched. Even passing a nil error for the message param has
		// it format it as "<nil>" which is a bit odd when exiting an exec for example. You'd expect to just cleanly return to
		// your hosts terminal but get one line of "<nil>" which is unneccessary, so pass an empty string explicitly if we don't
		// get nil.
		return cli.NewExitError(errMsg, cmd.ProcessState.ExitCode())
	},
}

func containerSetup(c net.Conn) (_ *exec.Cmd, err error) {
	var lpo jobcontainers.LaunchProcessOptions
	if err := json.NewDecoder(c).Decode(&lpo); err != nil {
		return nil, err
	}

	enc := json.NewEncoder(c)
	defer func() {
		var errMsg string
		if err != nil {
			errMsg = err.Error()
		}
		// Send this to the parent so they know the status of the setup.
		_ = enc.Encode(&jobcontainers.ProcStartError{Err: errMsg})
	}()

	// If this is the init process, lets setup the mounts under the containers rootfs. Execs typically
	// can't specify additional mounts, so avoid remounting.
	if lpo.Init {
		// This is to make things backwards compatible with the approach on machines that don't have the bind filter
		// API available. On those machines, mounts would be placed under a relative path in the containers rootfs.
		// This should make an upgrade to WS2022, or just an older patch of WS2019 much easier with no need to rebuild
		// container images.
		if err := setupMounts(lpo.ContainerRootfs, lpo.Mounts); err != nil {
			return nil, err
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	appName, cmdLine, err := searchexe.GetApplicationName(lpo.CommandLine, wd, os.Getenv("PATH"))
	if err != nil {
		return nil, fmt.Errorf("failed to get application name from commandline %q: %w", lpo.CommandLine, err)
	}

	cmd := &exec.Cmd{
		Path: appName,
		SysProcAttr: &syscall.SysProcAttr{
			CmdLine: cmdLine,
		},
	}
	if lpo.In {
		cmd.Stdin = os.Stdin
	}
	if lpo.Out {
		cmd.Stdout = os.Stdout
	}
	if lpo.Err {
		cmd.Stderr = os.Stderr
	}

	// Send ctrl-c to the void if we have a pseudo console.
	if lpo.Tty {
		go func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt)
			defer signal.Stop(sigChan)

			for {
				select {
				case <-sigChan:
				}
			}
		}()
	}

	return cmd, nil
}

// Strip the drive letter (if there is one) so we don't end up with "%CONTAINER_SANDBOX_MOUNT_POINT%"\C:\path\to\mount
func stripDriveLetter(name string) string {
	// Remove drive letter
	if len(name) == 2 && name[1] == ':' {
		name = "."
	} else if len(name) > 2 && name[1] == ':' {
		name = name[2:]
	}
	return name
}

func setupMounts(rootfs string, mounts []specs.Mount) error {
	for _, mount := range mounts {
		if mount.Destination == "" || mount.Source == "" {
			return fmt.Errorf("invalid OCI spec - a mount must have both source and a destination: %+v", mount)
		}

		// For backwards compat with how mounts worked without the bind filter, additionally plop the directory/file
		// to a relative path inside the containers rootfs.
		fullCtrPath := filepath.Join(rootfs, stripDriveLetter(mount.Destination))
		// Make sure all of the dirs leading up to the full path exist.
		strippedCtrPath := filepath.Dir(fullCtrPath)
		if err := os.MkdirAll(strippedCtrPath, 0777); err != nil {
			return fmt.Errorf("failed to make directory for job container mount: %w", err)
		}

		// Best effort for the backwards compat mounts.
		_ = os.Symlink(mount.Source, fullCtrPath)
	}
	return nil
}
