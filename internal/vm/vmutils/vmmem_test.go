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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
