package svm

import (
	"io"
	"time"

	"github.com/Microsoft/hcsshim/internal/lcow"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	//	"github.com/sirupsen/logrus"
)

// ByteCounts are the number of bytes copied to/from standard handles. Note
// this is int64 rather than uint64 to match the golang io.Copy() signature.
type ByteCounts struct {
	In  int64
	Out int64
	Err int64
}

// ProcessOptions are the set of options which are passed RunProcess
// create a utility vm.
type ProcessOptions struct {
	Id          string
	Args        []string
	Stdin       io.Reader     // Optional reader for sending on to the processes stdin stream
	Stdout      io.Writer     // Optional writer for returning the processes stdout stream
	Stderr      io.Writer     // Optional writer for returning the processes stderr stream
	CopyTimeout time.Duration // Timeout for copying streams
}

// RunProcess runs a process in a service VM.
func (i *instance) RunProcess(opts *ProcessOptions) (*ByteCounts, int, error) {

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
		return nil, -1, ErrNotFound
	}
	svmItem, exists := i.serviceVMs[id]
	if !exists {
		return nil, -1, ErrNotFound
	}

	lcowpo := &lcow.ProcessOptions{
		HCSSystem:         svmItem.serviceVM.utilityVM.ComputeSystem(),
		Process:           &specs.Process{Args: opts.Args},
		Stdin:             opts.Stdin,
		Stdout:            opts.Stdout,
		Stderr:            opts.Stderr,
		CopyTimeout:       opts.CopyTimeout,
		CreateInUtilityVm: true,
		ByteCounts:        lcow.ByteCounts{},
	}

	p, bc, err := lcow.CreateProcess(lcowpo)
	if err != nil {
		return nil, -1, err
	}
	defer p.Close()

	if err := p.Wait(); err != nil {
		return nil, -1, err
	}

	ec, err := p.ExitCode()
	if err != nil {
		return nil, -1, err
	}

	return &ByteCounts{In: bc.In, Out: bc.Out, Err: bc.Err}, ec, err

}

// // ProcessOptions are the set of options which are passed to CreateProcessEx() to
// // create a utility vm.
// type ProcessOptions struct {
// 	HCSSystem         *hcs.System
// 	Process           *specs.Process
// 	Stdin             io.Reader     // Optional reader for sending on to the processes stdin stream
// 	Stdout            io.Writer     // Optional writer for returning the processes stdout stream
// 	Stderr            io.Writer     // Optional writer for returning the processes stderr stream
// 	CopyTimeout       time.Duration // Timeout for the copy
// 	CreateInUtilityVm bool          // If the compute system is a utility VM
// 	ByteCounts        ByteCounts    // How much data to copy on each stream if they are supplied. 0 means to io.EOF.
// }

// func CreateProcess(opts *ProcessOptions) (*hcs.Process, *ByteCounts, error) {
