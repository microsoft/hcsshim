//go:build linux
// +build linux

package devicemapper

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

var (
	integration = flag.Bool("integration", false, "run integration tests")
)

func clearTestDependencies() {
	_createDevice = CreateDevice
	removeDeviceWrapper = removeDevice
	openMapperWrapper = openMapper
}

func TestMain(m *testing.M) {
	flag.Parse()
	m.Run()
}

func validateDevice(t *testing.T, p string, sectors int64, writable bool) {
	t.Helper()
	dev, err := os.OpenFile(p, os.O_RDWR|os.O_SYNC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()

	var size int64
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, dev.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		t.Fatal(errno)
	}
	if size != sectors*512 {
		t.Fatalf("expected %d bytes, got %d", sectors*512, size)
	}

	var b [512]byte
	_, err = unix.Read(int(dev.Fd()), b[:])
	if !errors.Is(err, unix.EIO) {
		t.Fatalf("expected EIO, got %s", err)
	}
	_, err = unix.Write(int(dev.Fd()), b[:])
	if writable && !errors.Is(err, unix.EIO) {
		t.Fatalf("expected EIO, got %s", err)
	} else if !errors.Is(err, unix.EPERM) {
		t.Fatalf("expected EPERM, got %s", err)
	}
}

type device struct {
	Name, Path string
}

func (d *device) Close() (err error) {
	if d.Name != "" {
		err = RemoveDevice(d.Name)
		if err == nil {
			d.Name = ""
		}
	}
	return err
}

func createDevice(name string, flags CreateFlags, targets []Target) (*device, error) {
	p, err := _createDevice(name, flags, targets)
	if err != nil {
		return nil, err
	}
	return &device{Name: name, Path: p}, nil
}

func TestCreateError(t *testing.T) {
	clearTestDependencies()

	if !*integration {
		t.Skip()
	}
	d, err := createDevice("test-device", 0, []Target{
		{Type: "error", SectorStart: 0, LengthInBlocks: 1},
		{Type: "error", SectorStart: 1, LengthInBlocks: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	validateDevice(t, d.Path, 3, true)
	err = d.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadOnlyError(t *testing.T) {
	clearTestDependencies()

	if !*integration {
		t.Skip()
	}
	d, err := createDevice("test-device", CreateReadOnly, []Target{
		{Type: "error", SectorStart: 0, LengthInBlocks: 1},
		{Type: "error", SectorStart: 1, LengthInBlocks: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	validateDevice(t, d.Path, 3, false)
	err = d.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestLinearError(t *testing.T) {
	clearTestDependencies()

	if !*integration {
		t.Skip()
	}
	b, err := createDevice("base-device", 0, []Target{
		{Type: "error", SectorStart: 0, LengthInBlocks: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	d, err := createDevice("linear-device", 0, []Target{
		LinearTarget(0, 50, b.Path, 50),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	validateDevice(t, d.Path, 50, true)
	err = d.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = b.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestRemoveDeviceRetriesOnSyscallEBUSY(t *testing.T) {
	clearTestDependencies()

	rmDeviceCalled := false
	retryDone := false
	// Overrides openMapper to return temp file handle
	openMapperWrapper = func() (*os.File, error) {
		return os.CreateTemp("", "")
	}
	removeDeviceWrapper = func(_ *os.File, _ string) error {
		if !rmDeviceCalled {
			rmDeviceCalled = true
			return &dmError{
				Op:  1,
				Err: syscall.EBUSY,
			}
		}
		if !retryDone {
			retryDone = true
			return nil
		}
		return nil
	}

	if err := RemoveDevice("test"); err != nil {
		t.Fatalf("expected no error, got: %s", err)
	}
	if !rmDeviceCalled {
		t.Fatalf("expected removeDevice to be called at least once")
	}
	if !retryDone {
		t.Fatalf("expected removeDevice to be retried after initial failure")
	}
}

func TestCreateDeviceWithRetryError(t *testing.T) {
	for _, rerr := range retryErrors {
		t.Run(fmt.Sprintf("error-%s", rerr), func(t *testing.T) {
			clearTestDependencies()
			createDeviceCalled := false
			retryDone := false
			createDeviceStub := func(name string, flags CreateFlags, targets []Target) (string, error) {
				if !createDeviceCalled {
					createDeviceCalled = true
					return "", &dmError{
						Op:  0,
						Err: rerr,
					}
				}
				retryDone = true
				return fmt.Sprintf("/dev/mapper/%s", name), nil
			}
			_createDevice = createDeviceStub

			ctx := context.Background()
			_, err := CreateDeviceWithRetryErrors(ctx, "test-device", 0, []Target{}, retryErrors...)
			if err != nil {
				t.Fatalf("unexpected error: %s\n", err)
			}
			if !createDeviceCalled {
				t.Fatal("CreateDevice was not called")
			}
			if !retryDone {
				t.Fatalf("error %s was not retried\n", rerr)
			}
		})
	}
}

func TestRemoveDeviceFailsOnNonSyscallEBUSY(t *testing.T) {
	clearTestDependencies()

	expectedError := &dmError{
		Op:  0,
		Err: syscall.EACCES,
	}
	rmDeviceCalled := false
	retryDone := false
	openMapperWrapper = func() (*os.File, error) {
		return os.CreateTemp("", "")
	}
	removeDeviceWrapper = func(_ *os.File, _ string) error {
		if !rmDeviceCalled {
			rmDeviceCalled = true
			return expectedError
		}
		if !retryDone {
			retryDone = true
			return nil
		}
		return nil
	}

	if err := RemoveDevice("test"); err != expectedError { //nolint:errorlint
		t.Fatalf("expected error %q, instead got %q", expectedError, err)
	}
	if !rmDeviceCalled {
		t.Fatalf("expected removeDevice to be called once")
	}
	if retryDone {
		t.Fatalf("no retries should've been attempted")
	}
}
