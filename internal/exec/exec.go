//go:build windows

package exec

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	errProcNotStarted  = errors.New("process has not started yet")
	errProcNotFinished = errors.New("process has not finished yet")
)

// Exec is an object that represents an external process. A user should NOT initialize one manually and instead should
// call New() and pass in the relevant options to retrieve one.
//
// The Exec object is not intended to be used across threads and most methods should only be called once per object.
// It's expected to follow one of two conventions for starting and managing the lifetime of the process.
//
// Either: New() -> e.Start() -> e.Wait() -> (Optional) e.ExitCode()
//
// or: New() -> e.Run() -> (Optional) e.ExitCode()
//
// To capture output or send data to the process, the Stdin(), StdOut() and StdIn() methods can be used.
type Exec struct {
	path    string
	cmdline string
	// Process filled in after Start() returns successfully.
	process *os.Process
	// procState will be filled in after Wait() returns.
	procState  *os.ProcessState
	waitCalled bool
	// stdioPipesOurSide are the stdio pipes that Exec owns and that we will use to send and receive input from the process.
	// These are what will be returned from calls to Exec.Stdin()/Stdout()/Stderr().
	stdioPipesOurSide [3]*os.File
	// stdioPipesProcSide are the stdio pipes that will be passed into the process. These should not be interacted with at all
	// and aren't exposed in any way to a user of Exec.
	stdioPipesProcSide [3]*os.File
	attrList           *windows.ProcThreadAttributeListContainer
	*execConfig
}

// New returns a new instance of an `Exec` object. A process is not running at this point and must be started via either Run(), or a combination
// of Start() + Wait().
func New(path, cmdLine string, opts ...ExecOpts) (*Exec, error) {
	// Path is the only required parameter here, as we need something to launch.
	if path == "" {
		return nil, errors.New("path cannot be empty")
	}

	// Apply all of the options passed in.
	eopts := &execConfig{}
	for _, o := range opts {
		if err := o(eopts); err != nil {
			return nil, err
		}
	}

	e := &Exec{
		path:       path,
		cmdline:    cmdLine,
		execConfig: eopts,
	}

	if err := e.setupStdio(); err != nil {
		return nil, err
	}
	return e, nil
}

// Start starts the process with the path and cmdline specified when the Exec object was created. This does not wait for exit or release any resources,
// a call to Wait must be made afterwards.
func (e *Exec) Start() error {
	argv0 := e.path
	if len(e.dir) != 0 {
		// Windows CreateProcess looks for argv0 relative to the current
		// directory, and, only once the new process is started, it does
		// Chdir(attr.Dir). We are adjusting for that difference here by
		// making argv0 absolute.
		var err error
		argv0, err = joinExeDirAndFName(e.dir, e.path)
		if err != nil {
			return err
		}
	}

	argv0p, err := windows.UTF16PtrFromString(argv0)
	if err != nil {
		return err
	}

	argvp, err := windows.UTF16PtrFromString(e.cmdline)
	if err != nil {
		return err
	}

	var dirp *uint16
	if len(e.dir) != 0 {
		dirp, err = windows.UTF16PtrFromString(e.dir)
		if err != nil {
			return err
		}
	}

	siEx := new(windows.StartupInfoEx)
	siEx.Flags = windows.STARTF_USESTDHANDLES
	pi := new(windows.ProcessInformation)

	// Need EXTENDED_STARTUPINFO_PRESENT as we're making use of the attribute list field.
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT) | windows.EXTENDED_STARTUPINFO_PRESENT | e.execConfig.processFlags

	// Allocate an attribute list that's large enough to do the operations we care about
	// 1. Assigning to a job object at creation time
	// 2. Pseudo console setup if one was requested.
	// 3. Inherit only stdio handles if ones were requested.
	// Therefore we need a list of size 3.
	e.attrList, err = windows.NewProcThreadAttributeList(3)
	if err != nil {
		return fmt.Errorf("failed to initialize process thread attribute list: %w", err)
	}

	// Need to know whether the process needs to inherit stdio handles. The below setup is so that we only inherit the
	// stdio pipes and nothing else into the new process.
	inheritHandles := e.stdioPipesProcSide[0] != nil || e.stdioPipesProcSide[1] != nil || e.stdioPipesProcSide[2] != nil
	if inheritHandles {
		var handles []uintptr
		for _, file := range e.stdioPipesProcSide {
			if file.Fd() != uintptr(syscall.InvalidHandle) {
				handles = append(handles, file.Fd())
			}
		}

		// Set up the process to only inherit stdio handles and nothing else.
		err := e.attrList.Update(
			windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST,
			unsafe.Pointer(&handles[0]),
			uintptr(len(handles))*unsafe.Sizeof(handles[0]),
		)
		if err != nil {
			return err
		}

		// Assign the handles to the startupinfos stdio fields.
		if e.stdioPipesProcSide[0] != nil {
			siEx.StdInput = windows.Handle(e.stdioPipesProcSide[0].Fd())
		}
		if e.stdioPipesProcSide[1] != nil {
			siEx.StdOutput = windows.Handle(e.stdioPipesProcSide[1].Fd())
		}
		if e.stdioPipesProcSide[2] != nil {
			siEx.StdErr = windows.Handle(e.stdioPipesProcSide[2].Fd())
		}
	}

	if e.job != nil {
		if err := e.job.UpdateProcThreadAttribute(e.attrList); err != nil {
			return err
		}
	}

	if e.cpty != nil {
		if err := e.cpty.UpdateProcThreadAttribute(e.attrList); err != nil {
			return err
		}
	}

	var zeroSec windows.SecurityAttributes
	pSec := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(zeroSec)), InheritHandle: 1}
	tSec := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(zeroSec)), InheritHandle: 1}

	siEx.ProcThreadAttributeList = e.attrList.List() //nolint:govet // unusedwrite: ProcThreadAttributeList will be read in syscall
	siEx.Cb = uint32(unsafe.Sizeof(*siEx))
	if e.execConfig.token != 0 {
		err = windows.CreateProcessAsUser(
			e.execConfig.token,
			argv0p,
			argvp,
			pSec,
			tSec,
			inheritHandles,
			flags,
			createEnvBlock(addCriticalEnv(dedupEnvCase(true, e.env))),
			dirp,
			&siEx.StartupInfo,
			pi,
		)
	} else {
		err = windows.CreateProcess(
			argv0p,
			argvp,
			pSec,
			tSec,
			inheritHandles,
			flags,
			createEnvBlock(addCriticalEnv(dedupEnvCase(true, e.env))),
			dirp,
			&siEx.StartupInfo,
			pi,
		)
	}
	if err != nil {
		return fmt.Errorf("failed to create process: %w", err)
	}
	// Don't need the thread handle for anything.
	defer func() {
		_ = windows.CloseHandle(windows.Handle(pi.Thread))
	}()

	// Grab an *os.Process to avoid reinventing the wheel here. The stdlib has great logic around waiting, exit code status/cleanup after a
	// process has been launched.
	e.process, err = os.FindProcess(int(pi.ProcessId))
	if err != nil {
		// If we can't find the process via os.FindProcess, terminate the process as that's what we rely on for all further operations on the
		// object.
		if tErr := windows.TerminateProcess(pi.Process, 1); tErr != nil {
			return fmt.Errorf("failed to terminate process after process not found: %w", tErr)
		}
		return fmt.Errorf("failed to find process after starting: %w", err)
	}
	return nil
}

// Run will run the process to completion. This can be accomplished manually by calling Start + Wait afterwards.
func (e *Exec) Run() error {
	if err := e.Start(); err != nil {
		return err
	}
	return e.Wait()
}

// Close will release resources tied to the process (stdio etc.)
func (e *Exec) close() error {
	if e.procState == nil {
		return errProcNotFinished
	}
	e.attrList.Delete()
	e.closeStdio()
	return nil
}

// Pid returns the pid of the running process. If the process isn't running, this will return -1.
func (e *Exec) Pid() int {
	if e.process == nil {
		return -1
	}
	return e.process.Pid
}

// Exited returns if the process has exited.
func (e *Exec) Exited() bool {
	if e.procState == nil {
		return false
	}
	return e.procState.Exited()
}

// ExitCode returns the exit code of the process. If the process hasn't exited, this will return -1.
func (e *Exec) ExitCode() int {
	if e.procState == nil {
		return -1
	}
	return e.procState.ExitCode()
}

// Wait synchronously waits for the process to complete and will close the stdio pipes afterwards. This should only be called once per Exec
// object.
func (e *Exec) Wait() (err error) {
	if e.process == nil {
		return errProcNotStarted
	}
	if e.waitCalled {
		return errors.New("exec: Wait was already called")
	}
	e.waitCalled = true
	e.procState, err = e.process.Wait()
	if err != nil {
		return err
	}
	return e.close()
}

// Kill will forcefully kill the process.
func (e *Exec) Kill() error {
	if e.process == nil {
		return errProcNotStarted
	}
	return e.process.Kill()
}

// Stdin returns the pipe standard input is hooked up to. This will be closed once Wait returns.
func (e *Exec) Stdin() *os.File {
	if e.cpty != nil {
		return e.cpty.InPipe()
	}
	return e.stdioPipesOurSide[0]
}

// Stdout returns the pipe standard output is hooked up to. It's expected that the client will continuously drain the pipe if standard output is requested.
// The pipe will be closed once Wait returns.
func (e *Exec) Stdout() *os.File {
	if e.cpty != nil {
		return e.cpty.OutPipe()
	}
	return e.stdioPipesOurSide[1]
}

// Stderr returns the pipe standard error is hooked up to. It's expected that the client will continuously drain the pipe if standard output is requested.
// This will be closed once Wait returns.
func (e *Exec) Stderr() *os.File {
	if e.cpty != nil {
		return e.cpty.OutPipe()
	}
	return e.stdioPipesOurSide[2]
}

// setupStdio handles setting up stdio for the process.
func (e *Exec) setupStdio() error {
	stdioRequested := e.stdin || e.stderr || e.stdout
	// If the client requested a pseudo console then there's nothing we need to do pipe wise, as the process inherits the other end of the pty's
	// pipes.
	if e.cpty != nil && stdioRequested {
		return nil
	}

	// Go 1.16's pipe handles (from os.Pipe()) aren't inheritable, so mark them explicitly as such if any stdio handles are
	// requested and someone may be building on 1.16.

	if e.stdin {
		pr, pw, err := os.Pipe()
		if err != nil {
			return err
		}
		e.stdioPipesOurSide[0] = pw

		if err := windows.SetHandleInformation(
			windows.Handle(pr.Fd()),
			windows.HANDLE_FLAG_INHERIT,
			windows.HANDLE_FLAG_INHERIT,
		); err != nil {
			return fmt.Errorf("failed to make stdin pipe inheritable: %w", err)
		}
		e.stdioPipesProcSide[0] = pr
	}

	if e.stdout {
		pr, pw, err := os.Pipe()
		if err != nil {
			return err
		}
		e.stdioPipesOurSide[1] = pr

		if err := windows.SetHandleInformation(
			windows.Handle(pw.Fd()),
			windows.HANDLE_FLAG_INHERIT,
			windows.HANDLE_FLAG_INHERIT,
		); err != nil {
			return fmt.Errorf("failed to make stdout pipe inheritable: %w", err)
		}
		e.stdioPipesProcSide[1] = pw
	}

	if e.stderr {
		pr, pw, err := os.Pipe()
		if err != nil {
			return err
		}
		e.stdioPipesOurSide[2] = pr

		if err := windows.SetHandleInformation(
			windows.Handle(pw.Fd()),
			windows.HANDLE_FLAG_INHERIT,
			windows.HANDLE_FLAG_INHERIT,
		); err != nil {
			return fmt.Errorf("failed to make stderr pipe inheritable: %w", err)
		}
		e.stdioPipesProcSide[2] = pw
	}
	return nil
}

func (e *Exec) closeStdio() {
	for i, file := range e.stdioPipesOurSide {
		if file != nil {
			file.Close()
		}
		e.stdioPipesOurSide[i] = nil
	}
	for i, file := range e.stdioPipesProcSide {
		if file != nil {
			file.Close()
		}
		e.stdioPipesProcSide[i] = nil
	}
}

//
// Below are a bunch of helpers for working with Windows' CreateProcess family of functions. These are mostly exact copies of the same utilities
// found in the go stdlib.
//

func isSlash(c uint8) bool {
	return c == '\\' || c == '/'
}

func normalizeDir(dir string) (name string, err error) {
	ndir, err := syscall.FullPath(dir)
	if err != nil {
		return "", err
	}
	if len(ndir) > 2 && isSlash(ndir[0]) && isSlash(ndir[1]) {
		// dir cannot have \\server\share\path form
		return "", syscall.EINVAL
	}
	return ndir, nil
}

func volToUpper(ch int) int {
	if 'a' <= ch && ch <= 'z' {
		ch += 'A' - 'a'
	}
	return ch
}

func joinExeDirAndFName(dir, p string) (name string, err error) {
	if len(p) == 0 {
		return "", syscall.EINVAL
	}
	if len(p) > 2 && isSlash(p[0]) && isSlash(p[1]) {
		// \\server\share\path form
		return p, nil
	}
	if len(p) > 1 && p[1] == ':' {
		// has drive letter
		if len(p) == 2 {
			return "", syscall.EINVAL
		}
		if isSlash(p[2]) {
			return p, nil
		} else {
			d, err := normalizeDir(dir)
			if err != nil {
				return "", err
			}
			if volToUpper(int(p[0])) == volToUpper(int(d[0])) {
				return syscall.FullPath(d + "\\" + p[2:])
			} else {
				return syscall.FullPath(p)
			}
		}
	} else {
		// no drive letter
		d, err := normalizeDir(dir)
		if err != nil {
			return "", err
		}
		if isSlash(p[0]) {
			return windows.FullPath(d[:2] + p)
		} else {
			return windows.FullPath(d + "\\" + p)
		}
	}
}

// createEnvBlock converts an array of environment strings into
// the representation required by CreateProcess: a sequence of NUL
// terminated strings followed by a nil.
// Last bytes are two UCS-2 NULs, or four NUL bytes.
func createEnvBlock(envv []string) *uint16 {
	if len(envv) == 0 {
		return &utf16.Encode([]rune("\x00\x00"))[0]
	}
	length := 0
	for _, s := range envv {
		length += len(s) + 1
	}
	length += 1

	b := make([]byte, length)
	i := 0
	for _, s := range envv {
		l := len(s)
		copy(b[i:i+l], []byte(s))
		copy(b[i+l:i+l+1], []byte{0})
		i = i + l + 1
	}
	copy(b[i:i+1], []byte{0})

	return &utf16.Encode([]rune(string(b)))[0]
}

// dedupEnvCase is dedupEnv with a case option for testing.
// If caseInsensitive is true, the case of keys is ignored.
func dedupEnvCase(caseInsensitive bool, env []string) []string {
	out := make([]string, 0, len(env))
	saw := make(map[string]int, len(env)) // key => index into out
	for _, kv := range env {
		eq := strings.Index(kv, "=")
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		k := kv[:eq]
		if caseInsensitive {
			k = strings.ToLower(k)
		}
		if dupIdx, isDup := saw[k]; isDup {
			out[dupIdx] = kv
			continue
		}
		saw[k] = len(out)
		out = append(out, kv)
	}
	return out
}

// addCriticalEnv adds any critical environment variables that are required
// (or at least almost always required) on the operating system.
// Currently this is only used for Windows.
func addCriticalEnv(env []string) []string {
	for _, kv := range env {
		eq := strings.Index(kv, "=")
		if eq < 0 {
			continue
		}
		k := kv[:eq]
		if strings.EqualFold(k, "SYSTEMROOT") {
			// We already have it.
			return env
		}
	}
	return append(env, "SYSTEMROOT="+os.Getenv("SYSTEMROOT"))
}
