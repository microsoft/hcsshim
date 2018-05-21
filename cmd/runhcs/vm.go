package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"syscall"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/Microsoft/hcsshim/uvm"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func vmPipePath(id string) string {
	return safePipePath("runhcs-vmshim-" + id)
}

func vmID(id string) string {
	return id + "@vm"
}

var vmshimCommand = cli.Command{
	Name:   "vmshim",
	Usage:  `launch a VM and containers inside it (do not call it outside of runhcs)`,
	Hidden: true,
	Flags:  []cli.Flag{},
	Before: appargs.Validate(argID),
	Action: func(context *cli.Context) error {
		// Stdout is not used.
		os.Stdout.Close()

		id := context.Args().First()

		optsj, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		os.Stdin.Close()

		opts := &uvm.UVMOptions{}
		err = json.Unmarshal(optsj, opts)
		if err != nil {
			return err
		}

		// Listen on the named pipe associated with this VM.
		l, err := winio.ListenPipe(vmPipePath(id), &winio.PipeConfig{MessageMode: true})
		if err != nil {
			return err
		}

		vm, err := startVM(id, opts)
		if err != nil {
			return err
		}

		// Asynchronously wait for the VM to exit.
		exitCh := make(chan error)
		go func() {
			exitCh <- vm.Wait()
		}()

		defer vm.Terminate()

		// Alert the parent process that initialization has completed
		// successfully.
		os.Stderr.Write(shimSuccess)
		os.Stderr.Close()
		fatalWriter.Writer = ioutil.Discard

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
					_, err = pipe.Write(shimSuccess)
					// Wait until the pipe is closed before closing the
					// container so that it is properly handed off to the other
					// process.
					if err == nil {
						err = closeWritePipe(pipe)
					}
					if err == nil {
						ioutil.ReadAll(pipe)
					}
				} else {
					logrus.Error("failed creating container in VM", err)
					fmt.Fprintf(pipe, "%v", err)
				}
				pipe.Close()
			}
		}
	},
}

type vmRequestOp int

const (
	opCreateContainer vmRequestOp = iota
	opUnmountContainer
	opUnmountContainerDiskOnly
)

type vmRequest struct {
	ID string
	Op vmRequestOp
}

func startVM(id string, opts *uvm.UVMOptions) (*uvm.UtilityVM, error) {
	vm, err := uvm.Create(opts)
	if err != nil {
		return nil, err
	}
	err = vm.Start()
	if err != nil {
		vm.Close()
		return nil, err
	}
	return vm, nil
}

func processRequest(vm *uvm.UtilityVM, pipe net.Conn) error {
	var req vmRequest
	err := json.NewDecoder(pipe).Decode(&req)
	if err != nil {
		return err
	}
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
	case opCreateContainer:
		err = createContainerInHost(c, vm)
		if err != nil {
			return err
		}
		c2 := c
		c = nil
		go func() {
			c2.hc.Wait()
			c2.Close()
		}()
		c = nil

	case opUnmountContainer, opUnmountContainerDiskOnly:
		err = c.forceUnmount(vm, req.Op == opUnmountContainer)
		if err != nil {
			return err
		}

	default:
		panic("unknown operation")
	}
	return nil
}

var errNoVM = errors.New("the VM cannot be contacted")

func issueVMRequest(vmid, id string, op vmRequestOp) error {
	pipe, err := winio.DialPipe(vmPipePath(vmid), nil)
	if err != nil {
		if oerr, ok := err.(*net.OpError); ok && oerr.Err == syscall.ERROR_FILE_NOT_FOUND {
			return errNoVM
		}
		return err
	}
	defer pipe.Close()
	req := vmRequest{
		ID: id,
		Op: op,
	}
	err = json.NewEncoder(pipe).Encode(&req)
	if err != nil {
		return err
	}
	err = getErrorFromPipe(pipe, nil)
	if err != nil {
		return err
	}
	return nil
}
