package cgrouputils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Memory cgroup root path
const (
	MemRoot string = "/sys/fs/cgroup/memory"
)

// RegisterMemoryThreshold registers an eventfd(2) and a memory threshold in
// 'cgroup.event_control' of a cgroup. The eventfd will trigger when
// memory.usage_in_bytes exceeds the threshold set.
// https://www.kernel.org/doc/Documentation/cgroup-v1/memory.txt Section 9
func RegisterMemoryThreshold(cgPath string, threshold int64) (_ *os.File, err error) {
	efd, err := unix.Eventfd(0, unix.EFD_CLOEXEC)
	if err != nil {
		return nil, err
	}

	efdFile := os.NewFile(uintptr(efd), "efd")
	defer func() {
		if err != nil {
			efdFile.Close()
		}
	}()

	f, err := os.Open(filepath.Join(cgPath, "memory.usage_in_bytes"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data := fmt.Sprintf("%d %d %d", efdFile.Fd(), f.Fd(), threshold)
	ecPath := filepath.Join(cgPath, "cgroup.event_control")
	if err := ioutil.WriteFile(ecPath, []byte(data), 0700); err != nil {
		return nil, err
	}
	return efdFile, err
}
