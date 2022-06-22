//go:build windows

package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/jobcontainers"
	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/Microsoft/hcsshim/internal/searchexe"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const hpcCmdName = "hpc"

var hpcCommand = cli.Command{
	Name:           hpcCmdName,
	Hidden:         true,
	SkipArgReorder: true,

	Action: func(clictx *cli.Context) (err error) {
		// This command is to handle a special case for HostProcess containers because of some silo and bind mount interactions. For HostProcess
		// containers the containers rootfs is bind mounted to a static path in the container (default of C:\hpc) and is unique per container.
		// The contents of this path aren't viewable from outside of the container, and as we launch the containers processes via CreateProcess
		// on the host, we can't actually launch C:\hpc\path\to\binary.exe as we'll fail to find the file. Once a process is associated with a
		// job it can't be disassociated (at least in usermode) so temporarily joining, launching the process, and then leaving isn't feasible.
		// An alternative to this command would be invoking 'cmd /c' or the powershell equivalent and having them launch the process, but this
		// gives us finer grained control over how the process is launched and managed.
		workDir, err := os.Getwd()
		if err != nil {
			return err
		}

		f, err := os.Open(filepath.Join(workDir, "hpcopts"))
		if err != nil {
			return err
		}

		var lpo jobcontainers.LaunchProcessOptions
		if err := gob.NewDecoder(f).Decode(&lpo); err != nil {
			return err
		}

		ctx := context.Background()
		job, err := jobobject.Open(ctx, &jobobject.Options{Name: lpo.SiloName})
		if err != nil {
			return err
		}

		// If this is the init process, lets setup the mounts under the containers rootfs. Execs typically
		// can't specify additional mounts, so avoid rebinding.
		if lpo.Init {
			// This is to make things backwards compatible with the approach on machines that don't have the bind filter
			// API available. On those machines, mounts would be placed under a relative path in the containers rootfs.
			// This should make an upgrade to WS2022, or just an older patch of WS2019 much easier with no need to rebuild
			// container images.
			if err := setupMounts(lpo.ContainerRootfs, job, lpo.Mounts); err != nil {
				return err
			}
		}

		if err := job.Assign(uint32(os.Getpid())); err != nil {
			return err
		}

		if err := os.MkdirAll(lpo.WorkingDirectory, 0700); err != nil {
			return err
		}

		appName, cmdLine, err := searchexe.GetApplicationName(lpo.CommandLine, lpo.WorkingDirectory, os.Getenv("PATH"))
		if err != nil {
			return fmt.Errorf("failed to get application name from commandline %q: %w", lpo.CommandLine, err)
		}

		// exec.Cmd internally does its own path resolution and as part of this checks some well known file extensions on the file given (e.g. if
		// the user just provided /path/to/mybinary). CreateProcess is perfectly capable of launching an executable that doesn't have the .exe extension
		// so this adds an empty string entry to the end of what extensions GO checks against so that a binary with no extension can be launched.
		// The extensions are checked in order, so that if mybinary.exe and mybinary both existed in the same directory, mybinary.exe would be chosen.
		// This is mostly to handle a common Kubernetes test image named agnhost that has the main entrypoint as a binary named agnhost with no extension.
		// https://github.com/kubernetes/kubernetes/blob/d64e91878517b1208a0bce7e2b7944645ace8ede/test/images/agnhost/Dockerfile_windows
		if err := os.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD; "); err != nil {
			return fmt.Errorf("failed to set PATHEXT: %w", err)
		}

		cmd := &exec.Cmd{
			Path: appName,
			Env:  lpo.Env,
			Dir:  lpo.WorkingDirectory,
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
			sigChan := make(chan os.Signal)
			signal.Notify(sigChan, os.Interrupt)
			defer signal.Stop(sigChan)

			go func() {
				for {
					select {
					case <-sigChan:
					}
				}
			}()
		}

		if err := cmd.Run(); err != nil {
			return err
		}

		// Exit with the exit code of the containers process we launched.
		return cli.NewExitError("", cmd.ProcessState.ExitCode())
	},
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

func setupMounts(rootfs string, job *jobobject.JobObject, mounts []specs.Mount) error {
	for _, mount := range mounts {
		if mount.Destination == "" || mount.Source == "" {
			return fmt.Errorf("invalid OCI spec - a mount must have both source and a destination: %+v", mount)
		}

		fullCtrPath := filepath.Join(rootfs, stripDriveLetter(mount.Destination))
		// Make sure all of the dirs leading up to the full path exist.
		strippedCtrPath := filepath.Dir(fullCtrPath)
		if err := os.MkdirAll(strippedCtrPath, 0777); err != nil {
			return errors.Wrap(err, "failed to make directory for job container mount")
		}

		if err := job.ApplyFileBinding(fullCtrPath, mount.Source, false); err != nil {
			return err
		}
	}
	return nil
}
