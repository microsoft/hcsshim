//go:build windows

package vmutils

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/windows/mock"

	"github.com/Microsoft/go-winio/pkg/guid"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/windows"
)

const (
	testSnapshot      windows.Handle = 1000
	testProcessHandle windows.Handle = 2000
	testToken         windows.Token  = 3000
	testPID           uint32         = 1234

	// Second set of handles for multi-process test scenarios where two
	// vmmem entries appear in the same snapshot.
	testProcessHandle2 windows.Handle = 4000
	testToken2         windows.Token  = 5000
	testPID2           uint32         = 5678
)

var (
	testVMID = guid.GUID{
		Data1: 0x12345678,
		Data2: 0x1234,
		Data3: 0x5678,
		Data4: [8]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0},
	}
	testVMIDStr   = testVMID.String()
	mockSID       = &windows.SID{}
	mockTokenUser = &windows.Tokenuser{
		User: windows.SIDAndAttributes{Sid: mockSID},
	}
	errNoMoreProcesses = errors.New("no more processes")
)

func TestLookupVMMEM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupMock     func(*mockHelper)
		expectError   bool
		errorContains string
	}{
		{
			name: "successful lookup - vmmem.exe",
			setupMock: func(h *mockHelper) {
				h.expectSuccessfulMatch("vmmem.exe")
			},
		},
		{
			name: "successful lookup - vmmem without extension",
			setupMock: func(h *mockHelper) {
				h.expectSuccessfulMatch("vmmem")
			},
		},
		{
			name: "failed to create snapshot",
			setupMock: func(h *mockHelper) {
				h.m.EXPECT().
					CreateToolhelp32Snapshot(uint32(windows.TH32CS_SNAPPROCESS), uint32(0)).
					Return(windows.Handle(0), errors.New("access denied"))
			},
			expectError:   true,
			errorContains: "failed to find matching vmmem process",
		},
		{
			name: "no vmmem process found",
			setupMock: func(h *mockHelper) {
				h.expectSnapshot()
				h.expectProcess32(testPID, "explorer.exe")
				h.m.EXPECT().Process32Next(testSnapshot, gomock.Any()).Return(errNoMoreProcesses)
				h.expectCloseSnapshot()
			},
			expectError:   true,
			errorContains: "failed to find matching vmmem process",
		},
		{
			name: "vmmem found but wrong VM ID",
			setupMock: func(h *mockHelper) {
				h.expectSnapshot()
				h.expectProcess32(testPID, "vmmem.exe")
				h.expectOpenProcess(testPID, testProcessHandle, nil)
				h.expectOpenProcessToken(nil)
				h.expectGetTokenUser(mockTokenUser, nil)
				h.expectLookupAccount("DIFFERENT-VM-ID", "NT VIRTUAL MACHINE", nil)
				h.expectCloseToken()
				h.expectCloseProcessHandle()
				h.expectNoMoreProcesses()
				h.expectCloseSnapshot()
			},
			expectError:   true,
			errorContains: "failed to find matching vmmem process",
		},
		{
			name: "vmmem found but wrong domain",
			setupMock: func(h *mockHelper) {
				h.expectSnapshot()
				h.expectProcess32(testPID, "vmmem.exe")
				h.expectOpenProcess(testPID, testProcessHandle, nil)
				h.expectOpenProcessToken(nil)
				h.expectGetTokenUser(mockTokenUser, nil)
				h.expectLookupAccount(testVMIDStr, "WORKGROUP", nil)
				h.expectCloseToken()
				h.expectCloseProcessHandle()
				h.expectNoMoreProcesses()
				h.expectCloseSnapshot()
			},
			expectError:   true,
			errorContains: "failed to find matching vmmem process",
		},
		{
			name: "OpenProcess fails",
			setupMock: func(h *mockHelper) {
				h.expectSnapshot()
				h.expectProcess32(testPID, "vmmem.exe")
				h.expectOpenProcess(testPID, 0, errors.New("access denied"))
				h.expectNoMoreProcesses()
				h.expectCloseSnapshot()
			},
			expectError:   true,
			errorContains: "failed to find matching vmmem process",
		},
		{
			name: "OpenProcessToken fails",
			setupMock: func(h *mockHelper) {
				h.expectSnapshot()
				h.expectProcess32(testPID, "vmmem.exe")
				h.expectOpenProcess(testPID, testProcessHandle, nil)
				h.expectOpenProcessToken(errors.New("token access denied"))
				h.expectCloseProcessHandle()
				h.expectNoMoreProcesses()
				h.expectCloseSnapshot()
			},
			expectError:   true,
			errorContains: "failed to find matching vmmem process",
		},
		{
			name: "GetTokenUser fails",
			setupMock: func(h *mockHelper) {
				h.expectSnapshot()
				h.expectProcess32(testPID, "vmmem.exe")
				h.expectOpenProcess(testPID, testProcessHandle, nil)
				h.expectOpenProcessToken(nil)
				h.expectGetTokenUser(nil, errors.New("failed to get token user"))
				h.expectCloseToken()
				h.expectCloseProcessHandle()
				h.expectNoMoreProcesses()
				h.expectCloseSnapshot()
			},
			expectError:   true,
			errorContains: "failed to find matching vmmem process",
		},
		{
			name: "LookupAccount fails",
			setupMock: func(h *mockHelper) {
				h.expectSnapshot()
				h.expectProcess32(testPID, "vmmem.exe")
				h.expectOpenProcess(testPID, testProcessHandle, nil)
				h.expectOpenProcessToken(nil)
				h.expectGetTokenUser(mockTokenUser, nil)
				h.expectLookupAccount("", "", errors.New("lookup failed"))
				h.expectCloseToken()
				h.expectCloseProcessHandle()
				h.expectNoMoreProcesses()
				h.expectCloseSnapshot()
			},
			expectError:   true,
			errorContains: "failed to find matching vmmem process",
		},
		{
			name: "Process32First fails",
			setupMock: func(h *mockHelper) {
				h.expectSnapshot()
				h.m.EXPECT().
					Process32First(testSnapshot, gomock.Any()).
					Return(errors.New("failed to get first process"))
				h.expectCloseSnapshot()
			},
			expectError:   true,
			errorContains: "failed to find matching vmmem process",
		},

		// --- Multi-process and case-sensitivity edge cases ---

		{
			// Two vmmem processes in the snapshot. The first belongs to a
			// different VM; the function should close its handle and keep
			// scanning until it finds the one matching our VM ID.
			name: "skips wrong VM vmmem, matches second",
			setupMock: func(h *mockHelper) {
				h.expectSkipThenMatch(testVMIDStr)
			},
		},
		{
			// Multiple vmmem processes in the snapshot but none of them
			// belong to the VM we're looking for.
			name: "multiple vmmem processes, none match",
			setupMock: func(h *mockHelper) {
				h.expectMultipleVmmemNoneMatch()
			},
			expectError:   true,
			errorContains: "failed to find matching vmmem process",
		},
		{
			// The process name uses unusual casing ("VMMEM.EXE"). The code
			// does a case-insensitive comparison via strings.EqualFold, so
			// this should still match.
			name: "case-insensitive process name match",
			setupMock: func(h *mockHelper) {
				h.expectSnapshot()
				h.expectProcess32(testPID, "VMMEM.EXE")
				h.expectOpenProcess(testPID, testProcessHandle, nil)
				h.expectOpenProcessToken(nil)
				h.expectGetTokenUser(mockTokenUser, nil)
				h.expectLookupAccount(testVMIDStr, "NT VIRTUAL MACHINE", nil)
				h.expectCloseToken()
				h.expectNoMoreProcesses()
				h.expectCloseSnapshot()
			},
		},
		{
			// LookupAccount returns the VM ID in lowercase. The GUID
			// comparison uses strings.EqualFold, so casing shouldn't matter.
			name: "case-insensitive VM ID match",
			setupMock: func(h *mockHelper) {
				h.expectSnapshot()
				h.expectProcess32(testPID, "vmmem")
				h.expectOpenProcess(testPID, testProcessHandle, nil)
				h.expectOpenProcessToken(nil)
				h.expectGetTokenUser(mockTokenUser, nil)
				// Return the VM ID in lowercase — EqualFold should still match.
				h.expectLookupAccount(strings.ToLower(testVMIDStr), "nt virtual machine", nil)
				h.expectCloseToken()
				h.expectNoMoreProcesses()
				h.expectCloseSnapshot()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAPI := mock.NewMockAPI(ctrl)
			helper := &mockHelper{m: mockAPI}
			tt.setupMock(helper)

			handle, err := LookupVMMEM(context.Background(), testVMID, mockAPI)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, but got: %v", tt.errorContains, err)
				}
				if handle != 0 {
					t.Errorf("expected handle to be 0 on error, got: %v", handle)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if handle == 0 {
					t.Errorf("expected non-zero handle on success")
				}
			}
		})
	}
}

// TestAllProcessEntries verifies the allProcessEntries iterator independently
// from LookupVMMEM to ensure correct ordering, early termination cleanup,
// and faithful forwarding of ProcessEntry32 fields.
func TestAllProcessEntries(t *testing.T) {
	t.Parallel()
	type wantEntry struct {
		pid  uint32
		name string
	}

	tests := []struct {
		name       string
		setupMock  func(*mockHelper)
		breakAfter int // 0 = consume all entries; >0 = break after N
		want       []wantEntry
	}{
		{
			// Snapshot contains exactly one process; iterator should yield it once
			// and stop when Process32Next signals the end.
			name: "single process entry",
			setupMock: func(h *mockHelper) {
				gomock.InOrder(
					h.m.EXPECT().
						CreateToolhelp32Snapshot(uint32(windows.TH32CS_SNAPPROCESS), uint32(0)).
						Return(testSnapshot, nil),
					h.m.EXPECT().
						Process32First(testSnapshot, gomock.Any()).
						DoAndReturn(func(_ windows.Handle, pe *windows.ProcessEntry32) error {
							*pe = makeProcessEntry(100, "explorer.exe")
							return nil
						}),
					h.m.EXPECT().
						Process32Next(testSnapshot, gomock.Any()).
						Return(errNoMoreProcesses),
					h.m.EXPECT().CloseHandle(testSnapshot).Return(nil),
				)
			},
			want: []wantEntry{{100, "explorer.exe"}},
		},
		{
			// Three distinct processes in the snapshot; all must be yielded in
			// the exact order the OS returns them (First, Next, Next).
			name: "multiple process entries yielded in order",
			setupMock: func(h *mockHelper) {
				gomock.InOrder(
					h.m.EXPECT().
						CreateToolhelp32Snapshot(uint32(windows.TH32CS_SNAPPROCESS), uint32(0)).
						Return(testSnapshot, nil),
					h.m.EXPECT().
						Process32First(testSnapshot, gomock.Any()).
						DoAndReturn(func(_ windows.Handle, pe *windows.ProcessEntry32) error {
							*pe = makeProcessEntry(10, "init.exe")
							return nil
						}),
					h.m.EXPECT().
						Process32Next(testSnapshot, gomock.Any()).
						DoAndReturn(func(_ windows.Handle, pe *windows.ProcessEntry32) error {
							*pe = makeProcessEntry(20, "svchost.exe")
							return nil
						}),
					h.m.EXPECT().
						Process32Next(testSnapshot, gomock.Any()).
						DoAndReturn(func(_ windows.Handle, pe *windows.ProcessEntry32) error {
							*pe = makeProcessEntry(30, "explorer.exe")
							return nil
						}),
					h.m.EXPECT().
						Process32Next(testSnapshot, gomock.Any()).
						Return(errNoMoreProcesses),
					h.m.EXPECT().CloseHandle(testSnapshot).Return(nil),
				)
			},
			want: []wantEntry{
				{10, "init.exe"},
				{20, "svchost.exe"},
				{30, "explorer.exe"},
			},
		},
		{
			// Consumer breaks out of the range loop after the first entry.
			// The snapshot handle must still be closed (gomock enforces this).
			name: "consumer breaks early — snapshot still closed",
			setupMock: func(h *mockHelper) {
				gomock.InOrder(
					h.m.EXPECT().
						CreateToolhelp32Snapshot(uint32(windows.TH32CS_SNAPPROCESS), uint32(0)).
						Return(testSnapshot, nil),
					h.m.EXPECT().
						Process32First(testSnapshot, gomock.Any()).
						DoAndReturn(func(_ windows.Handle, pe *windows.ProcessEntry32) error {
							*pe = makeProcessEntry(10, "init.exe")
							return nil
						}),
					// No Process32Next expected — consumer breaks before it's called.
					h.m.EXPECT().CloseHandle(testSnapshot).Return(nil),
				)
			},
			breakAfter: 1,
			want:       []wantEntry{{10, "init.exe"}},
		},
		{
			// Verify that the yielded ProcessEntry32 carries the correct PID
			// and ExeFile name end-to-end through the iterator.
			name: "validates ProcessEntry32 fields",
			setupMock: func(h *mockHelper) {
				gomock.InOrder(
					h.m.EXPECT().
						CreateToolhelp32Snapshot(uint32(windows.TH32CS_SNAPPROCESS), uint32(0)).
						Return(testSnapshot, nil),
					h.m.EXPECT().
						Process32First(testSnapshot, gomock.Any()).
						DoAndReturn(func(_ windows.Handle, pe *windows.ProcessEntry32) error {
							*pe = makeProcessEntry(testPID, "vmmem.exe")
							return nil
						}),
					h.m.EXPECT().
						Process32Next(testSnapshot, gomock.Any()).
						Return(errNoMoreProcesses),
					h.m.EXPECT().CloseHandle(testSnapshot).Return(nil),
				)
			},
			want: []wantEntry{{testPID, "vmmem.exe"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAPI := mock.NewMockAPI(ctrl)
			helper := &mockHelper{m: mockAPI}
			tt.setupMock(helper)

			var got []wantEntry
			i := 0
			for pe := range allProcessEntries(context.Background(), mockAPI) {
				name := windows.UTF16ToString(pe.ExeFile[:])
				got = append(got, wantEntry{pe.ProcessID, name})
				i++
				if tt.breakAfter > 0 && i >= tt.breakAfter {
					break
				}
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d", len(got), len(tt.want))
			}
			for idx, w := range tt.want {
				if got[idx].pid != w.pid {
					t.Errorf("entry[%d] PID: got %d, want %d", idx, got[idx].pid, w.pid)
				}
				if got[idx].name != w.name {
					t.Errorf("entry[%d] ExeFile: got %q, want %q", idx, got[idx].name, w.name)
				}
			}
		})
	}
}

// makeProcessEntry creates a ProcessEntry32 with the given name and PID.
func makeProcessEntry(pid uint32, exeName string) windows.ProcessEntry32 {
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	pe.ProcessID = pid
	utf16Name, _ := windows.UTF16FromString(exeName)
	copy(pe.ExeFile[:], utf16Name)
	return pe
}

// mockHelper provides common mock setup operations to reduce duplication.
type mockHelper struct {
	m *mock.MockAPI
}

func (h *mockHelper) expectSnapshot() {
	h.m.EXPECT().
		CreateToolhelp32Snapshot(uint32(windows.TH32CS_SNAPPROCESS), uint32(0)).
		Return(testSnapshot, nil)
}

func (h *mockHelper) expectCloseSnapshot() {
	h.m.EXPECT().CloseHandle(testSnapshot).Return(nil)
}

func (h *mockHelper) expectProcess32(pid uint32, name string) {
	h.m.EXPECT().
		Process32First(testSnapshot, gomock.Any()).
		DoAndReturn(func(_ windows.Handle, pe *windows.ProcessEntry32) error {
			*pe = makeProcessEntry(pid, name)
			return nil
		})
}

func (h *mockHelper) expectNoMoreProcesses() {
	h.m.EXPECT().
		Process32Next(testSnapshot, gomock.Any()).
		Return(errNoMoreProcesses).AnyTimes()
}

func (h *mockHelper) expectOpenProcess(pid uint32, handle windows.Handle, err error) {
	h.m.EXPECT().
		OpenProcess(uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION), false, pid).
		Return(handle, err)
}

func (h *mockHelper) expectCloseProcessHandle() {
	h.m.EXPECT().CloseHandle(testProcessHandle).Return(nil)
}

func (h *mockHelper) expectOpenProcessToken(err error) {
	call := h.m.EXPECT().
		OpenProcessToken(testProcessHandle, uint32(windows.TOKEN_QUERY), gomock.Any())
	if err != nil {
		call.Return(err)
	} else {
		call.DoAndReturn(func(_ windows.Handle, _ uint32, token *windows.Token) error {
			*token = testToken
			return nil
		})
	}
}

func (h *mockHelper) expectGetTokenUser(user *windows.Tokenuser, err error) {
	h.m.EXPECT().GetTokenUser(testToken).Return(user, err)
}

func (h *mockHelper) expectLookupAccount(account, domain string, err error) {
	h.m.EXPECT().LookupAccount(mockSID, "").Return(account, domain, uint32(0), err)
}

func (h *mockHelper) expectCloseToken() {
	h.m.EXPECT().CloseToken(testToken).Return(nil)
}

// expectSuccessfulMatch sets up all mocks for a successful vmmem match.
func (h *mockHelper) expectSuccessfulMatch(processName string) {
	h.expectSnapshot()
	h.expectProcess32(testPID, processName)
	h.expectOpenProcess(testPID, testProcessHandle, nil)
	h.expectOpenProcessToken(nil)
	h.expectGetTokenUser(mockTokenUser, nil)
	h.expectLookupAccount(testVMIDStr, "NT VIRTUAL MACHINE", nil)
	h.expectCloseToken()
	h.expectNoMoreProcesses()
	h.expectCloseSnapshot()
}

// expectSkipThenMatch sets up a snapshot with two vmmem processes: the first
// belongs to a different VM and should be skipped, while the second matches
// the given vmIDStr.
func (h *mockHelper) expectSkipThenMatch(vmIDStr string) {
	gomock.InOrder(
		h.m.EXPECT().CreateToolhelp32Snapshot(uint32(windows.TH32CS_SNAPPROCESS), uint32(0)).Return(testSnapshot, nil),
		// First vmmem: wrong VM.
		h.m.EXPECT().Process32First(testSnapshot, gomock.Any()).DoAndReturn(func(_ windows.Handle, pe *windows.ProcessEntry32) error {
			*pe = makeProcessEntry(testPID, "vmmem")
			return nil
		}),
		h.m.EXPECT().OpenProcess(uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION), false, testPID).Return(testProcessHandle, nil),
		h.m.EXPECT().OpenProcessToken(testProcessHandle, uint32(windows.TOKEN_QUERY), gomock.Any()).DoAndReturn(
			func(_ windows.Handle, _ uint32, token *windows.Token) error { *token = testToken; return nil },
		),
		h.m.EXPECT().GetTokenUser(testToken).Return(mockTokenUser, nil),
		h.m.EXPECT().LookupAccount(mockSID, "").Return("OTHER-GUID", "NT VIRTUAL MACHINE", uint32(0), nil),
		h.m.EXPECT().CloseToken(testToken).Return(nil),
		h.m.EXPECT().CloseHandle(testProcessHandle).Return(nil),
		// Second vmmem: correct VM.
		h.m.EXPECT().Process32Next(testSnapshot, gomock.Any()).DoAndReturn(func(_ windows.Handle, pe *windows.ProcessEntry32) error {
			*pe = makeProcessEntry(testPID2, "vmmem.exe")
			return nil
		}),
		h.m.EXPECT().OpenProcess(uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION), false, testPID2).Return(testProcessHandle2, nil),
		h.m.EXPECT().OpenProcessToken(testProcessHandle2, uint32(windows.TOKEN_QUERY), gomock.Any()).DoAndReturn(
			func(_ windows.Handle, _ uint32, token *windows.Token) error { *token = testToken2; return nil },
		),
		h.m.EXPECT().GetTokenUser(testToken2).Return(mockTokenUser, nil),
		h.m.EXPECT().LookupAccount(mockSID, "").Return(vmIDStr, "NT VIRTUAL MACHINE", uint32(0), nil),
		h.m.EXPECT().CloseToken(testToken2).Return(nil),
		h.m.EXPECT().Process32Next(testSnapshot, gomock.Any()).Return(errNoMoreProcesses).AnyTimes(),
		h.m.EXPECT().CloseHandle(testSnapshot).Return(nil),
	)
}

// expectMultipleVmmemNoneMatch sets up a snapshot with two vmmem processes,
// both belonging to different VMs. Verifies that all handles are properly
// closed when no match is found.
func (h *mockHelper) expectMultipleVmmemNoneMatch() {
	gomock.InOrder(
		h.m.EXPECT().CreateToolhelp32Snapshot(uint32(windows.TH32CS_SNAPPROCESS), uint32(0)).Return(testSnapshot, nil),
		// First vmmem: wrong VM.
		h.m.EXPECT().Process32First(testSnapshot, gomock.Any()).DoAndReturn(func(_ windows.Handle, pe *windows.ProcessEntry32) error {
			*pe = makeProcessEntry(testPID, "vmmem")
			return nil
		}),
		h.m.EXPECT().OpenProcess(uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION), false, testPID).Return(testProcessHandle, nil),
		h.m.EXPECT().OpenProcessToken(testProcessHandle, uint32(windows.TOKEN_QUERY), gomock.Any()).DoAndReturn(
			func(_ windows.Handle, _ uint32, token *windows.Token) error { *token = testToken; return nil },
		),
		h.m.EXPECT().GetTokenUser(testToken).Return(mockTokenUser, nil),
		h.m.EXPECT().LookupAccount(mockSID, "").Return("WRONG-GUID-1", "NT VIRTUAL MACHINE", uint32(0), nil),
		h.m.EXPECT().CloseToken(testToken).Return(nil),
		h.m.EXPECT().CloseHandle(testProcessHandle).Return(nil),
		// Second vmmem: also wrong VM.
		h.m.EXPECT().Process32Next(testSnapshot, gomock.Any()).DoAndReturn(func(_ windows.Handle, pe *windows.ProcessEntry32) error {
			*pe = makeProcessEntry(testPID2, "vmmem.exe")
			return nil
		}),
		h.m.EXPECT().OpenProcess(uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION), false, testPID2).Return(testProcessHandle2, nil),
		h.m.EXPECT().OpenProcessToken(testProcessHandle2, uint32(windows.TOKEN_QUERY), gomock.Any()).DoAndReturn(
			func(_ windows.Handle, _ uint32, token *windows.Token) error { *token = testToken2; return nil },
		),
		h.m.EXPECT().GetTokenUser(testToken2).Return(mockTokenUser, nil),
		h.m.EXPECT().LookupAccount(mockSID, "").Return("WRONG-GUID-2", "NT VIRTUAL MACHINE", uint32(0), nil),
		h.m.EXPECT().CloseToken(testToken2).Return(nil),
		h.m.EXPECT().CloseHandle(testProcessHandle2).Return(nil),
		// No more processes.
		h.m.EXPECT().Process32Next(testSnapshot, gomock.Any()).Return(errNoMoreProcesses),
		h.m.EXPECT().CloseHandle(testSnapshot).Return(nil),
	)
}
