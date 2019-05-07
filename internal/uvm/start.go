package uvm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/microsoft/hcsshim/internal/logfields"
	"github.com/microsoft/hcsshim/internal/schema1"
	"github.com/sirupsen/logrus"
)

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

type acceptResult struct {
	c   net.Conn
	err error
}

func processOutput(ctx context.Context, l net.Listener, doneChan chan struct{}, handler OutputHandler) {
	defer close(doneChan)

	ch := make(chan acceptResult)
	go func() {
		c, err := l.Accept()
		ch <- acceptResult{c, err}
	}()

	select {
	case <-ctx.Done():
		l.Close()
		return
	case ar := <-ch:
		c, err := ar.c, ar.err
		l.Close()
		if err != nil {
			logrus.Error("accepting log socket: ", err)
			return
		}
		defer c.Close()

		handler(c)
	}
}

// Start synchronously starts the utility VM.
func (uvm *UtilityVM) Start() (err error) {
	op := "uvm::Start"
	log := logrus.WithFields(logrus.Fields{
		logfields.UVMID: uvm.id,
	})
	log.Debug(op + " - Begin Operation")
	defer func() {
		if err != nil {
			log.Data[logrus.ErrorKey] = err
			log.Error(op + " - End Operation - Error")
		} else {
			log.Debug(op + " - End Operation - Success")
		}
	}()

	if uvm.outputListener != nil {
		ctx, cancel := context.WithCancel(context.Background())
		go processOutput(ctx, uvm.outputListener, uvm.outputProcessingDone, uvm.outputHandler)
		uvm.outputProcessingCancel = cancel
		uvm.outputListener = nil
	}
	err = uvm.hcsSystem.Start()
	if err != nil {
		return err
	}

	// Cache the guest connection properties.
	properties, err := uvm.hcsSystem.Properties(schema1.PropertyTypeGuestConnection)
	if err != nil {
		uvm.hcsSystem.Terminate()
		uvm.hcsSystem.Wait()
		return err
	}
	uvm.guestCaps = properties.GuestConnectionInfo.GuestDefinedCapabilities
	uvm.protocol = properties.GuestConnectionInfo.ProtocolVersion
	return nil
}
