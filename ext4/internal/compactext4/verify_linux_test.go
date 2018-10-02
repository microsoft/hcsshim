package compactext4

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/Microsoft/hcsshim/ext4/internal/format"
)

func expectedMode(f *File) uint16 {
	switch f.Mode & format.TypeMask {
	case 0:
		return f.Mode | S_IFREG
	case S_IFLNK:
		return f.Mode | 0777
	default:
		return f.Mode
	}
}

func expectedSize(f *File) int64 {
	switch f.Mode & format.TypeMask {
	case 0, S_IFREG:
		return f.Size
	case S_IFLNK:
		return int64(len(f.Linkname))
	default:
		return 0
	}
}

func expectedDevice(f *File) uint64 {
	return uint64(f.Devminor&0xff | f.Devmajor<<8 | (f.Devminor&0xffffff00)<<12)
}

func timeEqual(ts syscall.Timespec, t time.Time) bool {
	sec, nsec := t.Unix(), t.Nanosecond()
	if t.IsZero() {
		sec, nsec = 0, 0
	}
	return ts.Sec == sec && int(ts.Nsec) == nsec
}

func verifyTestFile(t *testing.T, mountPath string, tf testFile) {
	name := path.Join(mountPath, tf.Path)
	fi, err := os.Lstat(name)
	if err != nil {
		t.Error(err)
		return
	}
	st := fi.Sys().(*syscall.Stat_t)
	if tf.File != nil {
		if st.Mode != uint32(expectedMode(tf.File)) ||
			st.Uid != tf.File.Uid ||
			st.Gid != tf.File.Gid ||
			(!fi.IsDir() && st.Size != expectedSize(tf.File)) ||
			st.Rdev != expectedDevice(tf.File) ||
			!timeEqual(st.Atim, tf.File.Atime) ||
			!timeEqual(st.Mtim, tf.File.Mtime) ||
			!timeEqual(st.Ctim, tf.File.Ctime) {

			t.Errorf("%s: stat mismatch, expected: %#v got: %#v", tf.Path, tf.File, st)
		}

		switch tf.File.Mode & format.TypeMask {
		case S_IFREG:
			if f, err := os.Open(name); err != nil {
				t.Error(err)
			} else {
				b, err := ioutil.ReadAll(f)
				if err != nil {
					t.Error(err)
				} else if !bytes.Equal(b, tf.Data) {
					t.Errorf("%s: data mismatch", tf.Path)
				}
				f.Close()
			}
		case S_IFLNK:
			if link, err := os.Readlink(name); err != nil {
				t.Error(err)
			} else if link != tf.File.Linkname {
				t.Errorf("%s: link mismatch, expected: %s got: %s", tf.Path, tf.File.Linkname, link)
			}
		}
	} else {
		lfi, err := os.Lstat(path.Join(mountPath, tf.Link))
		if err != nil {
			t.Error(err)
			return
		}

		lst := lfi.Sys().(*syscall.Stat_t)
		if lst.Ino != st.Ino {
			t.Errorf("%s: hard link mismatch with %s, expected inode: %d got inode: %d", tf.Path, tf.Link, lst.Ino, st.Ino)
		}
	}
}

type capHeader struct {
	version uint32
	pid     int
}

type capData struct {
	effective   uint32
	permitted   uint32
	inheritable uint32
}

const CAP_SYS_ADMIN = 21

type caps struct {
	hdr  capHeader
	data [2]capData
}

func getCaps() (caps, error) {
	var c caps

	// Get capability version
	if _, _, errno := syscall.Syscall(syscall.SYS_CAPGET, uintptr(unsafe.Pointer(&c.hdr)), uintptr(unsafe.Pointer(nil)), 0); errno != 0 {
		return c, fmt.Errorf("SYS_CAPGET: %v", errno)
	}

	// Get current capabilities
	if _, _, errno := syscall.Syscall(syscall.SYS_CAPGET, uintptr(unsafe.Pointer(&c.hdr)), uintptr(unsafe.Pointer(&c.data[0])), 0); errno != 0 {
		return c, fmt.Errorf("SYS_CAPGET: %v", errno)
	}

	return c, nil
}

func mountImage(t *testing.T, image string, mountPath string) bool {
	caps, err := getCaps()
	if err != nil || caps.data[0].effective&(1<<uint(CAP_SYS_ADMIN)) == 0 {
		t.Log("cannot mount to run verification tests without CAP_SYS_ADMIN")
		return false
	}

	err = os.MkdirAll(mountPath, 0777)
	if err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("mount", "-o", "loop,ro", "-t", "ext4", image, mountPath).CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatal(err)
	}
	return true
}

func unmountImage(t *testing.T, mountPath string) {
	out, err := exec.Command("umount", mountPath).CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Log(err)
	}
}

func fsck(t *testing.T, image string) {
	cmd := exec.Command("e2fsck", "-v", "-f", "-n", image)
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatal(err)
	}
}
