// Package exec implements a minimalized process execution wrapper.
package exec

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/winapi"
	"golang.org/x/sys/windows"
)

var (
	errProcNotStarted  = errors.New("process has not started yet")
	errProcNotFinished = errors.New("process has not finished yet")
)

type Exec struct {
	path         string
	cmdline      string
	process      *os.Process
	procState    *os.ProcessState
	waitCalled   bool
	stdioOurEnd  [3]*os.File
	stdioProcEnd [3]*os.File
	attrList     *winapi.ProcThreadAttributeList
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

	siEx := new(winapi.StartupInfoEx)
	siEx.Flags = windows.STARTF_USESTDHANDLES
	pi := new(windows.ProcessInformation)

	// Need EXTENDED_STARTUPINFO_PRESENT as we're making use of the attribute list field.
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT) | windows.EXTENDED_STARTUPINFO_PRESENT | e.execConfig.processFlags

	// Allocate an attribute list that's large enough to do the operations we care about
	// 1. Assigning to a job object at creation time
	// 2. Pseudo console setup if one was requested.
	// 3. Inherit only stdio handles if ones were requested.
	e.attrList, err = winapi.NewProcThreadAttributeList(3)
	if err != nil {
		return fmt.Errorf("failed to initialize process thread attribute list: %w", err)
	}
	siEx.ProcThreadAttributeList = e.attrList

	// Need to know whether the process needs to inherit stdio handles.
	inheritHandles := e.stdioProcEnd[0] != nil || e.stdioProcEnd[1] != nil || e.stdioProcEnd[2] != nil
	if inheritHandles {
		var handles []uintptr
		for _, file := range e.stdioProcEnd {
			if file.Fd() != uintptr(syscall.InvalidHandle) {
				handles = append(handles, file.Fd())
			}
		}

		// Set up the process to only inherit stdio handles and nothing else.
		err = winapi.UpdateProcThreadAttribute(
			siEx.ProcThreadAttributeList,
			0,
			windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST,
			unsafe.Pointer(&handles[0]),
			uintptr(len(handles))*unsafe.Sizeof(handles[0]),
			nil,
			nil,
		)
		if err != nil {
			return err
		}

		// Assign the handles to the startupinfos stdio fields.
		if e.stdioProcEnd[0] != nil {
			siEx.StdInput = windows.Handle(e.stdioProcEnd[0].Fd())
		}
		if e.stdioProcEnd[1] != nil {
			siEx.StdOutput = windows.Handle(e.stdioProcEnd[1].Fd())
		}
		if e.stdioProcEnd[2] != nil {
			siEx.StdErr = windows.Handle(e.stdioProcEnd[2].Fd())
		}
	}

	// Update the attribute list for pseudo console and job object setup if requested.

	// if e.conPty != nil {
	// 	if err := e.conPty.UpdateProcThreadAttribute(siEx.ProcThreadAttributeList); err != nil {
	// 		return err
	// 	}
	// }

	if e.job != nil {
		if err := e.job.UpdateProcThreadAttribute(siEx.ProcThreadAttributeList); err != nil {
			return err
		}
	}

	var zeroSec windows.SecurityAttributes
	pSec := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(zeroSec)), InheritHandle: 1}
	tSec := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(zeroSec)), InheritHandle: 1}

	siEx.Cb = uint32(unsafe.Sizeof(*siEx))
	if e.execConfig.token != 0 {
		err = winapi.CreateProcessAsUser(
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
	defer windows.CloseHandle(windows.Handle(pi.Thread)) //nolint:errcheck

	// Grab an *os.Process to avoid reinventing the wheel here. The stdlib has great logic around waiting, exit code status/cleanup after a
	// process has been launched.
	e.process, err = os.FindProcess(int(pi.ProcessId))
	if err != nil {
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
func (e *Exec) Close() error {
	if e.procState == nil {
		return errProcNotFinished
	}
	winapi.DeleteProcThreadAttributeList(e.attrList)
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
func (e *Exec) Wait() error {
	if e.process == nil {
		return errProcNotStarted
	}
	if e.waitCalled {
		return errors.New("exec: Wait was already called")
	}
	e.waitCalled = true
	state, err := e.process.Wait()
	if err != nil {
		return err
	}
	e.procState = state
	e.Close()
	return nil
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
	return e.stdioOurEnd[0]
}

// Stdout returns the pipe standard output is hooked up to. It's expected that the client will continuously drain the pipe if standard output is requested.
// The pipe will be closed once Wait returns.
func (e *Exec) Stdout() *os.File {
	return e.stdioOurEnd[1]
}

// Stderr returns the pipe standard error is hooked up to. It's expected that the client will continuously drain the pipe if standard output is requested.
// This will be closed once Wait returns.
func (e *Exec) Stderr() *os.File {
	return e.stdioOurEnd[2]
}

// setupStdio handles setting up stdio for the process.
func (e *Exec) setupStdio() error {
	// stdioRequested := e.stdin || e.stderr || e.stdout
	// If the client requested a pseudo console then there's nothing we need to do pipe wise, as the process inherits the other end of the pty's
	// pipes.
	// if e.conPty != nil && stdioRequested {
	// 	return errors.New("can't setup both stdio pipes and a pseudo console")
	// }

	if e.stdin {
		pr, pw, err := os.Pipe()
		if err != nil {
			return err
		}
		e.stdioOurEnd[0] = pw
		e.stdioProcEnd[0] = pr
	}

	if e.stdout {
		pr, pw, err := os.Pipe()
		if err != nil {
			return err
		}
		e.stdioOurEnd[1] = pr
		e.stdioProcEnd[1] = pw
	}

	if e.stderr {
		pr, pw, err := os.Pipe()
		if err != nil {
			return err
		}
		e.stdioOurEnd[2] = pr
		e.stdioProcEnd[2] = pw
	}
	return nil
}

func (e *Exec) closeStdio() {
	for i, file := range e.stdioOurEnd {
		if file != nil {
			file.Close()
		}
		e.stdioOurEnd[i] = nil
	}
	for i, file := range e.stdioProcEnd {
		if file != nil {
			file.Close()
		}
		e.stdioProcEnd[i] = nil
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
