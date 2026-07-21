//go:build windows

package gcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	"github.com/Microsoft/hcsshim/internal/ot"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const pipePortFmt = `\\.\pipe\gctest-port-%d`

func npipeIoListen(port uint32) (net.Listener, error) {
	return winio.ListenPipe(fmt.Sprintf(pipePortFmt, port), &winio.PipeConfig{
		MessageMode: true,
	})
}

func dialPort(port uint32) (net.Conn, error) {
	return winio.DialPipe(fmt.Sprintf(pipePortFmt, port), nil)
}

func simpleGcs(t *testing.T, rwc io.ReadWriteCloser) {
	t.Helper()
	defer rwc.Close()
	err := simpleGcsLoop(t, rwc)
	if err != nil {
		t.Error(err)
	}
}

func simpleGcsLoop(t *testing.T, rw io.ReadWriter) error {
	t.Helper()
	for {
		id, typ, b, err := readMessage(rw)
		if err != nil {
			// EOF, ErrClosedPipe, and ErrUnexpectedEOF can all surface when
			// the test's bridge closes the pipe mid-message during teardown.
			// Treat them all as a clean shutdown of the fake guest.
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, io.ErrUnexpectedEOF) {
				err = nil
			}
			return err
		}
		switch proc := prot.RPCProc(typ &^ prot.MsgTypeRequest); proc {
		case prot.RPCNegotiateProtocol:
			err := sendJSON(t, rw, prot.MsgTypeResponse|prot.MsgType(proc), id, &prot.NegotiateProtocolResponse{
				Version: protocolVersion,
				Capabilities: prot.GcsCapabilities{
					RuntimeOsType: "linux",
				},
			})
			if err != nil {
				return err
			}
		case prot.RPCCreate:
			err := sendJSON(t, rw, prot.MsgTypeResponse|prot.MsgType(proc), id, &prot.ContainerCreateResponse{})
			if err != nil {
				return err
			}
		case prot.RPCExecuteProcess:
			var req prot.ContainerExecuteProcess
			var params baseProcessParams
			req.Settings.ProcessParameters.Value = &params
			err := json.Unmarshal(b, &req)
			if err != nil {
				return err
			}
			var stdin, stdout, stderr net.Conn
			if params.CreateStdInPipe {
				stdin, err = dialPort(req.Settings.VsockStdioRelaySettings.StdIn)
				if err != nil {
					return err
				}
				defer stdin.Close()
			}
			if params.CreateStdOutPipe {
				stdout, err = dialPort(req.Settings.VsockStdioRelaySettings.StdOut)
				if err != nil {
					return err
				}
				defer stdout.Close()
			}
			if params.CreateStdErrPipe {
				stderr, err = dialPort(req.Settings.VsockStdioRelaySettings.StdErr)
				if err != nil {
					return err
				}
				defer stderr.Close()
			}
			if stdin != nil && stdout != nil {
				go func() {
					_, err := io.Copy(stdout, stdin)
					if err != nil {
						t.Error(err)
					}
					stdin.Close()
					stdout.Close()
				}()
			}
			err = sendJSON(t, rw, prot.MsgTypeResponse|prot.MsgType(proc), id, &prot.ContainerExecuteProcessResponse{
				ProcessID: 42,
			})
			if err != nil {
				return err
			}
		case prot.RPCWaitForProcess:
			// nothing
		case prot.RPCShutdownForced:
			var req prot.RequestBase
			err = json.Unmarshal(b, &req)
			if err != nil {
				return err
			}
			err = sendJSON(t, rw, prot.MsgTypeResponse|prot.MsgType(proc), id, &prot.ResponseBase{})
			if err != nil {
				return err
			}
			time.Sleep(50 * time.Millisecond)
			err = sendJSON(t, rw, prot.MsgType(prot.MsgTypeNotify|prot.ComputeSystem|prot.NotifyContainer), 0, &prot.ContainerNotification{
				RequestBase: prot.RequestBase{
					ContainerID: req.ContainerID,
				},
			})
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported msg %s", typ)
		}
	}
}

func connectGcs(ctx context.Context, t *testing.T) *GuestConnection {
	t.Helper()
	s, c := pipeConn()
	if ctx != context.Background() && ctx != context.TODO() {
		go func() {
			<-ctx.Done()
			c.Close()
		}()
	}
	// Join the fake-guest goroutine before the test returns so that any
	// t.Error it makes during teardown lands inside the test scope, instead
	// of panicking the runtime ("Fail in goroutine after Test... completed").
	done := make(chan struct{})
	go func() {
		defer close(done)
		simpleGcs(t, c)
	}()
	t.Cleanup(func() { <-done })
	gcc := &GuestConnectionConfig{
		Conn:     s,
		Log:      logrus.NewEntry(logrus.StandardLogger()),
		IoListen: npipeIoListen,
	}
	gc, err := gcc.Connect(context.Background(), true)
	if err != nil {
		c.Close()
		t.Fatal(err)
	}
	return gc
}

func TestGcsConnect(t *testing.T) {
	gc := connectGcs(context.Background(), t)
	defer gc.Close()
}

func TestGcsCreateContainer(t *testing.T) {
	gc := connectGcs(context.Background(), t)
	defer gc.Close()
	c, err := gc.CreateContainer(context.Background(), "foo", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

func TestGcsWaitContainer(t *testing.T) {
	gc := connectGcs(context.Background(), t)
	defer gc.Close()
	c, err := gc.CreateContainer(context.Background(), "foo", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	err = c.Terminate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	err = c.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func TestGcsWaitContainerBridgeTerminated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gc := connectGcs(ctx, t)
	c, err := gc.CreateContainer(context.Background(), "foo", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	cancel() // close the GCS connection
	err = c.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func TestGcsCreateProcess(t *testing.T) {
	gc := connectGcs(context.Background(), t)
	defer gc.Close()
	p, err := gc.CreateProcess(context.Background(), &baseProcessParams{
		CreateStdInPipe:  true,
		CreateStdOutPipe: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	stdin, stdout, _ := p.Stdio()
	_, err = stdin.Write(([]byte)("hello world"))
	if err != nil {
		t.Fatal(err)
	}
	err = p.CloseStdin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(stdout)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello world" {
		t.Errorf("unexpected: %q", string(b))
	}
}

func TestGcsWaitProcessBridgeTerminated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gc := connectGcs(ctx, t)
	defer gc.Close()
	p, err := gc.CreateProcess(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// There is a race condition here. gc.CreateProcess starts an AsyncRPC to wait on
	// the created process. However, the AsyncRPC sends the request message on rpcCh
	// and returns immediately (after the sendLoop reads that message). The test then
	// sometimes ends up canceling the context (which closes the communication pipes)
	// before the request message on rpcCh is processes and written on the pipe by
	// `sendRPC`. In that case we receive the "bridge write failed" error instead of
	// "bridge closed" error. To avoid this we put a small sleep here.
	time.Sleep(1 * time.Second)

	cancel()
	err = p.Wait()
	if err == nil || (!strings.Contains(err.Error(), "bridge closed") && !strings.Contains(err.Error(), "bridge write")) {
		t.Fatal("unexpected: ", err)
	}
}

func Test_makeRequestNoSpan(t *testing.T) {
	r := makeRequest(context.Background(), t.Name())

	if r.ContainerID != t.Name() {
		t.Fatalf("expected ContainerID: %q, got: %q", t.Name(), r.ContainerID)
	}
	var empty guid.GUID
	if r.ActivityID != empty {
		t.Fatalf("expected ActivityID empty, got: %q", r.ActivityID.String())
	}
	if len(r.OpenTelemetrySpanContext) != 0 {
		t.Fatal("expected nil span context")
	}
}

func setupTestOtel(t *testing.T) {
	t.Helper()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}

func Test_makeRequestWithSpan(t *testing.T) {
	setupTestOtel(t)

	ctx, span := ot.StartSpan(context.Background(), t.Name())
	defer span.End()
	r := makeRequest(ctx, t.Name())

	if r.ContainerID != t.Name() {
		t.Fatalf("expected ContainerID: %q, got: %q", t.Name(), r.ContainerID)
	}
	var empty guid.GUID
	if r.ActivityID != empty {
		t.Fatalf("expected ActivityID empty, got: %q", r.ActivityID.String())
	}
	if len(r.OpenTelemetrySpanContext) == 0 {
		t.Fatal("expected non-nil span context")
	}
	otsc := r.OpenTelemetrySpanContext
	if _, ok := otsc["traceparent"]; !ok {
		t.Fatalf("expected traceparent key in otsc, got: %v", otsc)
	}

	// Roundtrip: extract otsc and verify trace context matches.
	sc := span.SpanContext()
	extractedCtx := otel.GetTextMapPropagator().Extract(context.Background(), propagation.MapCarrier(otsc))
	extractedSC := trace.SpanContextFromContext(extractedCtx)
	if extractedSC.TraceID() != sc.TraceID() {
		t.Fatalf("roundtrip TraceID mismatch: got %s, want %s", extractedSC.TraceID(), sc.TraceID())
	}
	if extractedSC.SpanID() != sc.SpanID() {
		t.Fatalf("roundtrip SpanID mismatch: got %s, want %s", extractedSC.SpanID(), sc.SpanID())
	}
}

func Test_makeRequestWithSpan_TraceStateEmptyEntries(t *testing.T) {
	setupTestOtel(t)

	// Start a remote context span so we can forward trace state.
	ts := trace.TraceState{}
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceState: ts,
	})
	ctx, span := ot.StartSpanWithRemoteParent(context.Background(), t.Name(), parent)
	defer span.End()
	r := makeRequest(ctx, t.Name())

	if len(r.OpenTelemetrySpanContext) == 0 {
		t.Fatal("expected non-nil span context")
	}
	otsc := r.OpenTelemetrySpanContext
	// With empty trace state, traceparent should still be present.
	if _, ok := otsc["traceparent"]; !ok {
		t.Fatalf("expected traceparent key in otsc, got: %v", otsc)
	}
	// tracestate should not be present (empty).
	if ts, ok := otsc["tracestate"]; ok && ts != "" {
		t.Fatalf("expected no tracestate, got: %q", ts)
	}
}

func Test_makeRequestWithSpan_TraceStateEntries(t *testing.T) {
	setupTestOtel(t)

	// Start a remote context span so we can forward trace state.
	ts := trace.TraceState{}
	ts, err := ts.Insert("test", "test")
	if err != nil {
		t.Fatal(err)
	}
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceState: ts,
	})
	ctx, span := ot.StartSpanWithRemoteParent(context.Background(), t.Name(), parent)
	defer span.End()
	r := makeRequest(ctx, t.Name())

	if len(r.OpenTelemetrySpanContext) == 0 {
		t.Fatal("expected non-nil span context")
	}
	otsc := r.OpenTelemetrySpanContext
	if _, ok := otsc["traceparent"]; !ok {
		t.Fatalf("expected traceparent key in otsc, got: %v", otsc)
	}
	// tracestate should be present with the test entry.
	if tsVal, ok := otsc["tracestate"]; !ok || tsVal == "" {
		t.Fatalf("expected non-empty tracestate, got: %q", tsVal)
	}
}
