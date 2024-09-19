//go:build linux
// +build linux

package devicemapper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"syscall"
	"time"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/log"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/guest/linux"
)

// CreateFlags modify the operation of CreateDevice
type CreateFlags int

const (
	// CreateReadOnly specifies that the device is not writable
	CreateReadOnly CreateFlags = 1 << iota
)

var (
	removeDeviceWrapper = removeDevice
	openMapperWrapper   = openMapper
	_createDevice       = CreateDevice
)

//nolint:stylecheck // ST1003: ALL_CAPS
const (
	_DM_IOCTL      = 0xfd
	_DM_IOCTL_SIZE = 312
	_DM_IOCTL_BASE = linux.IocWRBase | _DM_IOCTL<<linux.IocTypeShift | _DM_IOCTL_SIZE<<linux.IocSizeShift

	_DM_READONLY_FLAG       = 1 << 0
	_DM_SUSPEND_FLAG        = 1 << 1
	_DM_PERSISTENT_DEV_FLAG = 1 << 3
)

const blockSize = 512

//nolint:stylecheck // ST1003: ALL_CAPS
const (
	_DM_VERSION = iota
	_DM_REMOVE_ALL
	_DM_LIST_DEVICES
	_DM_DEV_CREATE
	_DM_DEV_REMOVE
	_DM_DEV_RENAME
	_DM_DEV_SUSPEND
	_DM_DEV_STATUS
	_DM_DEV_WAIT
	_DM_TABLE_LOAD
	_DM_TABLE_CLEAR
	_DM_TABLE_DEPS
	_DM_TABLE_STATUS
)

var dmOpName = []string{
	"version",
	"remove all",
	"list devices",
	"device create",
	"device remove",
	"device rename",
	"device suspend",
	"device status",
	"device wait",
	"table load",
	"table clear",
	"table deps",
	"table status",
}

type dmIoctl struct {
	Version     [3]uint32
	DataSize    uint32
	DataStart   uint32
	TargetCount uint32
	OpenCount   int32
	Flags       uint32
	EventNumber uint32
	_           uint32
	Dev         uint64
	Name        [128]byte
	UUID        [129]byte
	_           [7]byte
}

type targetSpec struct {
	SectorStart    int64
	LengthInBlocks int64
	Status         int32
	Next           uint32
	Type           [16]byte
}

// initIoctl initializes a device-mapper ioctl input struct with the given size
// and device name.
func initIoctl(d *dmIoctl, size int, name string) {
	*d = dmIoctl{
		Version:  [3]uint32{4, 0, 0},
		DataSize: uint32(size),
	}
	copy(d.Name[:], name)
}

type dmError struct {
	Op  int
	Err error
}

func (err *dmError) Error() string {
	op := "<bad operation>"
	if err.Op < len(dmOpName) {
		op = dmOpName[err.Op]
	}
	return "device-mapper " + op + ": " + err.Err.Error()
}

// devMapperIoctl issues the specified device-mapper ioctl.
func devMapperIoctl(f *os.File, code int, data *dmIoctl) error {
	if err := linux.Ioctl(f, code|_DM_IOCTL_BASE, unsafe.Pointer(data)); err != nil {
		return &dmError{Op: code, Err: err}
	}
	return nil
}

// openMapper opens the device-mapper control device and validates that it
// supports the required version.
func openMapper() (f *os.File, err error) {
	f, err = os.OpenFile("/dev/mapper/control", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	var d dmIoctl
	initIoctl(&d, int(unsafe.Sizeof(d)), "")
	err = devMapperIoctl(f, _DM_VERSION, &d)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Target specifies a single entry in a device's target specification.
type Target struct {
	Type           string
	SectorStart    int64
	LengthInBlocks int64
	Params         string
}

// sizeof returns the size of a targetSpec needed to fit this specification.
func (t *Target) sizeof() int {
	// include a null terminator (not sure if necessary) and round up to 8-byte
	// alignment
	return (int(unsafe.Sizeof(targetSpec{})) + len(t.Params) + 1 + 7) &^ 7
}

// LinearTarget constructs a device-mapper target that maps a portion of a block
// device at the specified offset.
//
//	Example linear target table:
//	0 20971520 linear /dev/hda 384
//	|     |      |        |     |
//	start |   target   data_dev |
//	     size                 offset
func LinearTarget(sectorStart, lengthBlocks int64, path string, deviceStart int64) Target {
	return Target{
		Type:           "linear",
		SectorStart:    sectorStart,
		LengthInBlocks: lengthBlocks,
		Params:         fmt.Sprintf("%s %d", path, deviceStart),
	}
}

// zeroSectorLinearTarget creates a Target for devices with 0 sector start and length/device start
// expected to be in bytes rather than blocks.
func zeroSectorLinearTarget(lengthBytes int64, path string, deviceStartBytes int64) Target {
	lengthInBlocks := lengthBytes / blockSize
	startInBlocks := deviceStartBytes / blockSize
	return LinearTarget(0, lengthInBlocks, path, startInBlocks)
}

// makeTableIoctl builds an ioctl input structure with a table of the specified
// targets.
func makeTableIoctl(name string, targets []Target) *dmIoctl {
	off := int(unsafe.Sizeof(dmIoctl{}))
	n := off
	for _, t := range targets {
		n += t.sizeof()
	}
	b := make([]byte, n)
	d := (*dmIoctl)(unsafe.Pointer(&b[0]))
	initIoctl(d, n, name)
	d.DataStart = uint32(off)
	d.TargetCount = uint32(len(targets))
	for _, t := range targets {
		spec := (*targetSpec)(unsafe.Pointer(&b[off]))
		sn := t.sizeof()
		spec.SectorStart = t.SectorStart
		spec.LengthInBlocks = t.LengthInBlocks
		spec.Next = uint32(sn)
		copy(spec.Type[:], t.Type)
		copy(b[off+int(unsafe.Sizeof(*spec)):], t.Params)
		off += sn
	}
	return d
}

// CreateDeviceWithRetryErrors keeps retrying to create device mapper target
func CreateDeviceWithRetryErrors(
	ctx context.Context,
	name string,
	flags CreateFlags,
	targets []Target,
	errs ...error,
) (string, error) {
retry:
	for {
		dmPath, err := _createDevice(name, flags, targets)
		if err == nil {
			return dmPath, nil
		}
		log.G(ctx).WithError(err).Warning("CreateDevice error")
		// In some cases
		dmErr, ok := err.(*dmError) //nolint:errorlint // explicitly returned
		if !ok {
			return "", err
		}
		// check retry-able errors
		for _, e := range errs {
			if errors.Is(dmErr.Err, e) {
				select {
				case <-ctx.Done():
					log.G(ctx).WithError(err).Error("CreateDeviceWithRetryErrors failed, context timeout")
					return "", err
				default:
					time.Sleep(100 * time.Millisecond)
					continue retry
				}
			}
		}
		return "", fmt.Errorf("CreateDeviceWithRetryErrors failed: %w", err)
	}
}

// CreateDevice creates a device-mapper device with the given target spec. It returns
// the path of the new device node.
func CreateDevice(name string, flags CreateFlags, targets []Target) (_ string, err error) {
	f, err := openMapperWrapper()
	if err != nil {
		return "", err
	}
	defer f.Close()

	var d dmIoctl
	size := int(unsafe.Sizeof(d))
	initIoctl(&d, size, name)
	err = devMapperIoctl(f, _DM_DEV_CREATE, &d)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = removeDeviceWrapper(f, name)
		}
	}()

	dev := int(d.Dev)

	di := makeTableIoctl(name, targets)
	if flags&CreateReadOnly != 0 {
		di.Flags |= _DM_READONLY_FLAG
	}
	err = devMapperIoctl(f, _DM_TABLE_LOAD, di)
	if err != nil {
		return "", err
	}
	initIoctl(&d, size, name)
	err = devMapperIoctl(f, _DM_DEV_SUSPEND, &d)
	if err != nil {
		return "", err
	}

	p := path.Join("/dev/mapper", name)
	os.Remove(p)
	err = unix.Mknod(p, unix.S_IFBLK|0600, dev)
	if err != nil {
		return "", nil
	}

	return p, nil
}

// RemoveDevice removes a device-mapper device and its associated device node.
func RemoveDevice(name string) (err error) {
	rm := func() error {
		f, err := openMapperWrapper()
		if err != nil {
			return err
		}
		defer f.Close()
		os.Remove(path.Join("/dev/mapper", name))
		return removeDeviceWrapper(f, name)
	}

	// This is workaround for "device or resource busy" error, which occasionally happens after the device mapper
	// target has been unmounted.
	for i := 0; i < 10; i++ {
		if err = rm(); err != nil {
			if e, ok := err.(*dmError); !ok || !errors.Is(e.Err, syscall.EBUSY) { //nolint:errorlint // explicitly returned
				break
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		break
	}
	return err
}

func removeDevice(f *os.File, name string) error {
	var d dmIoctl
	initIoctl(&d, int(unsafe.Sizeof(d)), name)
	err := devMapperIoctl(f, _DM_DEV_REMOVE, &d)
	if err != nil {
		return err
	}
	return nil
}
