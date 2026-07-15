//go:build windows && (lcow || wcow)

package process

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/Microsoft/hcsshim/internal/controller/process/mocks"
	procsave "github.com/Microsoft/hcsshim/internal/controller/process/save"
	"github.com/Microsoft/hcsshim/internal/cow"
)

const (
	testBundle     = "/test/bundle"
	testStdinPort  = uint32(101)
	testStdoutPort = uint32(102)
	testStderrPort = uint32(103)
	testWaitCallID = int64(99)
)

// TestSave_WrongState verifies that only a running process can be saved.
func TestSave_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateCreated, StateTerminated, StateDestinationMigrating, StateSourceMigrating}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			_, _, _, controller := newSetup(t)
			controller.state = state

			if _, err := controller.Save(t.Context()); err == nil {
				t.Errorf("Save() = nil; want error for state %s", state)
			}
		})
	}
}

// TestSave_Succeeds verifies that a running process is serialized into a
// payload carrying its identity, live IO ports, and (for an exec) its spec.
func TestSave_Succeeds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		spec *specs.Process
	}{
		{name: "exec with spec", spec: &specs.Process{Args: []string{"/bin/sh"}}},
		{name: "init without spec", spec: nil},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			mockCtrl, _, _, controller := newSetup(t)
			mockProc := mocks.NewMockProcess(mockCtrl)
			controller.state = StateRunning
			controller.process = mockProc
			controller.processID = testPID
			controller.bundle = testBundle
			controller.processSpec = testCase.spec

			mockProc.EXPECT().MigrationState().Return(cow.MigrationState{
				StdinPort:  testStdinPort,
				StdoutPort: testStdoutPort,
				StderrPort: testStderrPort,
				WaitCallID: testWaitCallID,
			})

			env, err := controller.Save(t.Context())
			if err != nil {
				t.Fatalf("Save() = %v; want nil", err)
			}
			if env.GetTypeUrl() != procsave.TypeURL {
				t.Errorf("TypeUrl = %q; want %q", env.GetTypeUrl(), procsave.TypeURL)
			}

			// Decode the payload and verify the serialized fields.
			got := &procsave.Payload{}
			if err := proto.Unmarshal(env.GetValue(), got); err != nil {
				t.Fatalf("Unmarshal payload = %v; want nil", err)
			}
			if got.GetSchemaVersion() != procsave.SchemaVersion {
				t.Errorf("SchemaVersion = %d; want %d", got.GetSchemaVersion(), procsave.SchemaVersion)
			}
			if got.GetExecID() != testExecID {
				t.Errorf("ExecID = %q; want %q", got.GetExecID(), testExecID)
			}
			if got.GetPid() != int32(testPID) {
				t.Errorf("Pid = %d; want %d", got.GetPid(), testPID)
			}
			if got.GetBundle() != testBundle {
				t.Errorf("Bundle = %q; want %q", got.GetBundle(), testBundle)
			}
			if got.GetStdinPort() != testStdinPort || got.GetStdoutPort() != testStdoutPort || got.GetStderrPort() != testStderrPort {
				t.Errorf("ports = (%d,%d,%d); want (%d,%d,%d)", got.GetStdinPort(), got.GetStdoutPort(), got.GetStderrPort(), testStdinPort, testStdoutPort, testStderrPort)
			}
			if got.GetWaitCallID() != testWaitCallID {
				t.Errorf("WaitCallID = %d; want %d", got.GetWaitCallID(), testWaitCallID)
			}
			// The spec is present only for an exec process.
			if (len(got.GetOciProcessSpecJson()) > 0) != (testCase.spec != nil) {
				t.Errorf("spec present = %v; want %v", len(got.GetOciProcessSpecJson()) > 0, testCase.spec != nil)
			}
			// A successful save freezes the source until it is resumed or terminated.
			if controller.state != StateSourceMigrating {
				t.Errorf("state = %s; want StateSourceMigrating", controller.state)
			}
		})
	}
}

// TestImport_InvalidEnvelope verifies that Import rejects malformed or
// incompatible envelopes.
func TestImport_InvalidEnvelope(t *testing.T) {
	t.Parallel()

	// A payload stamped with an unsupported schema version.
	badVersion, err := proto.Marshal(&procsave.Payload{SchemaVersion: procsave.SchemaVersion + 1})
	if err != nil {
		t.Fatalf("marshal bad-version payload = %v", err)
	}

	tests := []struct {
		name string
		env  *anypb.Any
	}{
		{name: "nil envelope", env: nil},
		{name: "wrong type url", env: &anypb.Any{TypeUrl: "type.microsoft.com/other", Value: nil}},
		{name: "undecodable value", env: &anypb.Any{TypeUrl: procsave.TypeURL, Value: []byte{0x08, 0xff}}},
		{name: "schema version mismatch", env: &anypb.Any{TypeUrl: procsave.TypeURL, Value: badVersion}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Import(t.Context(), testCase.env, testContainerID); err == nil {
				t.Errorf("Import() = nil; want error")
			}
		})
	}
}

// TestImport_Succeeds verifies that Import reconstructs the controller in the
// migrating state with the saved fields, restoring the spec only for an exec.
func TestImport_Succeeds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		spec *specs.Process
	}{
		{name: "exec with spec", spec: &specs.Process{Args: []string{"/bin/sh"}}},
		{name: "init without spec", spec: nil},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			env := buildEnvelope(t, testCase.spec)

			controller, err := Import(t.Context(), env, testContainerID)
			if err != nil {
				t.Fatalf("Import() = %v; want nil", err)
			}
			if controller.state != StateDestinationMigrating {
				t.Errorf("state = %s; want StateDestinationMigrating", controller.state)
			}
			if controller.containerID != testContainerID {
				t.Errorf("containerID = %q; want %q", controller.containerID, testContainerID)
			}
			if controller.execID != testExecID {
				t.Errorf("execID = %q; want %q", controller.execID, testExecID)
			}
			if controller.processID != testPID {
				t.Errorf("processID = %d; want %d", controller.processID, testPID)
			}
			if controller.bundle != testBundle {
				t.Errorf("bundle = %q; want %q", controller.bundle, testBundle)
			}
			if controller.stdinPort != testStdinPort || controller.stdoutPort != testStdoutPort || controller.stderrPort != testStderrPort {
				t.Errorf("ports = (%d,%d,%d); want (%d,%d,%d)", controller.stdinPort, controller.stdoutPort, controller.stderrPort, testStdinPort, testStdoutPort, testStderrPort)
			}
			if controller.waitCallID != testWaitCallID {
				t.Errorf("waitCallID = %d; want %d", controller.waitCallID, testWaitCallID)
			}
			if controller.exitedCh == nil {
				t.Error("exitedCh must be non-nil after Import")
			}
			if !reflect.DeepEqual(controller.processSpec, testCase.spec) {
				t.Errorf("processSpec = %+v; want %+v", controller.processSpec, testCase.spec)
			}
		})
	}
}

// TestPatch_InvalidArgs verifies that Patch rejects missing options or an
// empty destination container id.
func TestPatch_InvalidArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		opts        *CreateOptions
		containerID string
	}{
		{name: "nil options", opts: nil, containerID: testContainerID},
		{name: "empty container id", opts: &CreateOptions{}, containerID: ""},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, _, _, controller := newSetup(t)
			controller.state = StateDestinationMigrating

			err := controller.Patch(t.Context(), testCase.containerID, testCase.opts)
			if !errors.Is(err, errdefs.ErrInvalidArgument) {
				t.Errorf("Patch() = %v; want ErrInvalidArgument", err)
			}
		})
	}
}

// TestPatch_WrongState verifies that Patch only operates on a destination-migrating process.
func TestPatch_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateCreated, StateRunning, StateTerminated, StateSourceMigrating}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			_, _, _, controller := newSetup(t)
			controller.state = state

			err := controller.Patch(t.Context(), testContainerID, &CreateOptions{})
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("Patch() = %v; want ErrFailedPrecondition", err)
			}
		})
	}
}

// TestPatch_TerminalWithStderr verifies that Patch rejects the terminal+stderr
// combination a fresh create would also refuse.
func TestPatch_TerminalWithStderr(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)
	controller.state = StateDestinationMigrating

	err := controller.Patch(t.Context(), testContainerID, &CreateOptions{
		Terminal: true,
		Stderr:   `\\.\pipe\some-stderr`,
	})
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Errorf("Patch(terminal+stderr) = %v; want ErrFailedPrecondition", err)
	}
}

// TestPatch_Succeeds verifies that Patch adopts the destination container and
// opens fresh IO while leaving the process in the migrating state. Empty IO
// paths are used so no real named-pipe connections are attempted.
func TestPatch_Succeeds(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)
	controller.state = StateDestinationMigrating

	const destContainerID = "dest-container-9999"
	opts := &CreateOptions{Bundle: testBundle}

	if err := controller.Patch(t.Context(), destContainerID, opts); err != nil {
		t.Fatalf("Patch() = %v; want nil", err)
	}
	if controller.state != StateDestinationMigrating {
		t.Errorf("state = %s; want StateDestinationMigrating", controller.state)
	}
	if controller.containerID != destContainerID {
		t.Errorf("containerID = %q; want %q", controller.containerID, destContainerID)
	}
	if controller.bundle != testBundle {
		t.Errorf("bundle = %q; want %q", controller.bundle, testBundle)
	}
	if controller.upstreamIO == nil {
		t.Error("upstreamIO must be non-nil after Patch")
	}
}

// TestResume_WrongState verifies that Resume only operates on a migrating
// process and rejects other states before touching the host.
func TestResume_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateCreated, StateRunning, StateTerminated}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			_, _, _, controller := newSetup(t)
			controller.state = state

			err := controller.Resume(t.Context(), nil, nil)
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("Resume() = %v; want ErrFailedPrecondition", err)
			}
		})
	}
}

// TestResume_SourceRollback verifies that resuming a source-migrating process
// lifts the freeze and returns it to running without touching the host.
func TestResume_SourceRollback(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)
	controller.state = StateSourceMigrating

	// nil host/events are unused: the live process and IO stay intact.
	if err := controller.Resume(t.Context(), nil, nil); err != nil {
		t.Fatalf("Resume() = %v; want nil", err)
	}
	if controller.state != StateRunning {
		t.Errorf("state = %s; want StateRunning", controller.state)
	}
}

// TestAbortMigrated_NoOp verifies that AbortMigrated leaves a non-migrating
// process untouched.
func TestAbortMigrated_NoOp(t *testing.T) {
	t.Parallel()
	otherStates := []State{StateNotCreated, StateCreated, StateRunning, StateTerminated, StateSourceMigrating}

	for _, state := range otherStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			_, _, _, controller := newSetup(t)
			controller.state = state

			controller.AbortMigrated(t.Context())
			if controller.state != state {
				t.Errorf("state = %s; want unchanged %s", controller.state, state)
			}
		})
	}
}

// TestAbortMigrated_Succeeds verifies that AbortMigrated terminates a migrating
// process, recording exit code 137 and unblocking waiters.
func TestAbortMigrated_Succeeds(t *testing.T) {
	t.Parallel()
	_, _, _, controller := newSetup(t)
	controller.state = StateDestinationMigrating
	// upstreamIO intentionally nil — abort must tolerate it.

	controller.AbortMigrated(t.Context())

	if controller.state != StateTerminated {
		t.Errorf("state = %s; want StateTerminated", controller.state)
	}
	if controller.exitCode != 137 {
		t.Errorf("exitCode = %d; want 137", controller.exitCode)
	}
	select {
	case <-controller.exitedCh:
	default:
		t.Error("exitedCh should be closed after AbortMigrated")
	}
}

// TestSaveImport_RoundTrip verifies that a payload produced by Save restores an
// equivalent process via Import.
func TestSaveImport_RoundTrip(t *testing.T) {
	t.Parallel()
	mockCtrl, _, _, src := newSetup(t)
	mockProc := mocks.NewMockProcess(mockCtrl)
	src.state = StateRunning
	src.process = mockProc
	src.processID = testPID
	src.bundle = testBundle
	src.processSpec = &specs.Process{Args: []string{"/bin/sh"}}

	mockProc.EXPECT().MigrationState().Return(cow.MigrationState{
		StdinPort:  testStdinPort,
		StdoutPort: testStdoutPort,
		StderrPort: testStderrPort,
		WaitCallID: testWaitCallID,
	})

	env, err := src.Save(t.Context())
	if err != nil {
		t.Fatalf("Save() = %v; want nil", err)
	}

	dst, err := Import(t.Context(), env, testContainerID)
	if err != nil {
		t.Fatalf("Import() = %v; want nil", err)
	}

	if dst.execID != src.execID || dst.processID != src.processID || dst.bundle != src.bundle {
		t.Errorf("restored identity mismatch: got (%q,%d,%q); want (%q,%d,%q)", dst.execID, dst.processID, dst.bundle, src.execID, src.processID, src.bundle)
	}
	if dst.stdinPort != testStdinPort || dst.stdoutPort != testStdoutPort || dst.stderrPort != testStderrPort || dst.waitCallID != testWaitCallID {
		t.Errorf("restored migration state mismatch: ports=(%d,%d,%d) wait=%d", dst.stdinPort, dst.stdoutPort, dst.stderrPort, dst.waitCallID)
	}
	if !reflect.DeepEqual(dst.processSpec, src.processSpec) {
		t.Errorf("restored spec = %+v; want %+v", dst.processSpec, src.processSpec)
	}
}

// buildEnvelope marshals a payload with the standard test fields and the given
// spec into an envelope Import can consume.
func buildEnvelope(t *testing.T, spec *specs.Process) *anypb.Any {
	t.Helper()
	payload := &procsave.Payload{
		SchemaVersion:  procsave.SchemaVersion,
		ExecID:         testExecID,
		Pid:            int32(testPID),
		Bundle:         testBundle,
		IoRetryTimeout: durationpb.New(time.Second),
		StdinPort:      testStdinPort,
		StdoutPort:     testStdoutPort,
		StderrPort:     testStderrPort,
		WaitCallID:     testWaitCallID,
	}
	if spec != nil {
		raw, err := json.Marshal(spec)
		if err != nil {
			t.Fatalf("marshal spec = %v", err)
		}
		payload.OciProcessSpecJson = raw
	}

	value, err := proto.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload = %v", err)
	}
	return &anypb.Any{TypeUrl: procsave.TypeURL, Value: value}
}
