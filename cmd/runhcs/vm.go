//go:build windows

package main

import (
	gcontext "context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/runhcs"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func vmID(id string) string {
	return id + "@vm"
}

var vmshimCommand = cli.Command{
	Name:   "vmshim",
	Usage:  `launch a VM and containers inside it (do not call it outside of runhcs)`,
	Hidden: true,
	Flags: []cli.Flag{
		cli.StringFlag{Name: "log-pipe", Hidden: true},
		cli.StringFlag{Name: "os", Hidden: true},
	},
	Before: appargs.Validate(argID),
	Action: func(context *cli.Context) error {
		logPipe := context.String("log-pipe")
		if logPipe != "" {
			lpc, err := winio.DialPipe(logPipe, nil)
			if err != nil {
				return err
			}
			defer lpc.Close()
			logrus.SetOutput(lpc)
		} else {
			logrus.SetOutput(os.Stderr)
		}
		fatalWriter.Writer = os.Stdout

		pipePath := context.Args().First()

		optsj, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		os.Stdin.Close()

		var opts interface{}
		isLCOW := context.String("os") == "linux"
		if isLCOW {
			opts = &uvm.OptionsLCOW{}
		} else {
			opts = &uvm.OptionsWCOW{}
		}

		err = json.Unmarshal(optsj, opts)
		if err != nil {
			return err
		}

		// Listen on the named pipe associated with this VM.
		l, err := winio.ListenPipe(pipePath, &winio.PipeConfig{MessageMode: true})
		if err != nil {
			return err
		}

		var vm *uvm.UtilityVM
		if isLCOW {
			vm, err = uvm.CreateLCOW(gcontext.Background(), opts.(*uvm.OptionsLCOW))
		} else {
			vm, err = uvm.CreateWCOW(gcontext.Background(), opts.(*uvm.OptionsWCOW))
		}
		if err != nil {
			return err
		}
		defer vm.Close()
		if err = vm.Start(gcontext.Background()); err != nil {
			return err
		}

		// Asynchronously wait for the VM to exit.
		exitCh := make(chan error)
		go func() {
			exitCh <- vm.Wait()
		}()

		defer vm.Close()

		// Alert the parent process that initialization has completed
		// successfully.
		os.Stdout.Write(runhcs.ShimSuccess)
		os.Stdout.Close()
		fatalWriter.Writer = io.Discard

		pipeCh := make(chan net.Conn)
		go func() {
			for {
				conn, err := l.Accept()
				if err != nil {
					logrus.Error(err)
					continue
				}
				pipeCh <- conn
			}
		}()

		for {
			select {
			case <-exitCh:
				return nil
			case pipe := <-pipeCh:
				err = processRequest(vm, pipe)
				if err == nil {
					_, err = pipe.Write(runhcs.ShimSuccess)
					// Wait until the pipe is closed before closing the
					// container so that it is properly handed off to the other
					// process.
					if err == nil {
						err = closeWritePipe(pipe)
					}
					if err == nil {
						_, _ = io.ReadAll(pipe)
					}
				} else {
					logrus.WithError(err).
						Error("failed creating container in VM")
					fmt.Fprintf(pipe, "%v", err)
				}
				pipe.Close()
			}
		}
	},
}

func processRequest(vm *uvm.UtilityVM, pipe net.Conn) error {
	var req runhcs.VMRequest
	err := json.NewDecoder(pipe).Decode(&req)
	if err != nil {
		return err
	}
	logrus.WithFields(logrus.Fields{
		logfields.ContainerID:     req.ID,
		logfields.VMShimOperation: req.Op,
	}).Debug("process request")
	c, err := getContainer(req.ID, false)
	if err != nil {
		return err
	}
	defer func() {
		if c != nil {
			c.Close()
		}
	}()
	switch req.Op {
	case runhcs.OpCreateContainer:
		err = createContainerInHost(c, vm)
		if err != nil {
			return err
		}
		c2 := c
		c = nil
		go func() {
			_ = c2.hc.Wait()
			c2.Close()
		}()

	case runhcs.OpUnmountContainer, runhcs.OpUnmountContainerDiskOnly:
		err = c.unmountInHost(vm, req.Op == runhcs.OpUnmountContainer)
		if err != nil {
			return err
		}

	case runhcs.OpSyncNamespace:
		return errors.New("Not implemented")
	default:
		panic("unknown operation")
	}
	return nil
}

type noVMError struct {
	ID string
}

func (err *noVMError) Error() string {
	return "VM " + err.ID + " cannot be contacted"
}

func (c *container) issueVMRequest(op runhcs.VMRequestOp) error {
	req := runhcs.VMRequest{
		ID: c.ID,
		Op: op,
	}
	if err := runhcs.IssueVMRequest(c.VMPipePath(), &req); err != nil {
		if perr, ok := err.(*os.PathError); ok && perr.Err == syscall.ERROR_FILE_NOT_FOUND {
			return &noVMError{c.HostID}
		}
		return err
	}
	return nil
}
