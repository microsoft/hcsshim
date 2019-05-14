package svm

import (
	"context"
	"io"
	"time"

	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"
)

// ProcessOptions are the set of options which are passed RunProcess
// create a utility vm.
type ProcessOptions struct {
	Id      string
	Args    []string
	Stdin   io.Reader     // Optional reader for sending on to the processes stdin stream
	Stdout  io.Writer     // Optional writer for returning the processes stdout stream
	Stderr  io.Writer     // Optional writer for returning the processes stderr stream
	Timeout time.Duration // Timeout for copying streams
}

// RunProcess runs a process in a service VM.
func (i *instance) RunProcess(opts *ProcessOptions) (int, error) {

	id := opts.Id

	// Keep a global service VM running - effectively a no-op
	if i.mode == ModeGlobal {
		id = globalID
	}

	// Write operation. Must hold the lock. TODO Not sure we do....
	//i.Lock()
	//defer i.Unlock()

	// Nothing to do if no service VMs or not found
	if i.serviceVMs == nil {
		return -1, ErrNotFound
	}
	svmItem, exists := i.serviceVMs[id]
	if !exists {
		return -1, ErrNotFound
	}

	ctx, cancel := context.WithTimeout(context.TODO(), opts.Timeout)
	defer cancel()

	cmd := hcsoci.CommandContext(ctx, svmItem.serviceVM.utilityVM, opts.Args[0], opts.Args[1:]...)
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Log = logrus.WithField(logfields.UVMID, opts.Id)
	err := cmd.Run()
	return cmd.ExitState.ExitCode(), err

}
