package uvm

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/Microsoft/hcsshim/internal/gcs"
	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	"github.com/Microsoft/hcsshim/internal/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
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
//                 (e.g. by using a different encoding such as protobuf).
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
	if o, ok := err.(*net.OpError); ok {
		if s, ok := o.Err.(*os.SyscallError); ok {
			return s.Err == syscall.WSAECONNABORTED || s.Err == syscall.WSAECONNRESET
		}
	}
	return false
}

func parseLogrus(vmid string) func(r io.Reader) {
	return func(r io.Reader) {
		j := json.NewDecoder(r)
		e := logrus.NewEntry(logrus.StandardLogger())
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
				if err != io.EOF && !isDisconnectError(err) {
					logrus.WithFields(logrus.Fields{
						logfields.UVMID: vmid,
						logrus.ErrorKey: err,
					}).Error("gcs log read")
				}
				rest, _ := ioutil.ReadAll(io.MultiReader(j.Buffered(), r))
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
// for HvSockt so that the GCS can setup some registry keys that are required for running
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
		ParentAddress: gcs.WindowsGcsHvHostID.String(),
	}

	conSetupReq := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.GuestRequest{
			RequestType:  requesttype.Update,
			ResourceType: guestrequest.ResourceTypeHvSocket,
			Settings:     hvsocketAddress,
		},
	}

	if err = uvm.modify(ctx, conSetupReq); err != nil {
		return fmt.Errorf("failed to configure HVSOCK for external GCS: %s", err)
	}

	return nil
}

// Start synchronously starts the utility VM.
func (uvm *UtilityVM) Start(ctx context.Context) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	g, gctx := errgroup.WithContext(ctx)
	defer g.Wait()
	defer cancel()

	// Prepare to provide entropy to the init process in the background. This
	// must be done in a goroutine since, when using the internal bridge, the
	// call to Start() will block until the GCS launches, and this cannot occur
	// until the host accepts and closes the entropy connection.
	if uvm.entropyListener != nil {
		g.Go(func() error {
			conn, err := uvm.acceptAndClose(gctx, uvm.entropyListener)
			uvm.entropyListener = nil
			if err != nil {
				return fmt.Errorf("failed to connect to entropy socket: %s", err)
			}
			defer conn.Close()
			_, err = io.CopyN(conn, rand.Reader, entropyBytes)
			if err != nil {
				return fmt.Errorf("failed to write entropy: %s", err)
			}
			return nil
		})
	}

	if uvm.outputListener != nil {
		g.Go(func() error {
			conn, err := uvm.acceptAndClose(gctx, uvm.outputListener)
			uvm.outputListener = nil
			if err != nil {
				close(uvm.outputProcessingDone)
				return fmt.Errorf("failed to connect to log socket: %s", err)
			}
			go func() {
				uvm.outputHandler(conn)
				close(uvm.outputProcessingDone)
			}()
			return nil
		})
	}

	err = uvm.hcsSystem.Start(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			uvm.hcsSystem.Terminate(ctx)
			uvm.hcsSystem.Wait()
		}
	}()

	// Start waiting on the utility VM.
	uvm.exitCh = make(chan struct{})
	go func() {
		err := uvm.hcsSystem.Wait()
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
		conn, err := uvm.acceptAndClose(ctx, uvm.gcListener)
		uvm.gcListener = nil
		if err != nil {
			return fmt.Errorf("failed to connect to GCS: %s", err)
		}
		// Start the GCS protocol.
		gcc := &gcs.GuestConnectionConfig{
			Conn:     conn,
			Log:      log.G(ctx).WithField(logfields.UVMID, uvm.id),
			IoListen: gcs.HvsockIoListen(uvm.runtimeID),
		}
		uvm.gc, err = gcc.Connect(ctx)
		if err != nil {
			return err
		}
		uvm.guestCaps = *uvm.gc.Capabilities()
		uvm.protocol = uvm.gc.Protocol()

		// initial setup required for external GCS connection
		if err = uvm.configureHvSocketForGCS(ctx); err != nil {
			return fmt.Errorf("failed to do initial GCS setup: %s", err)
		}
	} else {
		// Cache the guest connection properties.
		properties, err := uvm.hcsSystem.Properties(ctx, schema1.PropertyTypeGuestConnection)
		if err != nil {
			return err
		}
		uvm.guestCaps = properties.GuestConnectionInfo.GuestDefinedCapabilities
		uvm.protocol = properties.GuestConnectionInfo.ProtocolVersion
	}
	return nil
}

// acceptAndClose accepts a connection and then closes a listener. If the
// context becomes done or the utility VM terminates, the operation will be
// cancelled (but the listener will still be closed).
func (uvm *UtilityVM) acceptAndClose(ctx context.Context, l net.Listener) (net.Conn, error) {
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
