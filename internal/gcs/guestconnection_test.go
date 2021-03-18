package gcs

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"go.opencensus.io/trace/tracestate"
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
	defer rwc.Close()
	err := simpleGcsLoop(t, rwc)
	if err != nil {
		t.Error(err)
	}
}

func simpleGcsLoop(t *testing.T, rw io.ReadWriter) error {
	for {
		id, typ, b, err := readMessage(rw)
		if err != nil {
			if err == io.EOF || err == io.ErrClosedPipe {
				err = nil
			}
			return err
		}
		switch proc := rpcProc(typ &^ msgTypeRequest); proc {
		case rpcNegotiateProtocol:
			err := sendJSON(t, rw, msgTypeResponse|msgType(proc), id, &negotiateProtocolResponse{
				Version: protocolVersion,
				Capabilities: gcsCapabilities{
					RuntimeOsType: "linux",
				},
			})
			if err != nil {
				return err
			}
		case rpcCreate:
			err := sendJSON(t, rw, msgTypeResponse|msgType(proc), id, &containerCreateResponse{})
			if err != nil {
				return err
			}
		case rpcExecuteProcess:
			var req containerExecuteProcess
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
			err = sendJSON(t, rw, msgTypeResponse|msgType(proc), id, &containerExecuteProcessResponse{
				ProcessID: 42,
			})
			if err != nil {
				return err
			}
		case rpcWaitForProcess:
			// nothing
		case rpcShutdownForced:
			var req requestBase
			err = json.Unmarshal(b, &req)
			if err != nil {
				return err
			}
			err = sendJSON(t, rw, msgTypeResponse|msgType(proc), id, &responseBase{})
			if err != nil {
				return err
			}
			time.Sleep(50 * time.Millisecond)
			err = sendJSON(t, rw, msgType(msgTypeNotify|notifyContainer), 0, &containerNotification{
				requestBase: requestBase{
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
	s, c := pipeConn()
	if ctx != context.Background() && ctx != context.TODO() {
		go func() {
			<-ctx.Done()
			c.Close()
		}()
	}
	go simpleGcs(t, c)
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
	b, err := ioutil.ReadAll(stdout)
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
	cancel()
	err = p.Wait()
	if err == nil || !strings.Contains(err.Error(), "bridge closed") {
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
	if r.OpenCensusSpanContext != nil {
		t.Fatal("expected nil span context")
	}
}

func Test_makeRequestWithSpan(t *testing.T) {
	ctx, span := trace.StartSpan(context.Background(), t.Name())
	defer span.End()
	r := makeRequest(ctx, t.Name())

	if r.ContainerID != t.Name() {
		t.Fatalf("expected ContainerID: %q, got: %q", t.Name(), r.ContainerID)
	}
	var empty guid.GUID
	if r.ActivityID != empty {
		t.Fatalf("expected ActivityID empty, got: %q", r.ActivityID.String())
	}
	if r.OpenCensusSpanContext == nil {
		t.Fatal("expected non-nil span context")
	}
	sc := span.SpanContext()
	encodedTraceID := hex.EncodeToString(sc.TraceID[:])
	if r.OpenCensusSpanContext.TraceID != encodedTraceID {
		t.Fatalf("expected encoded TraceID: %q, got: %q", encodedTraceID, r.OpenCensusSpanContext.TraceID)
	}
	encodedSpanID := hex.EncodeToString(sc.SpanID[:])
	if r.OpenCensusSpanContext.SpanID != encodedSpanID {
		t.Fatalf("expected encoded SpanID: %q, got: %q", encodedSpanID, r.OpenCensusSpanContext.SpanID)
	}
	encodedTraceOptions := uint32(sc.TraceOptions)
	if r.OpenCensusSpanContext.TraceOptions != encodedTraceOptions {
		t.Fatalf("expected encoded TraceOptions: %v, got: %v", encodedTraceOptions, r.OpenCensusSpanContext.TraceOptions)
	}
	if r.OpenCensusSpanContext.Tracestate != "" {
		t.Fatalf("expected encoded TraceState: '', got: %q", r.OpenCensusSpanContext.Tracestate)
	}
}

func Test_makeRequestWithSpan_TraceStateEmptyEntries(t *testing.T) {
	// Start a remote context span so we can forward trace state.
	ts, err := tracestate.New(nil)
	if err != nil {
		t.Fatalf("failed to make test Tracestate")
	}
	parent := trace.SpanContext{
		Tracestate: ts,
	}
	ctx, span := trace.StartSpanWithRemoteParent(context.Background(), t.Name(), parent)
	defer span.End()
	r := makeRequest(ctx, t.Name())

	if r.OpenCensusSpanContext == nil {
		t.Fatal("expected non-nil span context")
	}
	if r.OpenCensusSpanContext.Tracestate != "" {
		t.Fatalf("expected encoded TraceState: '', got: %q", r.OpenCensusSpanContext.Tracestate)
	}
}

func Test_makeRequestWithSpan_TraceStateEntries(t *testing.T) {
	// Start a remote context span so we can forward trace state.
	ts, err := tracestate.New(nil, tracestate.Entry{Key: "test", Value: "test"})
	if err != nil {
		t.Fatalf("failed to make test Tracestate")
	}
	parent := trace.SpanContext{
		Tracestate: ts,
	}
	ctx, span := trace.StartSpanWithRemoteParent(context.Background(), t.Name(), parent)
	defer span.End()
	r := makeRequest(ctx, t.Name())

	if r.OpenCensusSpanContext == nil {
		t.Fatal("expected non-nil span context")
	}
	encodedTraceState := base64.StdEncoding.EncodeToString([]byte(`[{"Key":"test","Value":"test"}]`))
	if r.OpenCensusSpanContext.Tracestate != encodedTraceState {
		t.Fatalf("expected encoded TraceState: %q, got: %q", encodedTraceState, r.OpenCensusSpanContext.Tracestate)
	}
}
