//go:build windows
// +build windows

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/gcs"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"

	gcsBridge "github.com/Microsoft/hcsshim/cmd/gcs-sidecar/internal/bridge"
	"github.com/Microsoft/hcsshim/cmd/gcs-sidecar/internal/windowssecuritypolicy"
)

type handler struct {
	fromsvc chan error
}

// New guid for sidecar gcs service
// ae8da506-a019-4553-a52b-902bc0fa0411
var WindowsSidecarGcsHvsockServiceID = guid.GUID{
	Data1: 0xae8da506,
	Data2: 0xa019,
	Data3: 0x4553,
	Data4: [8]uint8{0xa5, 0x2b, 0x90, 0x2b, 0xc0, 0xfa, 0x04, 0x11},
}

// Accepts new connection closes listener
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

func (m *handler) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue

	status <- svc.Status{State: svc.StartPending, Accepts: 0}
	// unblock runService()
	m.fromsvc <- nil

	status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				log.Print("Shutting service...!")
				// TODO: service stop?!
				break loop
			case svc.Pause:
				status <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			case svc.Continue:
				status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			default:
				log.Printf("Unexpected service control request #%d", c)
			}
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
			err := debug.Run(name, h)
			if err != nil {
				log.Fatalf("Error running service in debug mode.Err: %v", err)
			}
		} else {
			err := svc.Run(name, h)
			if err != nil {
				log.Fatalf("Error running service in Service Control mode.Err %v", err)
			}
		}
		h.fromsvc <- err
	}()

	// Wait for the first signal from the service handler.
	log.Printf("waiting for first signal from service handler\n")
	err = <-h.fromsvc
	if err != nil {
		return err
	}
	return nil

}

func main() {
	// Ignore the following log when running sidecar outside the uvm.
	// Logs will be at C:\\gcs-sidecar-logs-redirect.log.
	// See internal/uvm/start.go#252 for more details.
	f, err := os.OpenFile("C:\\gcs-sidecar-logs.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("error opening file: %v", err)
	}
	defer f.Close()

	log.SetOutput(f)

	/*
		type srvResp struct {
			err error
		}

		chsrv := make(chan error)
		go func() {
			defer close(chsrv)

			if err := runService("gcs-sidecar", false); err != nil {
				log.Fatalf("error starting gcs-sidecar service: %v", err)
			}

			chsrv <- err
		}()

		select {
		// case <-ctx.Done():
		//	return ctx.Err()
		case r := <-chsrv:
			if r != nil {
				log.Fatal(r)
			}
		}
	*/

	// take in the uvm id as args
	if len(os.Args) != 2 {
		log.Printf("unexpected num of args: %v", len(os.Args))
		return
	}
	uvmID, err := guid.FromString(os.Args[1])
	if err != nil {
		log.Printf("error getting guid from string %v", os.Args[1])
		return
	}

	ctx := context.Background()
	// 1. Start external server to connect with inbox GCS
	listener, err := winio.ListenHvsock(&winio.HvsockAddr{
		VMID: uvmID,
		// TODO: Following line is commented out only for POC as we want to
		// start gcs-sidecar.exe on the host (external to uvm).
		// The VMID needs to be replaces with HV_GUID_PARENT in the
		// final changes.
		//HV_GUID_PARENT,
		ServiceID: gcs.WindowsGcsHvsockServiceID,
	})
	if err != nil {
		log.Printf("Error to start server for sidecar <-> inbox gcs communication: %v", err)
		return
	}

	var gcsListener net.Listener
	gcsListener = listener

	gcsCon, err := acceptAndClose(ctx, gcsListener)
	if err != nil {
		log.Printf("Err accepting inbox GCS connection %v", err)
		return
	}

	// 2. Setup connection with hcsshim external gcs connection
	hvsockAddr := &winio.HvsockAddr{
		VMID:      gcs.HV_GUID_LOOPBACK,
		ServiceID: gcs.WindowsSidecarGcsHvsockServiceID,
	}
	log.Printf("Dialing to hcsshim external bridge at address %v", hvsockAddr)

	shimCon, err := winio.Dial(ctx, hvsockAddr)
	if err != nil {
		log.Printf("Error dialing hcsshim external bridge at address %v", hvsockAddr)
		return
	}

	// set up our initial stance policy enforcer
	var initialEnforcer windowssecuritypolicy.SecurityPolicyEnforcer
	initialPolicyStance := "allow"
	switch initialPolicyStance {
	case "allow":
		initialEnforcer = &windowssecuritypolicy.OpenDoorSecurityPolicyEnforcer{}
		log.Printf("initial-policy-stance: allow")
	case "deny":
		initialEnforcer = &windowssecuritypolicy.ClosedDoorSecurityPolicyEnforcer{}
		log.Printf("initial-policy-stance: deny")
	default:
		log.Printf("unknown initial-policy-stance")
	}

	// 3. Create bridge and initializa
	brdg := gcsBridge.NewBridge(shimCon, gcsCon)
	brdg.PolicyEnforcer = gcsBridge.NewPolicyEnforcer(initialEnforcer)
	brdg.AssignHandlers()

	// 3. Listen and serve for hcsshim requests.
	// startSendAndRecvLoops(shimCon, gcsCon)
	err = brdg.ListenAndServeShimRequests()
	if err != nil {
		log.Printf("failed to serve gcs service \n")
	}
}
