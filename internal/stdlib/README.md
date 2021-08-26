# Why
Package stdlib is a fork of a small set of packages from the go stdlib and specifically the os, syscall, os/exec, and /internal/syscall/execenv packages.
It exists because the process execution mechanism in the stdlib (os/exec.Cmd and everything in it's call chain all the way down to syscall.StartProcess)
currently don't expose what's necessary to be able to accomplish some things that we need on Windows. This boils down to three things currently.

1. `exec.Cmd.Start()` calls `exec.LookPath` which looks for certain windows extensions to launch an executable at the path provided and will fail if
one is not found. Although rare, there are cases of binaries with no extension that are perfectly valid and able to be launched by `CreateProcessW`.
Granted this can be worked around by setting PATHEXT to anything with an empty space entry after the colon separator, so this is more of an added
annoyance for the other two reasons.
For example:
```go
os.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD;.VBS;.VBE;.JS;.JSE;.WSF;.WSH;.MSC;.CPL; ")
```

2. For job containers we'd like the ability to launch a process in a job directly as it's created, instead of launching it and assigning it to the
job slightly afterwards. This introduces a small window where the process is unaccounted for and not in the "container".
The desired behavior can be accomplished in Windows by calling [UpdateProcThreadAtrribute](https://docs.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-updateprocthreadattribute)
and using the constant `PROC_THREAD_ATTRIBUTE_JOB_LIST`. For the stdlib to support this it would need a new field off of syscall.SysProcAttr to be
able to pass in the job object handle that you'd like the process added to. However, there is no way to create a job object in the stdlib itself
and the syscall package is locked down for the most part.

3. Almost same story as 2, but in this case we'd like to support assigning a [pseudo console](https://docs.microsoft.com/en-us/windows/console/createpseudoconsole)
to a process. There's no exposed way to pass in the pseudo console handle and there's no syscall package support for making one in the first place.

The stdlib packages have been modified to use the x/sys/windows package where needed, removed some unneccesary functionality that we
don't need, as well as added the additions described above.

The fork is based off of go 1.17 with HEAD at a6ff433d6a927e8ad8eaa6828127233296d12ce5.