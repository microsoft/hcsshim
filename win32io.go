package hcsshim

import (
	"io"
	"runtime"
	"syscall"
)

//sys cancelThreadpoolIo(io uintptr) = CancelThreadpoolIo
//sys closeThreadpoolIo(io uintptr) = CloseThreadpoolIo
//sys createThreadpoolIo(file syscall.Handle, callback uintptr, context uintptr, environ uintptr) (io uintptr, err error) = CreateThreadpoolIo
//sys startThreadpoolIo(io uintptr) = StartThreadpoolIo
//sys setFileCompletionNotificationModes(h syscall.Handle, flags uint8) (err error) = SetFileCompletionNotificationModes

const (
	fileSkipCompletionOnSuccess = 1
	fileSkipSetEventOnHandle    = 2
)

// ioResult contains the result of an asynchronous IO operation
type ioResult struct {
	bytes uintptr
	err   error
}

// ioOperation represents an outstanding asynchronous Win32 IO
type ioOperation struct {
	o  syscall.Overlapped
	ch chan ioResult
}

var ioCallbackPtr = syscall.NewCallback(ioCallback)

// ioCallback is called by the Windows thread pool when an asynchronous IO completes
func ioCallback(_, _ uintptr, c *ioOperation, result uint32, bytes uintptr, _ uintptr) int {
	var err error
	if result != 0 {
		err = syscall.Errno(result)
	}
	c.ch <- ioResult{bytes, err}
	return 0
}

// prepareIo prepares for a new IO operation on a threadpool IO object
func prepareIo(io uintptr) *ioOperation {
	c := &ioOperation{}
	c.ch = make(chan ioResult)
	startThreadpoolIo(io)
	return c
}

// abortIo is called when an IO operation fails synchronous, undoing what is done in prepareIo
func abortIo(io uintptr) {
	cancelThreadpoolIo(io)
}

// win32File implements Reader, Writer, and Closer on a Win32 handle without blocking in a syscall.
// It takes ownership of this handle and will close it if it is garbage collected.
type win32File struct {
	handle syscall.Handle
	io     uintptr
}

// makeWin32File makes a new win32File from an existing file handle
func makeWin32File(h syscall.Handle) (*win32File, error) {
	io, err := createThreadpoolIo(h, ioCallbackPtr, 0, 0)
	if err != nil {
		return nil, err
	}
	err = setFileCompletionNotificationModes(h, fileSkipCompletionOnSuccess|fileSkipSetEventOnHandle)
	if err != nil {
		return nil, err
	}
	f := &win32File{h, io}
	runtime.SetFinalizer(f, (*win32File).closeHandle)
	return f, nil
}

// closeHandle closes the resources associated with a Win32 handle
func (f *win32File) closeHandle() {
	if f.handle != 0 {
		closeThreadpoolIo(f.io)
		syscall.CloseHandle(f.handle)
		f.handle = 0
		f.io = 0
	}
}

// Close closes a win32File.
func (f *win32File) Close() error {
	f.closeHandle()
	runtime.SetFinalizer(f, nil)
	return nil
}

// asyncIo processes the return value from ReadFile or WriteFile, blocking until
// the operation has actually completed.
func (f *win32File) asyncIo(c *ioOperation, bytes uint32, err error) (int, error) {
	if err == nil {
		abortIo(f.io)
		return int(bytes), nil
	} else if err == syscall.ERROR_IO_PENDING {
		r := <-c.ch
		return int(r.bytes), r.err
	} else {
		abortIo(f.io)
		return 0, err
	}
}

// Read reads from a file handle.
func (f *win32File) Read(b []byte) (int, error) {
	c := prepareIo(f.io)
	var bytes uint32
	err := syscall.ReadFile(f.handle, b, &bytes, &c.o)
	n, err := f.asyncIo(c, bytes, err)

	// Handle EOF conditions.
	if err == nil && n == 0 && len(b) != 0 {
		return 0, io.EOF
	} else if err == syscall.ERROR_BROKEN_PIPE {
		return 0, io.EOF
	} else {
		return n, err
	}
}

// Write writes to a file handle.
func (f *win32File) Write(b []byte) (int, error) {
	c := prepareIo(f.io)
	var bytes uint32
	err := syscall.WriteFile(f.handle, b, &bytes, &c.o)
	return f.asyncIo(c, bytes, err)
}
