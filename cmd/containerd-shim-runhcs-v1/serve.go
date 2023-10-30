//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	task "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/ttrpc"
	typeurl "github.com/containerd/typeurl/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"golang.org/x/sys/windows"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/internal/extendedtask"
	hcslog "github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/pkg/octtrpc"
)

var svc *service

var serveCommand = cli.Command{
	Name:           "serve",
	Hidden:         true,
	SkipArgReorder: true,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "socket",
			Usage: "the socket path to serve",
		},
		cli.BoolFlag{
			Name:  "is-sandbox",
			Usage: "is the task id a Kubernetes sandbox id",
		},
	},
	Action: func(ctx *cli.Context) error {
		// On Windows the serve command is internally used to actually create
		// the process that hosts the containerd/ttrpc entrypoint to the Runtime
		// V2 API's. The model requires this 2nd invocation of the shim process
		// so that the 1st invocation can return the address via stdout on
		// success.
		//
		// The activation model for this shim is as follows:
		//
		// The public invocation of `shim start` is called which internally
		// decides to either return the address of an existing shim or serve a
		// new one. If serve is decided it execs this entry point `shim serve`.
		// The handoff logic is that this shim will serve the ttrpc entrypoint
		// with only stderr set by the caller. Once the shim has successfully
		// served the entrypoint it is required to close stdout to alert the
		// caller it has completed to the point of handoff. If it fails it will
		// write the error to stderr and the caller will forward the error on as
		// part of the `shim start` failure path. Once successfully served the
		// shim `MUST` not use any std handles. The shim can log any errors to
		// the upstream caller by listening for a log connection and streaming
		// the events.

		var lerrs chan error

		// Default values for shim options.
		shimOpts := &runhcsopts.Options{
			Debug:     false,
			DebugType: runhcsopts.Options_NPIPE,
		}

		// containerd passes the shim options protobuf via stdin.
		newShimOpts, err := readOptions(os.Stdin)
		if err != nil {
			return errors.Wrap(err, "failed to read shim options from stdin")
		} else if newShimOpts != nil {
			// We received a valid shim options struct.
			shimOpts = newShimOpts
		}

		if shimOpts.Debug && shimOpts.LogLevel != "" {
			logrus.Warning("Both Debug and LogLevel specified, Debug will be overridden")
		}

		// For now keep supporting the debug option, this used to be the only way to specify a different logging
		// level for the shim.
		if shimOpts.Debug {
			logrus.SetLevel(logrus.DebugLevel)
		}

		// If log level is specified, set the corresponding logrus logging level. This overrides the debug option
		// (unless the level being asked for IS debug also, then this doesn't do much).
		if shimOpts.LogLevel != "" {
			lvl, err := logrus.ParseLevel(shimOpts.LogLevel)
			if err != nil {
				return errors.Wrapf(err, "failed to parse shim log level %q", shimOpts.LogLevel)
			}
			logrus.SetLevel(lvl)
		}

		switch shimOpts.DebugType {
		case runhcsopts.Options_NPIPE:
			logrus.SetFormatter(&logrus.TextFormatter{
				TimestampFormat: hcslog.TimeFormat,
				FullTimestamp:   true,
			})
			// Setup the log listener
			//
			// TODO: JTERRY75 we need this to be the reconnect log listener or
			// switch to events
			// TODO: JTERRY75 switch containerd to use the protected path.
			//const logAddrFmt = "\\\\.\\pipe\\ProtectedPrefix\\Administrators\\containerd-shim-%s-%s-log"
			const logAddrFmt = "\\\\.\\pipe\\containerd-shim-%s-%s-log"
			logl, err := winio.ListenPipe(fmt.Sprintf(logAddrFmt, namespaceFlag, idFlag), nil)
			if err != nil {
				return err
			}
			defer logl.Close()

			lerrs = make(chan error, 1)
			go func() {
				var cur net.Conn
				for {
					// Listen for log connections in the background
					// We assume that there is always only one client
					// which is containerd. If a new connection is
					// accepted, it means that containerd is restarted.
					// Note that logs generated during containerd restart
					// may be lost.
					new, err := logl.Accept()
					if err != nil {
						lerrs <- err
						return
					}
					if cur != nil {
						cur.Close()
					}
					cur = new
					// Switch the logrus output to here. Note: we wont get this
					// connection until the return from `shim start` so we still
					// havent transitioned the error model yet.
					logrus.SetOutput(cur)
				}
			}()
			// Logrus output will be redirected in the goroutine below that
			// handles the pipe connection.
		case runhcsopts.Options_FILE:
			panic("file log output mode is not supported")
		case runhcsopts.Options_ETW:
			logrus.SetFormatter(hcslog.NopFormatter{})
			logrus.SetOutput(io.Discard)
		}

		os.Stdin.Close()

		// enable scrubbing
		if shimOpts.ScrubLogs {
			hcslog.SetScrubbing(true)
		}

		// Force the cli.ErrWriter to be os.Stdout for this. We use stderr for
		// the panic.log attached via start.
		cli.ErrWriter = os.Stdout

		socket := ctx.String("socket")
		if !strings.HasPrefix(socket, `\\.\pipe`) {
			return errors.New("socket is required to be pipe address")
		}

		ttrpcAddress := os.Getenv(ttrpcAddressEnv)
		ttrpcEventPublisher, err := newEventPublisher(ttrpcAddress, namespaceFlag)
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				ttrpcEventPublisher.close()
			}
		}()

		// Setup the ttrpc server
		svc, err = NewService(WithEventPublisher(ttrpcEventPublisher),
			WithTID(idFlag),
			WithIsSandbox(ctx.Bool("is-sandbox")))
		if err != nil {
			return fmt.Errorf("failed to create new service: %w", err)
		}

		s, err := ttrpc.NewServer(ttrpc.WithUnaryServerInterceptor(octtrpc.ServerInterceptor()))
		if err != nil {
			return err
		}
		defer s.Close()
		task.RegisterTaskService(s, svc)
		shimdiag.RegisterShimDiagService(s, svc)
		extendedtask.RegisterExtendedTaskService(s, svc)

		sl, err := winio.ListenPipe(socket, nil)
		if err != nil {
			return err
		}
		defer sl.Close()

		serrs := make(chan error, 1)
		defer close(serrs)
		go func() {
			// Serve loops infinitely unless s.Shutdown or s.Close are called.
			// Passed in context is used as parent context for handling requests,
			// but canceliing does not bring down ttrpc service.
			if err := trapClosedConnErr(s.Serve(context.Background(), sl)); err != nil {
				logrus.WithError(err).Fatal("containerd-shim: ttrpc server failure")
				serrs <- err
				return
			}
			serrs <- nil
		}()

		select {
		case err := <-lerrs:
			return err
		case err := <-serrs:
			return err
		case <-time.After(2 * time.Millisecond):
			// TODO: Contribute a change to ttrpc so that you can:
			//
			// go func () { errs <- s.Serve() }
			// select {
			// case <-errs:
			// case <-s.Ready():
			// }

			// This is our best indication that we have not errored on creation
			// and are successfully serving the API.
			// Closing stdout signals to containerd that shim started successfully
			os.Stdout.Close()
		}

		// Wait for the serve API to be shut down.
		select {
		case err = <-serrs:
			// the ttrpc server shutdown without processing a shutdown request
		case <-svc.Done():
			if !svc.gracefulShutdown {
				// Return immediately, but still close ttrpc server, pipes, and spans
				// Shouldn't need to os.Exit without clean up (ie, deferred `.Close()`s)
				return nil
			}
			// currently the ttrpc shutdown is the only clean up to wait on
			sctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
			defer cancel()
			err = s.Shutdown(sctx)
		}

		return err
	},
}

func trapClosedConnErr(err error) error {
	if err == nil || strings.Contains(err.Error(), "use of closed network connection") {
		return nil
	}
	return err
}

// readOptions reads in bytes from the reader and converts it to a shim options
// struct. If no data is available from the reader, returns (nil, nil).
func readOptions(r io.Reader) (*runhcsopts.Options, error) {
	d, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read input")
	}
	if len(d) > 0 {
		var a anypb.Any
		if err := proto.Unmarshal(d, &a); err != nil {
			return nil, errors.Wrap(err, "failed unmarshalling into Any")
		}
		v, err := typeurl.UnmarshalAny(&a)
		if err != nil {
			return nil, errors.Wrap(err, "failed unmarshalling by typeurl")
		}
		return v.(*runhcsopts.Options), nil
	}
	return nil, nil
}

// createEvent creates a Windows event ACL'd to builtin administrator
// and local system. Can use docker-signal to signal the event.
func createEvent(event string) (windows.Handle, error) {
	ev, _ := windows.UTF16PtrFromString(event)
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;BA)(A;;GA;;;SY)")
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get security descriptor for event '%s'", event)
	}
	var sa windows.SecurityAttributes
	sa.Length = uint32(unsafe.Sizeof(sa))
	sa.InheritHandle = 1
	sa.SecurityDescriptor = sd
	h, err := windows.CreateEvent(&sa, 0, 0, ev)
	if h == 0 || err != nil {
		return 0, errors.Wrapf(err, "failed to create event '%s'", event)
	}
	return h, nil
}

// setupDebuggerEvent listens for an event to allow a debugger such as delve
// to attach for advanced debugging. It's called when handling a ContainerCreate
func setupDebuggerEvent() {
	if os.Getenv("CONTAINERD_SHIM_RUNHCS_V1_WAIT_DEBUGGER") == "" {
		return
	}
	event := "Global\\debugger-" + fmt.Sprint(os.Getpid())
	handle, err := createEvent(event)
	if err != nil {
		return
	}
	logrus.WithField("event", event).Info("Halting until signalled")
	_, _ = windows.WaitForSingleObject(handle, windows.INFINITE)
}
