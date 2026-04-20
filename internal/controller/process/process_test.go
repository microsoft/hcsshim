//go:build windows && (lcow || wcow)

package process

import (
	"context"
	"errors"
	"testing"
	"time"

	containerdtypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/internal/controller/process/mocks"
	"github.com/Microsoft/hcsshim/internal/hcs"
)

const (
	testContainerID = "test-container-1234"
	testExecID      = "test-exec-5678"
	testPID         = 42
)

// errTest is a sentinel used in table-driven tests to verify error propagation.
var errTest = errors.New("test error")

func newSetup(t *testing.T) (*gomock.Controller, *mocks.MockProcessHost, *mocks.MockUpstreamIO, *Controller) {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	mockHost := mocks.NewMockProcessHost(mockCtrl)
	mockIO := mocks.NewMockUpstreamIO(mockCtrl)
	return mockCtrl, mockHost, mockIO, New(testContainerID, testExecID, mockHost, time.Second)
}

// TestNew_InitializesFields verifies that New sets all fields correctly and
// that the initial state is StateNotCreated with exit code 255.
func TestNew_InitializesFields(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)

	if controller.containerID != testContainerID {
		t.Errorf("containerID = %q; want %q", controller.containerID, testContainerID)
	}
	if controller.execID != testExecID {
		t.Errorf("execID = %q; want %q", controller.execID, testExecID)
	}
	if controller.state != StateNotCreated {
		t.Errorf("initial state = %s; want StateNotCreated", controller.state)
	}
	if controller.exitCode != 255 {
		t.Errorf("initial exitCode = %d; want 255", controller.exitCode)
	}
	if controller.exitedCh == nil {
		t.Fatal("exitedCh must not be nil after New")
	}
	if controller.Pid() != 0 {
		t.Errorf("initial Pid() = %d; want 0", controller.Pid())
	}
}

// TestCreate_WrongState verifies that Create rejects calls made outside StateNotCreated.
func TestCreate_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateCreated, StateRunning, StateTerminated}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			_, _, _, controller := newSetup(t)
			controller.state = state

			err := controller.Create(t.Context(), &CreateOptions{})
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("Create() = %v; want ErrFailedPrecondition", err)
			}
		})
	}
}

// TestCreate_TerminalWithStderr verifies that Create rejects the combination of
// terminal=true and a non-empty stderr path.
func TestCreate_TerminalWithStderr(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)

	err := controller.Create(t.Context(), &CreateOptions{
		Terminal: true,
		Stderr:   `\\.\pipe\some-stderr`,
	})
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Errorf("Create(terminal+stderr) = %v; want ErrFailedPrecondition", err)
	}
}

// TestCreate_Succeeds verifies that Create transitions to StateCreated and stores
// the bundle and process spec. Empty IO paths are used so no real named-pipe
// connections are attempted.
func TestCreate_Succeeds(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)

	spec := &specs.Process{Args: []string{"/bin/sh"}}
	opts := &CreateOptions{
		Bundle: "/test/bundle",
		Spec:   spec,
	}

	if err := controller.Create(t.Context(), opts); err != nil {
		t.Fatalf("Create() = %v; want nil", err)
	}
	if controller.state != StateCreated {
		t.Errorf("state = %s; want StateCreated", controller.state)
	}
	if controller.bundle != opts.Bundle {
		t.Errorf("bundle = %q; want %q", controller.bundle, opts.Bundle)
	}
	if controller.processSpec != spec {
		t.Error("processSpec not stored correctly after Create")
	}
	if controller.upstreamIO == nil {
		t.Error("upstreamIO must be non-nil after Create")
	}
}

// TestStart_WrongState verifies that Start rejects calls made outside StateCreated.
func TestStart_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateRunning, StateTerminated}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			_, _, _, controller := newSetup(t)
			controller.state = state

			pid, err := controller.Start(t.Context(), nil)
			if pid != -1 {
				t.Errorf("Start() pid = %d; want -1", pid)
			}
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("Start() = %v; want ErrFailedPrecondition", err)
			}
		})
	}
}

// TestStart_Succeeds verifies the happy path: Start returns the correct PID,
// transitions to StateRunning, publishes a TaskExit event, and the background
// goroutine transitions to StateTerminated after the mock process exits.
func TestStart_Succeeds(t *testing.T) {
	t.Parallel()
	mockCtrl, mockHost, mockIO, controller := newSetup(t)
	controller.upstreamIO = mockIO
	controller.state = StateCreated
	mockProc := mocks.NewMockProcess(mockCtrl)

	// cmd.Cmd struct is populated with IO readers/writers before Start is called.
	mockIO.EXPECT().Stdin().Return(nil)
	mockIO.EXPECT().Stdout().Return(nil)
	mockIO.EXPECT().Stderr().Return(nil)

	// cmd.Cmd.Start calls IsOCI and then CreateProcess.
	mockHost.EXPECT().IsOCI().Return(true)
	mockHost.EXPECT().CreateProcess(gomock.Any(), gomock.Any()).Return(mockProc, nil)

	// Pid is called once inside cmd.Cmd.Start for log enrichment, and once in
	// process.Controller.Start to record the OS-level PID.
	mockProc.EXPECT().Pid().Return(testPID).Times(2)

	// cmd.Cmd.Start always calls Stdio() to retrieve the process IO streams
	// before deciding whether to start relay goroutines.
	mockProc.EXPECT().Stdio().Return(nil, nil, nil)

	// cmd.Cmd.Wait internally calls Process.Wait, Process.ExitCode, and
	// Process.Close once. handleProcessExit delegates entirely to
	// execCmd.Wait rather than calling these directly.
	mockProc.EXPECT().Wait().Return(nil)
	mockProc.EXPECT().ExitCode().Return(0, nil)
	mockProc.EXPECT().Close().Return(nil)

	// upstreamIO cleanup at the end of handleProcessExit.
	mockIO.EXPECT().Close(gomock.Any())

	// Status(true) is called inside handleProcessExit to populate the exit event.
	mockIO.EXPECT().StdinPath().Return("")
	mockIO.EXPECT().StdoutPath().Return("")
	mockIO.EXPECT().StderrPath().Return("")
	mockIO.EXPECT().Terminal().Return(false)

	// Use a buffered channel so the goroutine never blocks on send.
	events := make(chan interface{}, 1)

	pid, err := controller.Start(t.Context(), events)
	if err != nil {
		t.Fatalf("Start() = %v; want nil", err)
	}
	if pid != testPID {
		t.Errorf("Start() pid = %d; want %d", pid, testPID)
	}
	if controller.State() != StateRunning {
		t.Errorf("state after Start = %s; want StateRunning", controller.State())
	}

	// Block until handleProcessExit goroutine finishes so all mock expectations
	// are satisfied before the gomock controller runs Finish.
	controller.Wait(t.Context())

	if controller.State() != StateTerminated {
		t.Errorf("state after exit = %s; want StateTerminated", controller.State())
	}

	// Verify that a TaskExit event was published.
	select {
	case event := <-events:
		if event == nil {
			t.Error("received nil event; want TaskExit")
		}
	default:
		t.Error("expected a TaskExit event in events channel; got none")
	}
}

// TestStart_HostCreateProcessFails verifies that a CreateProcess error causes
// Start to abort the controller and transition it to StateTerminated.
func TestStart_HostCreateProcessFails(t *testing.T) {
	t.Parallel()
	_, mockHost, mockIO, controller := newSetup(t)
	controller.upstreamIO = mockIO
	controller.state = StateCreated

	// IO readers/writers are read before cmd.Cmd.Start is invoked.
	mockIO.EXPECT().Stdin().Return(nil)
	mockIO.EXPECT().Stdout().Return(nil)
	mockIO.EXPECT().Stderr().Return(nil)

	// CreateProcess fails; cmd.Cmd.Start returns the error.
	mockHost.EXPECT().IsOCI().Return(true)
	mockHost.EXPECT().CreateProcess(gomock.Any(), gomock.Any()).Return(nil, errTest)

	// abortInternal is called, which releases upstreamIO.
	mockIO.EXPECT().Close(gomock.Any())

	pid, err := controller.Start(t.Context(), nil)
	if pid != -1 {
		t.Errorf("Start() pid = %d; want -1", pid)
	}
	if !errors.Is(err, errTest) {
		t.Errorf("Start() = %v; want errTest", err)
	}
	if controller.State() != StateTerminated {
		t.Errorf("state = %s; want StateTerminated", controller.State())
	}
}

// TestKill_NotCreatedState verifies that Kill on a process that was never
// created transitions it directly to StateTerminated without error. Because
// upstreamIO has not been populated yet, abortInternal must tolerate a nil
// upstreamIO and not panic.
func TestKill_NotCreatedState(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)
	// upstreamIO intentionally left nil — Create was never called.

	if err := controller.Kill(t.Context(), nil); err != nil {
		t.Fatalf("Kill(NotCreated) = %v; want nil", err)
	}
	if controller.state != StateTerminated {
		t.Errorf("state = %s; want StateTerminated", controller.state)
	}
	// exitedCh must be closed so any waiters are unblocked.
	select {
	case <-controller.exitedCh:
	default:
		t.Error("exitedCh should be closed after Kill(NotCreated)")
	}
}

// TestKill_CreatedState verifies that Kill on a created-but-not-started process
// triggers abortInternal, transitioning the controller to StateTerminated.
func TestKill_CreatedState(t *testing.T) {
	t.Parallel()
	_, _, mockIO, controller := newSetup(t)
	controller.upstreamIO = mockIO
	controller.state = StateCreated
	mockIO.EXPECT().Close(gomock.Any())

	if err := controller.Kill(t.Context(), nil); err != nil {
		t.Fatalf("Kill(Created) = %v; want nil", err)
	}
	if controller.state != StateTerminated {
		t.Errorf("state = %s; want StateTerminated", controller.state)
	}
	// exitedCh must be closed so any waiters are unblocked.
	select {
	case <-controller.exitedCh:
	default:
		t.Error("exitedCh should be closed after Kill(Created)")
	}
}

// TestKill_TerminatedState verifies that Kill on an already terminated process
// is a no-op and returns nil.
func TestKill_TerminatedState(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)
	controller.state = StateTerminated

	if err := controller.Kill(t.Context(), nil); err != nil {
		t.Errorf("Kill(Terminated) = %v; want nil", err)
	}
}

// TestKill_RunningState_Signal verifies all signal-delivery outcomes when Kill
// is called with non-nil signal options on a running process.
func TestKill_RunningState_Signal(t *testing.T) {
	t.Parallel()
	signalOpts := struct{ sig int }{sig: 15}

	tests := []struct {
		name        string
		isDelivered bool
		signalErr   error
		wantErr     bool
		wantErrIs   error
	}{
		{
			name:        "signal delivered",
			isDelivered: true,
			signalErr:   nil,
			wantErr:     false,
		},
		{
			name:        "signal not delivered",
			isDelivered: false,
			signalErr:   nil,
			wantErr:     true,
			wantErrIs:   errdefs.ErrNotFound,
		},
		{
			name:        "signal error propagated",
			isDelivered: false,
			signalErr:   errTest,
			wantErr:     true,
			wantErrIs:   errTest,
		},
		{
			name:        "already stopped is treated as success",
			isDelivered: false,
			signalErr:   hcs.ErrProcessAlreadyStopped,
			wantErr:     false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			mockCtrl, _, _, controller := newSetup(t)
			mockProc := mocks.NewMockProcess(mockCtrl)

			controller.state = StateRunning
			controller.process = mockProc

			mockProc.EXPECT().Signal(gomock.Any(), signalOpts).Return(testCase.isDelivered, testCase.signalErr)

			err := controller.Kill(t.Context(), signalOpts)
			if (err != nil) != testCase.wantErr {
				t.Errorf("Kill() error = %v; wantErr = %v", err, testCase.wantErr)
			}
			if testCase.wantErrIs != nil && !errors.Is(err, testCase.wantErrIs) {
				t.Errorf("Kill() = %v; want errors.Is(%v)", err, testCase.wantErrIs)
			}
		})
	}
}

// TestKill_RunningState_Terminate verifies all terminate-delivery outcomes when
// Kill is called with nil signal options on a running process.
func TestKill_RunningState_Terminate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		isDelivered bool
		killErr     error
		wantErr     bool
		wantErrIs   error
	}{
		{
			name:        "terminate delivered",
			isDelivered: true,
			killErr:     nil,
			wantErr:     false,
		},
		{
			name:        "terminate not delivered",
			isDelivered: false,
			killErr:     nil,
			wantErr:     true,
			wantErrIs:   errdefs.ErrNotFound,
		},
		{
			name:        "terminate error propagated",
			isDelivered: false,
			killErr:     errTest,
			wantErr:     true,
			wantErrIs:   errTest,
		},
		{
			name:        "already stopped is treated as success",
			isDelivered: false,
			killErr:     hcs.ErrVmcomputeAlreadyStopped,
			wantErr:     false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			mockCtrl, _, _, controller := newSetup(t)
			mockProc := mocks.NewMockProcess(mockCtrl)

			controller.state = StateRunning
			controller.process = mockProc

			// nil signalOptions triggers the legacy Kill (terminate) path.
			mockProc.EXPECT().Kill(gomock.Any()).Return(testCase.isDelivered, testCase.killErr)

			err := controller.Kill(t.Context(), nil)
			if (err != nil) != testCase.wantErr {
				t.Errorf("Kill() error = %v; wantErr = %v", err, testCase.wantErr)
			}
			if testCase.wantErrIs != nil && !errors.Is(err, testCase.wantErrIs) {
				t.Errorf("Kill() = %v; want errors.Is(%v)", err, testCase.wantErrIs)
			}
		})
	}
}

// TestResizeConsole_WrongState verifies that ResizeConsole fails with
// ErrFailedPrecondition when the process is not in StateRunning.
func TestResizeConsole_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateCreated, StateTerminated}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			_, _, _, controller := newSetup(t)
			controller.state = state

			err := controller.ResizeConsole(t.Context(), 80, 24)
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("ResizeConsole() = %v; want ErrFailedPrecondition", err)
			}
		})
	}
}

// TestResizeConsole_NonTerminal verifies that ResizeConsole fails when the
// process was not started with a pseudo-TTY.
func TestResizeConsole_NonTerminal(t *testing.T) {
	t.Parallel()
	_, _, mockIO, controller := newSetup(t)
	controller.upstreamIO = mockIO
	controller.state = StateRunning

	mockIO.EXPECT().Terminal().Return(false)

	err := controller.ResizeConsole(t.Context(), 80, 24)
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Errorf("ResizeConsole(non-TTY) = %v; want ErrFailedPrecondition", err)
	}
}

// TestResizeConsole_Result verifies the happy path and error propagation for
// ResizeConsole when the process is running with a pseudo-TTY.
func TestResizeConsole_Result(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		width     uint32
		height    uint32
		resizeErr error
		wantErr   bool
	}{
		{
			name:      "succeeds",
			width:     80,
			height:    24,
			resizeErr: nil,
			wantErr:   false,
		},
		{
			name:      "error propagated",
			width:     100,
			height:    50,
			resizeErr: errTest,
			wantErr:   true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			mockCtrl, _, mockIO, controller := newSetup(t)
			mockProc := mocks.NewMockProcess(mockCtrl)
			controller.upstreamIO = mockIO
			controller.state = StateRunning
			controller.process = mockProc

			mockIO.EXPECT().Terminal().Return(true)
			mockProc.EXPECT().ResizeConsole(gomock.Any(), uint16(testCase.width), uint16(testCase.height)).Return(testCase.resizeErr)

			err := controller.ResizeConsole(t.Context(), testCase.width, testCase.height)
			if (err != nil) != testCase.wantErr {
				t.Errorf("ResizeConsole() error = %v; wantErr = %v", err, testCase.wantErr)
			}
			if testCase.resizeErr != nil && !errors.Is(err, testCase.resizeErr) {
				t.Errorf("ResizeConsole() = %v; want errors.Is(%v)", err, testCase.resizeErr)
			}
		})
	}
}

// TestCloseIO verifies that CloseIO is a no-op when the process has not been
// created (upstreamIO is nil) and forwards to upstreamIO.CloseStdin for all
// other states.
func TestCloseIO(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		state         State
		hasUpstreamIO bool
	}{
		{"NotCreated", StateNotCreated, false},
		{"Created", StateCreated, true},
		{"Running", StateRunning, true},
		{"Terminated", StateTerminated, true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, _, mockIO, controller := newSetup(t)
			controller.state = testCase.state

			if testCase.hasUpstreamIO {
				controller.upstreamIO = mockIO
				mockIO.EXPECT().CloseStdin(gomock.Any())
			}

			controller.CloseIO(t.Context())
		})
	}
}

// TestWait_ProcessExit verifies that Wait returns promptly once exitedCh is closed.
func TestWait_ProcessExit(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)
	// Simulate a process that has already exited.
	close(controller.exitedCh)

	finished := make(chan struct{})
	go func() {
		controller.Wait(t.Context())
		close(finished)
	}()

	select {
	case <-finished:
		// success
	case <-time.After(time.Second):
		t.Fatal("Wait() did not return after process exited")
	}
}

// TestWait_ContextCancelled verifies that Wait returns when the context is
// cancelled even if exitedCh is never closed.
func TestWait_ContextCancelled(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)
	// Do not close exitedCh; Wait must return via context cancellation.

	cancelCtx, cancel := context.WithCancel(t.Context())

	finished := make(chan struct{})
	go func() {
		controller.Wait(cancelCtx)
		close(finished)
	}()

	cancel()

	select {
	case <-finished:
		// success
	case <-time.After(time.Second):
		t.Fatal("Wait() did not return after context was cancelled")
	}
}

// TestDelete_WrongState verifies that Delete rejects calls made outside of valid states.
func TestDelete_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateRunning}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			_, _, _, controller := newSetup(t)
			controller.state = state

			err := controller.Delete(t.Context())
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("Delete() = %v; want ErrFailedPrecondition", err)
			}
		})
	}
}

// TestDelete_CreatedState_Succeeds verifies that Delete on a created-but-never-started
// process transitions to StateTerminated with exit code 0, releases upstreamIO,
// and closes exitedCh.
func TestDelete_CreatedState_Succeeds(t *testing.T) {
	t.Parallel()
	_, _, mockIO, controller := newSetup(t)
	controller.upstreamIO = mockIO
	controller.state = StateCreated
	mockIO.EXPECT().Close(gomock.Any())

	if err := controller.Delete(t.Context()); err != nil {
		t.Fatalf("Delete() = %v; want nil", err)
	}
	if controller.state != StateTerminated {
		t.Errorf("state = %s; want StateTerminated", controller.state)
	}
	if controller.exitCode != 0 {
		t.Errorf("exitCode = %d; want 0", controller.exitCode)
	}
	// exitedCh must be closed so any concurrent Wait calls unblock.
	select {
	case <-controller.exitedCh:
		// success
	default:
		t.Error("exitedCh should be closed after Delete")
	}
}

// TestDelete_TerminatedState_Succeeds verifies that Delete on an already-terminated
// process is a no-op and returns nil.
func TestDelete_TerminatedState_Succeeds(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)
	controller.state = StateTerminated

	if err := controller.Delete(t.Context()); err != nil {
		t.Fatalf("Delete() = %v; want nil", err)
	}
}

// TestStatus_NotCreated verifies that Status returns UNKNOWN status when the
// controller has not been created yet.
func TestStatus_NotCreated(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)

	status := controller.Status(false)

	if status.ID != testContainerID {
		t.Errorf("ID = %q; want %q", status.ID, testContainerID)
	}
	if status.ExecID != testExecID {
		t.Errorf("ExecID = %q; want %q", status.ExecID, testExecID)
	}
	if status.Status != containerdtypes.Status_UNKNOWN {
		t.Errorf("Status = %v; want UNKNOWN", status.Status)
	}
}

// TestStatus_Created_Detailed verifies that Status in StateCreated with
// isDetailed=true populates all IO path and bundle fields.
func TestStatus_Created_Detailed(t *testing.T) {
	t.Parallel()
	_, _, mockIO, controller := newSetup(t)
	controller.upstreamIO = mockIO
	controller.state = StateCreated
	controller.bundle = "/test/bundle"

	mockIO.EXPECT().StdinPath().Return("/pipe/stdin")
	mockIO.EXPECT().StdoutPath().Return("/pipe/stdout")
	mockIO.EXPECT().StderrPath().Return("/pipe/stderr")
	mockIO.EXPECT().Terminal().Return(false)

	status := controller.Status(true)

	if status.Status != containerdtypes.Status_CREATED {
		t.Errorf("Status = %v; want CREATED", status.Status)
	}
	if status.Bundle != "/test/bundle" {
		t.Errorf("Bundle = %q; want /test/bundle", status.Bundle)
	}
	if status.Stdin != "/pipe/stdin" {
		t.Errorf("Stdin = %q; want /pipe/stdin", status.Stdin)
	}
	if status.Stdout != "/pipe/stdout" {
		t.Errorf("Stdout = %q; want /pipe/stdout", status.Stdout)
	}
	if status.Stderr != "/pipe/stderr" {
		t.Errorf("Stderr = %q; want /pipe/stderr", status.Stderr)
	}
	if status.Terminal {
		t.Error("Terminal = true; want false")
	}
}

// TestStatus_Running verifies that Status reflects RUNNING state and the stored PID.
// Status(false) does not access upstreamIO so no IO mock expectations are needed.
func TestStatus_Running(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)
	controller.state = StateRunning
	controller.processID = testPID

	status := controller.Status(false)

	if status.Status != containerdtypes.Status_RUNNING {
		t.Errorf("Status = %v; want RUNNING", status.Status)
	}
	if int(status.Pid) != testPID {
		t.Errorf("Pid = %d; want %d", status.Pid, testPID)
	}
}

// TestStatus_Terminated_Detailed verifies that Status in StateTerminated with
// isDetailed=true includes the exit code and IO paths.
func TestStatus_Terminated_Detailed(t *testing.T) {
	t.Parallel()
	const wantExitCode = uint32(1)

	_, _, mockIO, controller := newSetup(t)
	controller.upstreamIO = mockIO
	controller.state = StateTerminated
	controller.exitCode = wantExitCode

	mockIO.EXPECT().StdinPath().Return("")
	mockIO.EXPECT().StdoutPath().Return("")
	mockIO.EXPECT().StderrPath().Return("")
	mockIO.EXPECT().Terminal().Return(false)

	status := controller.Status(true)

	if status.Status != containerdtypes.Status_STOPPED {
		t.Errorf("Status = %v; want STOPPED", status.Status)
	}
	if status.ExitStatus != wantExitCode {
		t.Errorf("ExitStatus = %d; want %d", status.ExitStatus, wantExitCode)
	}
}

// TestState_Returns verifies that the State accessor reflects every lifecycle state.
func TestState_Returns(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)

	for _, wantState := range []State{StateNotCreated, StateCreated, StateRunning, StateTerminated} {
		controller.state = wantState
		if got := controller.State(); got != wantState {
			t.Errorf("State() = %v; want %v", got, wantState)
		}
	}
}

// TestPid_Returns verifies that Pid returns the OS-level process ID stored
// during Start.
func TestPid_Returns(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)
	controller.processID = testPID

	if got := controller.Pid(); got != testPID {
		t.Errorf("Pid() = %d; want %d", got, testPID)
	}
}
