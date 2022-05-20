//go:build windows

package exec

import (
	"os"

	"github.com/Microsoft/hcsshim/internal/conpty"
	"github.com/Microsoft/hcsshim/internal/jobobject"
	"golang.org/x/sys/windows"
)

type ExecOpts func(e *execConfig) error

type execConfig struct {
	dir string
	env []string

	cpty                  *conpty.Pty
	stdin, stdout, stderr bool
	// pass in files directly to process, rather than create pipes
	stdinF, stdoutF, stderrF *os.File

	job          *jobobject.JobObject
	token        windows.Token
	processFlags uint32
	attrList     *windows.ProcThreadAttributeListContainer
}

// WithDir will use `dir` as the working directory for the process.
func WithDir(dir string) ExecOpts {
	return func(e *execConfig) error {
		e.dir = dir
		return nil
	}
}

// WithStdio will hook up stdio for the process to a pipe, the other end of which can be retrieved by calling Stdout(), StdErr(), or Stdin()
// respectively on the Exec object. Stdio will be hooked up to the NUL device otherwise.
func WithStdio(stdout, stderr, stdin bool) ExecOpts {
	return func(e *execConfig) error {
		e.stdout = stdout
		e.stderr = stderr
		e.stdin = stdin
		return nil
	}
}

// UsingStdio will pass the file handles to the process stdio directly. The files can be retrieved
// by calling Stdout(), StdErr(), or Stdin(), respectively, on the Exec object.
// Stdio will be hooked up to the NUL device otherwise.
func UsingStdio(stdin, stdout, stderr *os.File) ExecOpts {
	return func(e *execConfig) error {
		e.stdinF = stdin
		e.stdoutF = stdout
		e.stderrF = stderr
		return nil
	}
}

// WithEnv will use the passed in environment variables for the new process.
func WithEnv(env []string) ExecOpts {
	return func(e *execConfig) error {
		e.env = env
		return nil
	}
}

// WithJobObject will launch the newly created process in the passed in job.
func WithJobObject(job *jobobject.JobObject) ExecOpts {
	return func(e *execConfig) error {
		e.job = job
		return nil
	}
}

// WithConPty will launch the created process with a pseudo console attached to the process.
func WithConPty(cpty *conpty.Pty) ExecOpts {
	return func(e *execConfig) error {
		e.cpty = cpty
		return nil
	}
}

// WithToken will run the process as the user that `token` represents.
func WithToken(token windows.Token) ExecOpts {
	return func(e *execConfig) error {
		e.token = token
		return nil
	}
}

// WithProcessFlags will pass `flags` to CreateProcess's creationFlags parameter.
func WithProcessFlags(flags uint32) ExecOpts {
	return func(e *execConfig) error {
		e.processFlags = flags
		return nil
	}
}

// WithProcessAttributes will append `attr` to the attributes passed to CreateProcess.
func WithProcessAttributes(attr *windows.ProcThreadAttributeListContainer) ExecOpts {
	return func(e *execConfig) error {
		e.attrList = attr
		return nil
	}
}
