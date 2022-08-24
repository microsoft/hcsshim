//go:build windows

package main

import (
	"log"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "ncproxy"

var (
	panicFile *os.File
	oldStderr windows.Handle
)

type handler struct {
	fromsvc chan error
	done    chan struct{}
}

type serviceFailureActions struct {
	ResetPeriod  uint32
	RebootMsg    *uint16
	Command      *uint16
	ActionsCount uint32
	Actions      uintptr
}

type scAction struct {
	Type  uint32
	Delay uint32
}

// See http://stackoverflow.com/questions/35151052/how-do-i-configure-failure-actions-of-a-windows-service-written-in-go
const (
	scActionNone    = 0
	scActionRestart = 1

	serviceConfigFailureActions = 2
)

func initPanicFile(path string) error {
	panicFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	st, err := panicFile.Stat()
	if err != nil {
		return err
	}

	// If there are contents in the file already, move the file out of the way
	// and replace it.
	if st.Size() > 0 {
		panicFile.Close()
		_ = os.Rename(path, path+".old")
		panicFile, err = os.Create(path)
		if err != nil {
			return err
		}
	}

	// Update STD_ERROR_HANDLE to point to the panic file so that Go writes to
	// it when it panics. Remember the old stderr to restore it before removing
	// the panic file.
	sh := uint32(windows.STD_ERROR_HANDLE)
	h, err := windows.GetStdHandle(sh)
	if err != nil {
		return err
	}
	oldStderr = h

	if err := windows.SetStdHandle(sh, windows.Handle(panicFile.Fd())); err != nil {
		return err
	}

	// Reset os.Stderr to the panic file (so fmt.Fprintf(os.Stderr,...) actually gets redirected)
	os.Stderr = os.NewFile(panicFile.Fd(), "/dev/stderr-ncproxy")

	// Force threads that panic to write to stderr (the panicFile handle now).
	log.SetOutput(os.Stderr)
	return nil
}

func removePanicFile() {
	if st, err := panicFile.Stat(); err == nil {
		// If there's anything in the file we wrote (e.g. panic logs), don't delete it.
		if st.Size() == 0 {
			sh := uint32(windows.STD_ERROR_HANDLE)
			_ = windows.SetStdHandle(sh, oldStderr)
			_ = panicFile.Close()
			_ = os.Remove(panicFile.Name())
		}
	}
}

func registerService() error {
	p, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() {
		_ = m.Disconnect()
	}()

	c := mgr.Config{
		ServiceType:  windows.SERVICE_WIN32_OWN_PROCESS,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
		DisplayName:  "Ncproxy",
		Description:  "Network configuration proxy",
	}

	// Configure the service to launch with the arguments that were just passed.
	args := []string{"--run-service"}
	for _, a := range os.Args[1:] {
		if a != "--register-service" {
			args = append(args, a)
		}
	}

	s, err := m.CreateService(serviceName, p, c, args...)
	if err != nil {
		return err
	}
	defer s.Close()

	t := []scAction{
		{Type: scActionRestart, Delay: uint32(15 * time.Second / time.Millisecond)},
		{Type: scActionRestart, Delay: uint32(15 * time.Second / time.Millisecond)},
		{Type: scActionNone},
	}
	lpInfo := serviceFailureActions{ResetPeriod: uint32(24 * time.Hour / time.Second), ActionsCount: uint32(3), Actions: uintptr(unsafe.Pointer(&t[0]))}
	return windows.ChangeServiceConfig2(s.Handle, serviceConfigFailureActions, (*byte)(unsafe.Pointer(&lpInfo)))
}

func unregisterService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() {
		_ = m.Disconnect()
	}()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer s.Close()

	return s.Delete()
}

// launchService is the entry point for running ncproxy under SCM.
func launchService(done chan struct{}) error {
	h := &handler{
		fromsvc: make(chan error),
		done:    done,
	}

	interactive, err := svc.IsAnInteractiveSession() //nolint:staticcheck
	if err != nil {
		return err
	}

	go func() {
		if interactive {
			err = debug.Run(serviceName, h)
		} else {
			err = svc.Run(serviceName, h)
		}
		h.fromsvc <- err
	}()

	// Wait for the first signal from the service handler.
	return <-h.fromsvc
}

func (h *handler) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending, Accepts: 0}
	// Unblock launchService()
	h.fromsvc <- nil

	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown | svc.Accepted(windows.SERVICE_ACCEPT_PARAMCHANGE)}

Loop:
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			s <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			s <- svc.Status{State: svc.StopPending, Accepts: 0}
			break Loop
		}
	}

	removePanicFile()
	close(h.done)
	return false, 0
}
