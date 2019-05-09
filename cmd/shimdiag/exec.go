package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/containerd/console"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

type rawConReader struct {
	f *os.File
}

func (r rawConReader) Read(b []byte) (int, error) {
	n, err := syscall.Read(syscall.Handle(r.f.Fd()), b)
	if n == 0 && len(b) != 0 && err == nil {
		// A zero-byte read on a console indicates that the user wrote Ctrl-Z.
		b[0] = 26
		return 1, nil
	}
	return n, err
}

var execTty bool
var execCommand = cli.Command{
	Name:      "exec",
	Usage:     "Executes a command in a shim's hosting utility VM",
	ArgsUsage: "<shim name> <command> [args...]",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:        "tty,t",
			Usage:       "run with a terminal",
			Destination: &execTty},
	},
	SkipArgReorder: true,
	Before:         appargs.Validate(appargs.String, appargs.String, appargs.Rest(appargs.String)),
	Action: func(clictx *cli.Context) error {
		args := clictx.Args()
		shim, err := getShim(args[0])
		if err != nil {
			return err
		}

		var osStdin io.Reader = os.Stdin
		if execTty {
			// Enable raw mode on the client's console.
			con, err := console.ConsoleFromFile(os.Stdin)
			if err == nil {
				err = con.SetRaw()
				if err != nil {
					return err
				}
				defer con.Reset()
				// Console reads return EOF whenever the user presses Ctrl-Z.
				// Wrap the reads to translate these EOFs back.
				osStdin = rawConReader{os.Stdin}
			}
		}

		stdin, err := makePipe(osStdin, true)
		if err != nil {
			return err
		}
		stdout, err := makePipe(os.Stdout, false)
		if err != nil {
			return err
		}
		var stderr string
		if !execTty {
			stderr, err = makePipe(os.Stderr, false)
			if err != nil {
				return err
			}
		}
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-ch
			cancel()
		}()
		svc := shimdiag.NewShimDiagClient(shim)
		resp, err := svc.DiagExecInHost(ctx, &shimdiag.ExecProcessRequest{
			Args:     args[1:],
			Stdin:    stdin,
			Stdout:   stdout,
			Stderr:   stderr,
			Terminal: execTty,
		})
		if err != nil {
			return err
		}
		return cli.NewExitError(errors.New(""), int(resp.ExitCode))
	},
}

func makePipe(f interface{}, in bool) (string, error) {
	r, err := guid.NewV4()
	if err != nil {
		return "", err
	}
	p := `\\.\pipe\` + r.String()
	l, err := winio.ListenPipe(p, nil)
	if err != nil {
		return "", err
	}
	go func() {
		c, err := l.Accept()
		if err != nil {
			logrus.WithError(err).Error("failed to accept pipe")
			return
		}
		if in {
			io.Copy(c, f.(io.Reader))
			c.Close()
		} else {
			io.Copy(f.(io.Writer), c)
		}
	}()
	return p, nil
}
