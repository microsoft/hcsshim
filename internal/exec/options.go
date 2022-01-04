package exec

import (
	"github.com/Microsoft/hcsshim/internal/conpty"
	"github.com/Microsoft/hcsshim/internal/jobobject"
	"golang.org/x/sys/windows"
)

type ExecOpts func(e *execConfig) error

type execConfig struct {
	dir                   string
	env                   []string
	stdout, stderr, stdin bool

	job          *jobobject.JobObject
	cpty         *conpty.Pty
	token        windows.Token
	processFlags uint32
}

// WithDir will use `dir` as the working directory for the process.
func WithDir(dir string) ExecOpts {
	return func(e *execConfig) error {
		e.dir = dir
		return nil
	}
}

// WithStdio will hook up stdio for the process to a pipe, the other end of which can be retrieved by calling Stdout(), stdErr(), or Stdin()
// respectively on the Exec object. Stdio will be hooked up to the NUL device otherwise.
func WithStdio(stdout, stderr, stdin bool) ExecOpts {
	return func(e *execConfig) error {
		e.stdout = stdout
		e.stderr = stderr
		e.stdin = stdin
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
