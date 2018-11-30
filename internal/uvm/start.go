package uvm

import (
	"io"
	"net"
	"os"
	"syscall"

	"github.com/sirupsen/logrus"
)

const _ERROR_CONNECTION_ABORTED syscall.Errno = 1236

func forwardGcsLogs(l net.Listener, waitChannel chan struct{}, suppressOutput bool) {
	c, err := l.Accept()
	l.Close()
	if err != nil {
		logrus.Error("accepting log socket: ", err)
		return
	}
	defer c.Close()

	if !suppressOutput {
		io.Copy(os.Stdout, c)
	}

	close(waitChannel)
}

// Start synchronously starts the utility VM.
func (uvm *UtilityVM) Start() error {
	if uvm.gcslog != nil {
		go forwardGcsLogs(uvm.gcslog, uvm.gcsLogsExited, uvm.suppressGcsLogs)
		uvm.gcslog = nil
	}
	return uvm.hcsSystem.Start()
}
