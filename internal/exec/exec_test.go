package exec

import (
	"encoding/binary"
	"os"
	"testing"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/Microsoft/hcsshim/osversion"
	_ "github.com/Microsoft/hcsshim/test/functional/manifest"
	"golang.org/x/sys/windows"
)

// Tests that the standard os/exec.Cmd functionality works the same.
func TestExecProcess(t *testing.T) {
	cmd := Command("ping", "127.0.0.1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run process: %s", err)
	}
}

// Tests that the new procattrlist functionality works as expected.
func TestExecProcessAttrList(t *testing.T) {
	procAttrList, err := winapi.CreateProcThreadAttrList(1)
	if err != nil {
		t.Fatal(err.Error())
	}

	// This is useless but it tests all the win32 calls working. A better use for this test would be creating
	// this process to be a child of a different process other than the one launching it but we can't gurantee
	// any other processes running/get their pids.
	pHandle, err := windows.OpenProcess(winapi.PROCESS_ALL_ACCESS, false, uint32(os.Getpid()))
	if err != nil {
		t.Fatalf("failed to open process: %s", err)
	}
	uintPtrHandle := uintptr(pHandle)
	if err := winapi.UpdateProcThreadAttribute(
		procAttrList,
		0,
		winapi.PROC_THREAD_ATTRIBUTE_PARENT_PROCESS,
		&uintPtrHandle,
		unsafe.Sizeof(pHandle),
		nil,
		nil,
	); err != nil {
		t.Fatalf("UpdateProcThreadAttribute failed: %s", err)
	}

	cmd := Command("ping", "127.0.0.1")
	cmd.SysProcAttr = &SysProcAttr{
		ProcThreadAttrList: procAttrList,
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run process: %s", err)
	}
}

func TestJobObject(t *testing.T) {
	procAttrList, err := winapi.CreateProcThreadAttrList(1)
	if err != nil {
		t.Fatal(err.Error())
	}

	job, err := windows.CreateJobObject(nil, windows.StringToUTF16Ptr("test"))
	if err != nil {
		t.Fatalf("failed to create job object")
	}
	uintPtrHandle := uintptr(job)
	if err := winapi.UpdateProcThreadAttribute(
		procAttrList,
		0,
		winapi.PROC_THREAD_ATTRIBUTE_JOB_LIST,
		&uintPtrHandle,
		unsafe.Sizeof(job),
		nil,
		nil,
	); err != nil {
		t.Fatalf("UpdateProcThreadAttribute failed: %s", err)
	}

	cmd := Command("ping", "127.0.0.1")
	cmd.SysProcAttr = &SysProcAttr{
		ProcThreadAttrList: procAttrList,
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run process: %s", err)
	}
}

func TestPseudoConsole(t *testing.T) {
	// Skip if not on RS5. This is when the pseudoconsole API was introduced
	if osversion.Get().Build < osversion.RS5 {
		t.Skip()
	}

	cmd := Command("ping", "127.0.0.1")

	var b [4]byte
	binary.LittleEndian.PutUint16(b[0:], 80)
	binary.LittleEndian.PutUint16(b[2:], 40)
	coord := uintptr(binary.LittleEndian.Uint32(b[:]))

	var (
		ptyIn     windows.Handle
		ptyOut    windows.Handle
		pipeFdOut windows.Handle
		pipeFdIn  windows.Handle
	)

	if err := windows.CreatePipe(&ptyIn, &pipeFdIn, nil, 0); err != nil {
		t.Fatalf("failed to create pty input pipes: %s", err)
	}
	defer windows.CloseHandle(pipeFdIn)

	if err := windows.CreatePipe(&pipeFdOut, &ptyOut, nil, 0); err != nil {
		t.Fatalf("failed to create pty output pipes: %s", err)
	}
	defer windows.CloseHandle(pipeFdOut)

	var hpCon windows.Handle
	if err := winapi.CreatePseudoConsole(
		coord,
		ptyIn,
		ptyOut,
		0,
		&hpCon,
	); err != nil {
		t.Fatalf("failed to create pseudo console: %s", err)
	}

	if ptyOut != windows.InvalidHandle {
		windows.CloseHandle(ptyOut)
	}
	if ptyIn != windows.InvalidHandle {
		windows.CloseHandle(ptyIn)
	}

	procAttrList, err := winapi.CreateProcThreadAttrList(1)
	if err != nil {
		t.Fatal(err.Error())
	}

	uintPtrHandle := uintptr(hpCon)
	if err := winapi.UpdateProcThreadAttribute(
		procAttrList,
		0,
		winapi.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		&uintPtrHandle,
		unsafe.Sizeof(hpCon),
		nil,
		nil,
	); err != nil {
		t.Fatalf("UpdateProcThreadAttribute failed: %s", err)
	}

	cmd.SysProcAttr = &SysProcAttr{
		ProcThreadAttrList: procAttrList,
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run process: %s", err)
	}
}

func TestMultipleProcAttributes(t *testing.T) {
	// Skip if not on RS5. This is when the pseudoconsole API was introduced
	if osversion.Get().Build < osversion.RS5 {
		t.Skip()
	}
	procAttrList, err := winapi.CreateProcThreadAttrList(2)
	if err != nil {
		t.Fatal(err.Error())
	}

	var b [4]byte
	binary.LittleEndian.PutUint16(b[0:], 80)
	binary.LittleEndian.PutUint16(b[2:], 40)
	coord := uintptr(binary.LittleEndian.Uint32(b[:]))

	var (
		ptyIn     windows.Handle
		ptyOut    windows.Handle
		pipeFdOut windows.Handle
		pipeFdIn  windows.Handle
	)

	if err := windows.CreatePipe(&ptyIn, &pipeFdIn, nil, 0); err != nil {
		t.Fatalf("failed to create pty input pipes: %s", err)
	}
	defer windows.CloseHandle(pipeFdIn)

	if err := windows.CreatePipe(&pipeFdOut, &ptyOut, nil, 0); err != nil {
		t.Fatalf("failed to create pty output pipes: %s", err)
	}
	defer windows.CloseHandle(pipeFdOut)

	var hpCon windows.Handle
	if err := winapi.CreatePseudoConsole(
		coord,
		ptyIn,
		ptyOut,
		0,
		&hpCon,
	); err != nil {
		t.Fatalf("failed to create pseudo console: %s", err)
	}

	if ptyOut != windows.InvalidHandle {
		windows.CloseHandle(ptyOut)
	}
	if ptyIn != windows.InvalidHandle {
		windows.CloseHandle(ptyIn)
	}

	uintPtrHandle := uintptr(hpCon)
	if err := winapi.UpdateProcThreadAttribute(
		procAttrList,
		0,
		winapi.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		&uintPtrHandle,
		unsafe.Sizeof(hpCon),
		nil,
		nil,
	); err != nil {
		t.Fatalf("UpdateProcThreadAttribute failed: %s", err)
	}

	job, err := windows.CreateJobObject(nil, windows.StringToUTF16Ptr("test"))
	if err != nil {
		t.Fatalf("failed to create job object")
	}

	uintPtrHandle = uintptr(job)
	if err := winapi.UpdateProcThreadAttribute(
		procAttrList,
		0,
		winapi.PROC_THREAD_ATTRIBUTE_JOB_LIST,
		&uintPtrHandle,
		unsafe.Sizeof(job),
		nil,
		nil,
	); err != nil {
		t.Fatalf("UpdateProcThreadAttribute failed: %s", err)
	}

	cmd := Command("ping", "127.0.0.1")
	cmd.SysProcAttr = &SysProcAttr{
		ProcThreadAttrList: procAttrList,
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run process: %s", err)
	}
}
