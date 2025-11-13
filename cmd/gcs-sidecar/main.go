//go:build windows
// +build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	shimlog "github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/pspdriver"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"

	sidecar "github.com/Microsoft/hcsshim/internal/gcs-sidecar"
)

var (
	defaultLogFile  = "C:\\gcs-sidecar-logs.log"
	defaultLogLevel = "trace"
)

type handler struct {
	fromsvc chan error
}

// Accepts new connection and closes listener.
func acceptAndClose(ctx context.Context, l net.Listener) (net.Conn, error) {
	var conn net.Conn
	ch := make(chan error)
	go func() {
		var err error
		conn, err = l.Accept()
		ch <- err
	}()
	select {
	case err := <-ch:
		l.Close()
		return conn, err
	case <-ctx.Done():
	}
	l.Close()
	err := <-ch
	if err == nil {
		return conn, err
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, err
}

func (h *handler) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.Accepted(windows.SERVICE_ACCEPT_PARAMCHANGE)

	status <- svc.Status{State: svc.StartPending, Accepts: 0}
	// unblock runService()
	h.fromsvc <- nil

	status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

loop:
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			status <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			logrus.Println("Shutting service...!")
			break loop
		case svc.Pause:
			status <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
		case svc.Continue:
			status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
		default:
			logrus.Printf("Unexpected service control request #%d", c)
		}
	}

	status <- svc.Status{State: svc.StopPending}
	return false, 1
}

func runService(name string, isDebug bool) error {
	h := &handler{
		fromsvc: make(chan error),
	}

	var err error
	go func() {
		if isDebug {
			err = debug.Run(name, h)
			if err != nil {
				logrus.Errorf("Error running service in debug mode.Err: %v", err)
			}
		} else {
			err = svc.Run(name, h)
			if err != nil {
				logrus.Errorf("Error running service in Service Control mode.Err %v", err)
			}
		}
		h.fromsvc <- err
	}()

	// Wait for the first signal from the service handler.
	logrus.Tracef("waiting for first signal from service handler\n")
	return <-h.fromsvc
}

func main() {
	logLevel := flag.String("loglevel",
		defaultLogLevel,
		"Logging Level: trace, debug, info, warning, error, fatal, panic.")
	logFile := flag.String("logfile",
		defaultLogFile,
		"Logging Target. Default is at C:\\gcs-sidecar-logs.log inside UVM")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\nUsage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "    %s -loglevel=trace -logfile=C:\\sidecarLogs.log \n", os.Args[0])
	}

	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	logFileHandle, err := os.OpenFile(*logFile, os.O_RDWR|os.O_CREATE|os.O_SYNC|os.O_TRUNC, 0666)
	if err != nil {
		fmt.Printf("error opening file: %v", err)
	}
	defer logFileHandle.Close()

	logrus.AddHook(shimlog.NewHook())

	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.SetLevel(level)
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
	trace.RegisterExporter(&oc.LogrusExporter{})

	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(logFileHandle.Fd())); err != nil {
		logrus.WithError(err).Error("error redirecting handle")
		return
	}
	os.Stderr = logFileHandle

	chsrv := make(chan error)
	go func() {
		defer close(chsrv)

		if err := runService("gcs-sidecar", false); err != nil {
			logrus.Errorf("error starting gcs-sidecar service: %v", err)
		}

		chsrv <- err
	}()

	select {
	case <-ctx.Done():
		logrus.Error("context deadline exceeded")
		return
	case r := <-chsrv:
		if r != nil {
			logrus.Error(r)
			return
		}
	}

	logrus.Println("Initializing VSMB redirector..")
	sidecar.VsmbMain()

	// 1. Start external server to connect with inbox GCS
	listener, err := winio.ListenHvsock(&winio.HvsockAddr{
		VMID:      prot.HvGUIDLoopback,
		ServiceID: prot.WindowsGcsHvsockServiceID,
	})
	if err != nil {
		logrus.WithError(err).Error("error starting listener for sidecar <-> inbox gcs communication")
		return
	}

	var gcsListener net.Listener = listener
	gcsCon, err := acceptAndClose(ctx, gcsListener)
	if err != nil {
		logrus.WithError(err).Error("error accepting inbox GCS connection")
		return
	}

	// 2. Setup connection with external gcs connection started from hcsshim
	hvsockAddr := &winio.HvsockAddr{
		VMID:      prot.HvGUIDParent,
		ServiceID: prot.WindowsSidecarGcsHvsockServiceID,
	}

	logrus.WithFields(logrus.Fields{
		"hvsockAddr": hvsockAddr,
	}).Tracef("Dialing to hcsshim external bridge at address %v", hvsockAddr)
	shimCon, err := winio.Dial(ctx, hvsockAddr)
	if err != nil {
		logrus.WithError(err).Error("error dialing hcsshim external bridge")
		return
	}

	if err := pspdriver.StartPSPDriver(ctx); err != nil {
		// When error happens, pspdriver.GetPspDriverError() returns true.
		// In that case, gcs-sidecar should keep the initial "deny" policy
		// and reject all requests from the host.
		logrus.WithError(err).Errorf("failed to start PSP driver")
	}

	// Use "deny" policy as initial enforcer.
	// This is updated later with user provided policy.
	initialEnforcer := &securitypolicy.ClosedDoorSecurityPolicyEnforcer{}

	// 3. Create bridge and initializa
	brdg := sidecar.NewBridge(shimCon, gcsCon, initialEnforcer, logFileHandle)
	brdg.AssignHandlers()

	// 3. Listen and serve for hcsshim requests.
	err = brdg.ListenAndServeShimRequests()
	if err != nil {
		logrus.WithError(err).Error("failed to serve request")
	}
}
