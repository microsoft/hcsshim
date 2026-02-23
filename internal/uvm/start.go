//go:build windows

package uvm

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/netutil"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/timeout"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
)

// entropyBytes is the number of bytes of random data to send to a Linux UVM
// during boot to seed the CRNG. There is not much point in making this too
// large since the random data collected from the host is likely computed from a
// relatively small key (256 bits?), so additional bytes would not actually
// increase the entropy of the guest's pool. However, send enough to convince
// containers that there is a large amount of entropy since this idea is
// generally misunderstood.
const entropyBytes = 512

type gcsLogEntryStandard struct {
	Time    time.Time    `json:"time"`
	Level   logrus.Level `json:"level"`
	Message string       `json:"msg"`
}

type gcsLogEntry struct {
	gcsLogEntryStandard
	Fields map[string]interface{}
}

// FUTURE-jstarks: Change the GCS log format to include type information
// (e.g. by using a different encoding such as protobuf).
func (e *gcsLogEntry) UnmarshalJSON(b []byte) error {
	// Default the log level to info.
	e.Level = logrus.InfoLevel
	if err := json.Unmarshal(b, &e.gcsLogEntryStandard); err != nil {
		return err
	}
	if err := json.Unmarshal(b, &e.Fields); err != nil {
		return err
	}
	// Do not allow fatal or panic level errors to propagate.
	if e.Level < logrus.ErrorLevel {
		e.Level = logrus.ErrorLevel
	}
	if e.Fields["Source"] == "ETW" {
		// Windows ETW log entry
		// Original ETW Event Data may have "message" or "Message" field instead of "msg"
		if msg, ok := e.Fields["message"].(string); ok {
			e.Message = msg
			delete(e.Fields, "message")
		} else if msg, ok := e.Fields["Message"].(string); ok {
			e.Message = msg
			delete(e.Fields, "Message")
		}
	}
	// Clear special fields.
	delete(e.Fields, "time")
	delete(e.Fields, "level")
	delete(e.Fields, "msg")
	// Normalize floats to integers.
	for k, v := range e.Fields {
		if d, ok := v.(float64); ok && float64(int64(d)) == d {
			e.Fields[k] = int64(d)
		}
	}
	return nil
}

func isDisconnectError(err error) bool {
	return hcs.IsAny(err, windows.WSAECONNABORTED, windows.WSAECONNRESET)
}

func parseLogrus(o *Options) OutputHandler {
	vmid := ""
	if o != nil {
		vmid = o.ID
	}
	return func(r io.Reader) {
		j := json.NewDecoder(r)
		e := log.L.Dup()
		fields := e.Data
		for {
			for k := range fields {
				delete(fields, k)
			}
			gcsEntry := gcsLogEntry{Fields: e.Data}
			err := j.Decode(&gcsEntry)
			if err != nil {
				// Something went wrong. Read the rest of the data as a single
				// string and log it at once -- it's probably a GCS panic stack.
				if !errors.Is(err, io.EOF) && !isDisconnectError(err) {
					logrus.WithFields(logrus.Fields{
						logfields.UVMID: vmid,
						logrus.ErrorKey: err,
					}).Error("gcs log read")
				}
				rest, _ := io.ReadAll(io.MultiReader(j.Buffered(), r))
				rest = bytes.TrimSpace(rest)
				if len(rest) != 0 {
					logrus.WithFields(logrus.Fields{
						logfields.UVMID: vmid,
						"stderr":        string(rest),
					}).Error("gcs terminated")
				}
				break
			}
			fields[logfields.UVMID] = vmid
			fields["vm.time"] = gcsEntry.Time
			e.Log(gcsEntry.Level, gcsEntry.Message)
		}
	}
}

// When using an external GCS connection it is necessary to send a ModifySettings request
// for HvSocket so that the GCS can setup some registry keys that are required for running
// containers inside the UVM. In non external GCS connection scenarios this is done by the
// HCS immediately after the GCS connection is done. Since, we are using the external GCS
// connection we should do that setup here after we connect with the GCS.
// This only applies for WCOW
func (uvm *UtilityVM) configureHvSocketForGCS(ctx context.Context) (err error) {
	if uvm.OS() != "windows" {
		return nil
	}

	hvsocketAddress := &hcsschema.HvSocketAddress{
		LocalAddress:  uvm.runtimeID.String(),
		ParentAddress: prot.WindowsGcsHvHostID.String(),
	}

	conSetupReq := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			RequestType:  guestrequest.RequestTypeUpdate,
			ResourceType: guestresource.ResourceTypeHvSocket,
			Settings:     hvsocketAddress,
		},
	}

	if err = uvm.modify(ctx, conSetupReq); err != nil {
		return fmt.Errorf("failed to configure HVSOCK for external GCS: %w", err)
	}

	return nil
}

// Start synchronously starts the utility VM.
func (uvm *UtilityVM) Start(ctx context.Context) (err error) {
	// save parent context, without timeout to use in terminate
	pCtx := ctx
	ctx, cancel := context.WithTimeout(pCtx, timeout.GCSConnectionTimeout)
	log.G(ctx).Debugf("using gcs connection timeout: %s\n", timeout.GCSConnectionTimeout)

	g, gctx := errgroup.WithContext(ctx)
	defer func() {
		_ = g.Wait()
	}()
	defer cancel()

	// create exitCh ahead of time to prevent race conditions between writing
	// initalizing the channel and waiting on it during acceptAndClose
	uvm.exitCh = make(chan struct{})

	e := log.G(ctx).WithField(logfields.UVMID, uvm.id)

	// log errors in the the wait groups, since if multiple go routines return an error,
	// theres no guarantee on which will be returned.

	// Prepare to provide entropy to the init process in the background. This
	// must be done in a goroutine since, when using the internal bridge, the
	// call to Start() will block until the GCS launches, and this cannot occur
	// until the host accepts and closes the entropy connection.
	if uvm.entropyListener != nil {
		g.Go(func() error {
			conn, err := uvm.accept(gctx, uvm.entropyListener, true)
			uvm.entropyListener = nil
			if err != nil {
				e.WithError(err).Error("failed to connect to entropy socket")
				return fmt.Errorf("failed to connect to entropy socket: %w", err)
			}
			defer conn.Close()
			_, err = io.CopyN(conn, rand.Reader, entropyBytes)
			if err != nil {
				e.WithError(err).Error("failed to write entropy")
				return fmt.Errorf("failed to write entropy: %w", err)
			}
			return nil
		})
	}

	if uvm.outputListener != nil {
		switch uvm.operatingSystem {
		case "windows":
			// Windows specific handling
			// For windows, the Listener can recieve a connection later, so we
			// start the output handler in a goroutine with a non-timeout context.
			// This allows the output handler to run independently of the UVM Create's
			// lifecycle. The approach potentially allows to wait for reconnections too,
			// while limiting the number of concurrent connections to 1.
			// This is useful for the case when logging service is restarted.
			go func() {
				var wg sync.WaitGroup
				uvm.outputListener = netutil.LimitListener(uvm.outputListener, 1)
				for {
					conn, err := uvm.accept(context.WithoutCancel(ctx), uvm.outputListener, false)
					if err != nil {
						e.WithError(err).Error("failed to connect to log socket")
						break
					}
					wg.Add(1)
					go func() {
						defer wg.Done()
						e.Info("uvm output handler starting")
						uvm.outputHandler(conn)
					}()
					e.Info("uvm output handler finished")
				}
				wg.Wait()
				if _, ok := <-uvm.outputProcessingDone; ok {
					close(uvm.outputProcessingDone)
				}
			}()
		default:
			// Default handling
			g.Go(func() error {
				conn, err := uvm.accept(gctx, uvm.outputListener, true)
				uvm.outputListener = nil
				if err != nil {
					e.WithError(err).Error("failed to connect to log socket")
					close(uvm.outputProcessingDone)
					return fmt.Errorf("failed to connect to log socket: %w", err)
				}
				go func() {
					e.Trace("uvm output handler starting")
					uvm.outputHandler(conn)
					close(uvm.outputProcessingDone)
					e.Debug("uvm output handler finished")
				}()
				return nil
			})
		}
	}

	err = uvm.hcsSystem.Start(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			// use parent context, to prevent 2 minute timout (set above) from overridding terminate operation's
			// timeout and erroring out prematurely
			_ = uvm.hcsSystem.Terminate(pCtx)
			_ = uvm.hcsSystem.WaitCtx(pCtx)
		}
	}()

	// Start waiting on the utility VM.
	go func() {
		// the original context may have timeout or propagate a cancellation
		// copy the original to prevent it affecting the background wait go routine
		cCtx := context.WithoutCancel(pCtx)
		err := uvm.hcsSystem.WaitCtx(cCtx)
		if err == nil {
			err = uvm.hcsSystem.ExitError()
		}
		uvm.exitErr = err
		close(uvm.exitCh)
	}()

	// Collect any errors from writing entropy or establishing the log
	// connection.
	if err = g.Wait(); err != nil {
		return err
	}

	if uvm.gcListener != nil {
		// Accept the GCS connection.
		conn, err := uvm.accept(ctx, uvm.gcListener, true)
		uvm.gcListener = nil
		if err != nil {
			return fmt.Errorf("failed to connect to GCS: %w", err)
		}

		var initGuestState *gcs.InitialGuestState
		if uvm.OS() == "windows" {
			// Default to setting the time zone in the UVM to the hosts time zone unless the client asked to avoid this behavior. If so, assign
			// to UTC.
			if uvm.noInheritHostTimezone {
				initGuestState = &gcs.InitialGuestState{
					Timezone: utcTimezone,
				}
			} else {
				tz, err := getTimezone()
				if err != nil {
					return err
				}
				initGuestState = &gcs.InitialGuestState{
					Timezone: tz,
				}
			}
		}
		// Start the GCS protocol.
		gcc := &gcs.GuestConnectionConfig{
			Conn:           conn,
			Log:            e,
			IoListen:       gcs.HvsockIoListen(uvm.runtimeID),
			InitGuestState: initGuestState,
		}
		uvm.gc, err = gcc.Connect(ctx, true)
		if err != nil {
			return err
		}
		uvm.guestCaps = uvm.gc.Capabilities()
		uvm.protocol = uvm.gc.Protocol()

		// initial setup required for external GCS connection
		if err = uvm.configureHvSocketForGCS(ctx); err != nil {
			return fmt.Errorf("failed to do initial GCS setup: %w", err)
		}
	} else {
		// Cache the guest connection properties.
		properties, err := uvm.hcsSystem.Properties(ctx, schema1.PropertyTypeGuestConnection)
		if err != nil {
			return err
		}
		uvm.guestCaps = &gcs.WCOWGuestDefinedCapabilities{GuestDefinedCapabilities: properties.GuestConnectionInfo.GuestDefinedCapabilities}
		uvm.protocol = properties.GuestConnectionInfo.ProtocolVersion
	}

	// Initialize the SCSIManager.
	var gb scsi.GuestBackend
	if uvm.gc != nil {
		gb = scsi.NewBridgeGuestBackend(uvm.gc, uvm.OS())
	} else {
		gb = scsi.NewHCSGuestBackend(uvm.hcsSystem, uvm.OS())
	}
	guestMountFmt := `c:\mounts\scsi\m%d`
	if uvm.OS() == "linux" {
		guestMountFmt = "/run/mounts/scsi/m%d"
	}
	mgr, err := scsi.NewManager(
		scsi.NewHCSHostBackend(uvm.hcsSystem),
		gb,
		int(uvm.scsiControllerCount),
		64, // LUNs per controller, fixed by Hyper-V.
		guestMountFmt,
		uvm.reservedSCSISlots)
	if err != nil {
		return fmt.Errorf("creating scsi manager: %w", err)
	}
	uvm.SCSIManager = mgr

	if uvm.HasConfidentialPolicy() {
		var policy, enforcer, referenceInfoFileRoot, referenceInfoFilePath string
		if uvm.OS() == "linux" {
			policy = uvm.createOpts.(*OptionsLCOW).SecurityPolicy
			enforcer = uvm.createOpts.(*OptionsLCOW).SecurityPolicyEnforcer
			referenceInfoFilePath = uvm.createOpts.(*OptionsLCOW).UVMReferenceInfoFile
			referenceInfoFileRoot = vmutils.DefaultLCOWOSBootFilesPath()
		} else if uvm.OS() == "windows" {
			policy = uvm.createOpts.(*OptionsWCOW).SecurityPolicy
			enforcer = uvm.createOpts.(*OptionsWCOW).SecurityPolicyEnforcer
			referenceInfoFilePath = uvm.createOpts.(*OptionsWCOW).UVMReferenceInfoFile
		}
		copts := []ConfidentialUVMOpt{
			WithSecurityPolicy(policy),
			WithSecurityPolicyEnforcer(enforcer),
			WithUVMReferenceInfo(referenceInfoFileRoot, referenceInfoFilePath),
		}
		if err := uvm.SetConfidentialUVMOptions(ctx, copts...); err != nil {
			return err
		}
	}

	if uvm.OS() == "windows" && uvm.forwardLogs {
		// If the UVM is Windows and log forwarding is enabled, set the log sources
		// and start the log forwarding service.
		if err := uvm.SetLogSources(ctx); err != nil {
			e.WithError(err).Error("failed to set log sources")
		}
		if err := uvm.StartLogForwarding(ctx); err != nil {
			e.WithError(err).Error("failed to start log forwarding")
		}
	}

	return nil
}

// accept accepts a connection and then closes a listener. If the
// context becomes done or the utility VM terminates, the operation will be
// cancelled (but the listener will still be closed).
func (uvm *UtilityVM) accept(ctx context.Context, l net.Listener, closeListener bool) (net.Conn, error) {
	var conn net.Conn
	ch := make(chan error)
	go func() {
		var err error
		conn, err = l.Accept()
		ch <- err
	}()
	select {
	case err := <-ch:
		if closeListener {
			l.Close()
		}
		return conn, err
	case <-ctx.Done():
	case <-uvm.exitCh:
	}
	l.Close()
	err := <-ch
	if err == nil {
		return conn, err
	}
	// Prefer context error to VM error to accept error in order to return the
	// most useful error.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if uvm.exitErr != nil {
		return nil, uvm.exitErr
	}
	return nil, err
}
