//go:build windows

package main

import (
	"context"
	"errors"
	"math/rand"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func setupTestHcsTask(t *testing.T) (*hcsTask, *testShimExec, *testShimExec) {
	t.Helper()
	initExec := newTestShimExec(t.Name(), t.Name(), int(rand.Int31()))
	lt := &hcsTask{
		events: newFakePublisher(),
		id:     t.Name(),
		init:   initExec,
		closed: make(chan struct{}),
	}
	secondExecID := strconv.Itoa(rand.Int())
	secondExec := newTestShimExec(t.Name(), secondExecID, int(rand.Int31()))
	lt.execs.Store(secondExecID, secondExec)
	return lt, initExec, secondExec
}

func Test_hcsTask_ID(t *testing.T) {
	lt, _, _ := setupTestHcsTask(t)

	if lt.ID() != t.Name() {
		t.Fatalf("expect ID: '%s', got: '%s'", t.Name(), lt.ID())
	}
}

func Test_hcsTask_GetExec_Empty_Success(t *testing.T) {
	lt, i, _ := setupTestHcsTask(t)

	e, err := lt.GetExec("")
	if err != nil {
		t.Fatalf("should not have failed with error: %v", err)
	}
	if i != e {
		t.Fatal("should of returned the init exec on empty")
	}
}

func Test_hcsTask_GetExec_UnknownExecID_Error(t *testing.T) {
	lt, _, _ := setupTestHcsTask(t)

	e, err := lt.GetExec("shouldnotmatch")

	verifyExpectedError(t, e, err, errdefs.ErrNotFound)
}

func Test_hcsTask_GetExec_2ndID_Success(t *testing.T) {
	lt, _, second := setupTestHcsTask(t)

	e, err := lt.GetExec(second.id)
	if err != nil {
		t.Fatalf("should not have failed with error: %v", err)
	}
	if second != e {
		t.Fatal("should of returned the second exec")
	}
}

func Test_hcsTask_KillExec_UnknownExecID_Error(t *testing.T) {
	lt, _, _ := setupTestHcsTask(t)

	err := lt.KillExec(context.TODO(), "thisshouldnotmatch", 0xf, false)

	verifyExpectedError(t, nil, err, errdefs.ErrNotFound)
}

func Test_hcsTask_KillExec_InitExecID_Unexited2ndExec_Success(t *testing.T) {
	lt, init, second := setupTestHcsTask(t)

	err := lt.KillExec(context.TODO(), "", 0xf, false)
	if err != nil {
		t.Fatalf("should not have failed, got: %v", err)
	}
	if init.state != shimExecStateExited {
		t.Fatalf("init should be in exited state got: %v", init.state)
	}
	// A real platform would take this down when the pid namespace or silo goes
	// down. For the test verify the shim did not issue the signal.
	if second.state != shimExecStateCreated {
		t.Fatalf("2nd exec should be in created state, got: %v", second.state)
	}
}

func Test_hcsTask_KillExec_InitExecID_All_Success(t *testing.T) {
	lt, init, second := setupTestHcsTask(t)

	err := lt.KillExec(context.TODO(), "", 0xf, true)
	if err != nil {
		t.Fatalf("should not have failed, got: %v", err)
	}
	if init.state != shimExecStateExited {
		t.Fatalf("init should be in exited state got: %v", init.state)
	}
	if second.state != shimExecStateExited {
		t.Fatalf("2nd exec should be in exited state got: %v", second.state)
	}
}

func Test_hcsTask_KillExec_2ndExecID_Success(t *testing.T) {
	lt, _, second := setupTestHcsTask(t)

	err := lt.KillExec(context.TODO(), second.id, 0xf, false)
	if err != nil {
		t.Fatalf("should not have failed, got: %v", err)
	}
	if second.state != shimExecStateExited {
		t.Fatalf("2nd exec should be in exited state got: %v", second.state)
	}
}

func Test_hcsTask_KillExec_2ndExecID_All_Error(t *testing.T) {
	lt, _, second := setupTestHcsTask(t)

	err := lt.KillExec(context.TODO(), second.id, 0xf, true)

	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
}

func verifyDeleteFailureValues(t *testing.T, pid int, status uint32, at time.Time) {
	t.Helper()
	if pid != 0 {
		t.Fatalf("pid expected '0' got: '%d'", pid)
	}
	if status != 0 {
		t.Fatalf("status expected '0' got: '%d'", status)
	}
	if !at.IsZero() {
		t.Fatalf("at expected 'zero' got: '%v'", at)
	}
}

func verifyDeleteSuccessValues(t *testing.T, pid int, status uint32, at time.Time, e *testShimExec) {
	t.Helper()
	if pid != e.pid {
		t.Fatalf("pid expected '%d' got: '%d'", e.pid, pid)
	}
	if status != e.status {
		t.Fatalf("status expected '%d' got: '%d'", e.status, status)
	}
	if !at.Equal(e.at) {
		t.Fatalf("at expected '%v' got: '%v'", e.at, at)
	}
}

func Test_hcsTask_DeleteExec_UnknownExecID_Error(t *testing.T) {
	lt, _, _ := setupTestHcsTask(t)

	pid, status, at, err := lt.DeleteExec(context.TODO(), "thisshouldnotmatch")
	verifyExpectedError(t, nil, err, errdefs.ErrNotFound)
	verifyDeleteFailureValues(t, pid, status, at)
}

func Test_hcsTask_DeleteExec_InitExecID_CreatedState_Success(t *testing.T) {
	lt, init, second := setupTestHcsTask(t)
	// remove the 2nd exec so we just check without it.
	lt.execs.Delete(second.id)

	// Simulate waitInitExit() closing the host
	close(lt.closed)
	// try to delete the init exec
	pid, status, at, err := lt.DeleteExec(context.TODO(), "")

	if err != nil {
		t.Fatalf("expected nil err got: %v", err)
	}
	verifyDeleteSuccessValues(t, pid, status, at, init)
}

func Test_hcsTask_DeleteExec_InitExecID_RunningState_Error(t *testing.T) {
	lt, init, second := setupTestHcsTask(t)
	// remove the 2nd exec so we just check without it.
	lt.execs.Delete(second.id)

	// Start the init exec
	_ = init.Start(context.TODO())

	// Simulate waitInitExit() closing the host
	close(lt.closed)
	// try to delete the init exec
	pid, status, at, err := lt.DeleteExec(context.TODO(), "")

	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
	verifyDeleteFailureValues(t, pid, status, at)
}

func Test_hcsTask_DeleteExec_InitExecID_ExitedState_Success(t *testing.T) {
	lt, init, second := setupTestHcsTask(t)
	// remove the 2nd exec so we just check without it.
	lt.execs.Delete(second.id)

	_ = init.Kill(context.TODO(), 0xf)

	// Simulate waitInitExit() closing the host
	close(lt.closed)
	// try to delete the init exec
	pid, status, at, err := lt.DeleteExec(context.TODO(), "")

	if err != nil {
		t.Fatalf("expected nil err got: %v", err)
	}
	verifyDeleteSuccessValues(t, pid, status, at, init)
}

func Test_hcsTask_DeleteExec_InitExecID_2ndExec_CreatedState_Error(t *testing.T) {
	lt, init, second := setupTestHcsTask(t)

	// start the init exec (required to have 2nd exec)
	_ = init.Start(context.TODO())

	// Simulate waitInitExit() closing the host
	close(lt.closed)
	// try to delete the init exec
	pid, status, at, err := lt.DeleteExec(context.TODO(), "")

	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
	verifyDeleteFailureValues(t, pid, status, at)
	if second.state != shimExecStateExited {
		t.Fatalf("2nd exec should be in exited state, got: %v", second.state)
	}
}

func Test_hcsTask_DeleteExec_InitExecID_2ndExec_RunningState_Error(t *testing.T) {
	lt, init, second := setupTestHcsTask(t)

	// start the init exec (required to have 2nd exec)
	_ = init.Start(context.TODO())

	// put the 2nd exec into the running state
	_ = second.Start(context.TODO())

	// Simulate waitInitExit() closing the host
	close(lt.closed)
	// try to delete the init exec
	pid, status, at, err := lt.DeleteExec(context.TODO(), "")

	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
	verifyDeleteFailureValues(t, pid, status, at)
	if second.state != shimExecStateExited {
		t.Fatalf("2nd exec should be in exited state, got: %v", second.state)
	}
}

func Test_hcsTask_DeleteExec_InitExecID_2ndExec_ExitedState_Success(t *testing.T) {
	lt, init, second := setupTestHcsTask(t)

	// put the init exec into the exited state
	_ = init.Kill(context.TODO(), 0xf)
	// put the 2nd exec into the exited state
	_ = second.Kill(context.TODO(), 0xf)

	// Simulate waitInitExit() closing the host
	close(lt.closed)
	// try to delete the init exec
	pid, status, at, err := lt.DeleteExec(context.TODO(), "")

	if err != nil {
		t.Fatalf("expected nil err got: %v", err)
	}
	verifyDeleteSuccessValues(t, pid, status, at, init)
}

func Test_hcsTask_DeleteExec_2ndExecID_CreatedState_Success(t *testing.T) {
	lt, init, second := setupTestHcsTask(t)

	// start the init exec (required to have 2nd exec)
	_ = init.Start(context.TODO())

	// try to delete the 2nd exec
	pid, status, at, err := lt.DeleteExec(context.TODO(), second.id)

	if err != nil {
		t.Fatalf("expected nil err got: %v", err)
	}
	verifyDeleteSuccessValues(t, pid, status, at, second)
}

func Test_hcsTask_DeleteExec_2ndExecID_RunningState_Error(t *testing.T) {
	lt, init, second := setupTestHcsTask(t)

	// start the init exec (required to have 2nd exec)
	_ = init.Start(context.TODO())

	// put the 2nd exec into the running state
	_ = second.Start(context.TODO())

	// try to delete the 2nd exec
	pid, status, at, err := lt.DeleteExec(context.TODO(), second.id)

	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
	verifyDeleteFailureValues(t, pid, status, at)
}

func Test_hcsTask_DeleteExec_2ndExecID_ExitedState_Success(t *testing.T) {
	lt, init, second := setupTestHcsTask(t)

	// start the init exec (required to have 2nd exec)
	_ = init.Kill(context.TODO(), 0xf)

	// put the 2nd exec into the exited state
	_ = second.Kill(context.TODO(), 0xf)

	// try to delete the 2nd exec
	pid, status, at, err := lt.DeleteExec(context.TODO(), second.id)

	if err != nil {
		t.Fatalf("expected nil err got: %v", err)
	}
	verifyDeleteSuccessValues(t, pid, status, at, second)
}

func Test_handleProcessArgsForIsolatedJobContainer(t *testing.T) {
	ntAuthorityUser := `NT AUTHORITY\SYSTEM`
	testUserName := "testUser"

	tests := []struct {
		name             string
		taskAnnotations  map[string]string
		specs            *specs.Process
		expectedCmdLine  string
		expectedArgs     []string
		expectedUsername string
	}{
		{
			name:            "CommandLine starts with 'cmd' (lowercase) – unchanged",
			specs:           &specs.Process{CommandLine: "cmd /c dir"},
			expectedCmdLine: "cmd /c dir",
		},
		{
			name:            "CommandLine starts with 'CMD' (uppercase) – unchanged",
			specs:           &specs.Process{CommandLine: "CMD /C whoami"},
			expectedCmdLine: "CMD /C whoami",
		},
		{
			name:            "CommandLine starts with 'cmd.exe' – unchanged",
			specs:           &specs.Process{CommandLine: "cmd.exe /c ipconfig"},
			expectedCmdLine: "cmd.exe /c ipconfig",
		},
		{
			name:            "CommandLine plain – gets prefixed with 'cmd /c '",
			specs:           &specs.Process{CommandLine: "echo hello"},
			expectedCmdLine: "cmd /c echo hello",
		},
		{
			name:            "CommandLine mixed case 'CmD' – unchanged",
			specs:           &specs.Process{CommandLine: "CmD /c ping 127.0.0.1"},
			expectedCmdLine: "CmD /c ping 127.0.0.1",
		},
		{
			name:            "CommandLine has leading spaces before 'cmd' – unchanged",
			specs:           &specs.Process{CommandLine: "  cmd /c echo spaced"},
			expectedCmdLine: "  cmd /c echo spaced",
		},
		{
			name:            "CommandLine has leading spaces before 'CMD' – unchanged",
			specs:           &specs.Process{CommandLine: "   CMD /C echo spaced"},
			expectedCmdLine: "   CMD /C echo spaced",
		},
		{
			name:            "CommandLine whitespace-only – gets prefixed preserving spaces",
			specs:           &specs.Process{CommandLine: "    "},
			expectedCmdLine: "cmd /c     ",
		},
		{
			name:         "Args plain – gets ['cmd','/c',...] prefix",
			specs:        &specs.Process{Args: []string{"echo", "hello"}},
			expectedArgs: []string{"cmd", "/c", "echo", "hello"},
		},
		{
			name:         "Args already start with 'CMD' (uppercase) – unchanged",
			specs:        &specs.Process{Args: []string{"CMD", "/C", "echo", "hi"}},
			expectedArgs: []string{"CMD", "/C", "echo", "hi"},
		},
		{
			name:         "Args already start with 'cmd' (lowercase) – unchanged",
			specs:        &specs.Process{Args: []string{"cmd", "/c", "type", "file.txt"}},
			expectedArgs: []string{"cmd", "/c", "type", "file.txt"},
		},
		{
			name:         "Args first element mixed case 'Cmd' – unchanged",
			specs:        &specs.Process{Args: []string{"Cmd", "/c", "echo", "hi"}},
			expectedArgs: []string{"Cmd", "/c", "echo", "hi"},
		},
		{
			name:         "Args first element has leading/trailing spaces '  CMD  ' – unchanged (trimmed comparison)",
			specs:        &specs.Process{Args: []string{"  CMD  ", "/C", "echo", "trimmed"}},
			expectedArgs: []string{"  CMD  ", "/C", "echo", "trimmed"},
		},
		{
			name:            "Empty CommandLine and empty Args – unchanged",
			specs:           &specs.Process{},
			expectedCmdLine: "",
		},
		{
			name:            "Empty CommandLine and empty slice Args – unchanged (empty slice preserved)",
			specs:           &specs.Process{Args: []string{}},
			expectedCmdLine: "",
			expectedArgs:    []string{},
		},
		{
			name:            "CommandLine 'cmdkey ...' – not cmd, gets prefixed",
			specs:           &specs.Process{CommandLine: "cmdkey /list"},
			expectedCmdLine: "cmd /c cmdkey /list",
		},
		{
			name:            "CommandLine 'cmdtool foo' – not cmd, gets prefixed",
			specs:           &specs.Process{CommandLine: "cmdtool foo"},
			expectedCmdLine: "cmd /c cmdtool foo",
		},
		{
			name:         "Args starts with 'cmd.exe' – unchanged",
			specs:        &specs.Process{Args: []string{"cmd.exe", "/c", "echo", "hi"}},
			expectedArgs: []string{"cmd.exe", "/c", "echo", "hi"},
		},
		{
			name:         "Args starts with 'cmdkey' – not cmd, gets prefixed",
			specs:        &specs.Process{Args: []string{"cmdkey", "/list"}},
			expectedArgs: []string{"cmd", "/c", "cmdkey", "/list"},
		},
		{
			name:            "CommandLine absolute path to cmd.exe – unchanged",
			specs:           &specs.Process{CommandLine: `C:\Windows\System32\cmd.exe /c echo hi`},
			expectedCmdLine: `C:\Windows\System32\cmd.exe /c echo hi`,
		},
		{
			name:         "Args[0] absolute path to cmd.exe – unchanged",
			specs:        &specs.Process{Args: []string{`C:\Windows\System32\cmd.exe`, "/c", "echo", "hi"}},
			expectedArgs: []string{`C:\Windows\System32\cmd.exe`, "/c", "echo", "hi"},
		},
		{
			name:  "Nil Process – no panic",
			specs: nil,
		},
		// --- User inheritance behavior ---
		{
			name:             "HostProcessInheritUser=true – sets Username to NT AUTHORITY\\SYSTEM",
			taskAnnotations:  map[string]string{annotations.HostProcessInheritUser: "true"},
			specs:            &specs.Process{},
			expectedUsername: ntAuthorityUser,
		},
		{
			name:             "HostProcessInheritUser=false – does not set Username",
			taskAnnotations:  map[string]string{annotations.HostProcessInheritUser: "false"},
			specs:            &specs.Process{User: specs.User{Username: testUserName}},
			expectedUsername: testUserName,
		},
		{
			name:             "HostProcessInheritUser missing – does not set Username",
			taskAnnotations:  map[string]string{},
			specs:            &specs.Process{User: specs.User{Username: testUserName}},
			expectedUsername: testUserName,
		},
		{
			name:             "HostProcessInheritUser=true – overrides preexisting Username",
			taskAnnotations:  map[string]string{annotations.HostProcessInheritUser: "true"},
			specs:            &specs.Process{User: specs.User{Username: testUserName}},
			expectedUsername: ntAuthorityUser,
		},
		{
			name:             "HostProcessInheritUser=false – preserves preexisting Username",
			taskAnnotations:  map[string]string{annotations.HostProcessInheritUser: "false"},
			specs:            &specs.Process{User: specs.User{Username: testUserName}},
			expectedUsername: testUserName,
		},
		{
			name: "Nil annotation map – safely no change to Username",
			// Note: Annotations=nil is fine; indexing a nil map returns zero value
			taskAnnotations:  nil,
			specs:            &specs.Process{User: specs.User{Username: testUserName}},
			expectedUsername: testUserName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskSpec := &specs.Spec{Annotations: tt.taskAnnotations}

			handleProcessArgsForIsolatedJobContainer(taskSpec, tt.specs)

			if tt.specs == nil {
				return
			}

			if tt.specs.CommandLine != tt.expectedCmdLine {
				t.Errorf("CommandLine mismatch:  got:  %q  want: %q", tt.specs.CommandLine, tt.expectedCmdLine)
			}
			if !reflect.DeepEqual(tt.specs.Args, tt.expectedArgs) {
				t.Errorf("Args mismatch:  got:  %#v  want: %#v", tt.specs.Args, tt.expectedArgs)
			}
			if tt.specs.User.Username != tt.expectedUsername {
				t.Errorf("Username mismatch:  got:  %q  want: %q", tt.specs.User.Username, tt.expectedUsername)
			}
		})
	}
}

func u64(v uint64) *uint64 { return &v }
func u16(v uint16) *uint16 { return &v }

func Test_isValidWindowsCPUResources(t *testing.T) {
	affinity := []specs.WindowsCPUGroupAffinity{{Group: 0, Mask: 0x3}}
	for _, tt := range []struct {
		name string
		c    *specs.WindowsCPUResources
		want bool
	}{
		{"count only", &specs.WindowsCPUResources{Count: u64(2)}, true},
		{"shares only", &specs.WindowsCPUResources{Shares: u16(100)}, true},
		{"maximum only", &specs.WindowsCPUResources{Maximum: u16(5000)}, true},
		{"count and shares", &specs.WindowsCPUResources{Count: u64(2), Shares: u16(100)}, false},
		{"affinity only", &specs.WindowsCPUResources{Affinity: affinity}, true},
		{"affinity with count", &specs.WindowsCPUResources{Count: u64(2), Affinity: affinity}, true},
		{"empty", &specs.WindowsCPUResources{}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidWindowsCPUResources(tt.c); got != tt.want {
				t.Fatalf("isValidWindowsCPUResources(%+v) = %v, want %v", tt.c, got, tt.want)
			}
		})
	}
}

func Test_hcsTask_updateWCOWContainerCPUAffinity_NoAffinity(t *testing.T) {
	ht := &hcsTask{id: t.Name()}
	// An empty affinity slice is a no-op and must not require an HCS-backed container.
	if err := ht.updateWCOWContainerCPUAffinity(context.Background(), nil); err != nil {
		t.Fatalf("expected nil error for empty affinity, got %v", err)
	}
}

func Test_hcsTask_updateWCOWContainerCPUAffinity_XenonNotImplemented(t *testing.T) {
	ht := &hcsTask{id: t.Name(), host: &uvm.UtilityVM{}}
	err := ht.updateWCOWContainerCPUAffinity(context.Background(), []specs.WindowsCPUGroupAffinity{{Group: 0, Mask: 0x1}})
	if !errors.Is(err, errdefs.ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented for hypervisor-isolated container, got %v", err)
	}
}
