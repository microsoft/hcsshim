// +build linux

package hcsv2

import (
	"io"
	"os"
)

// copyFile copies `src` to `dest` and sets `perm` on `dest`.
func copyFile(src, dest string, perm os.FileMode) (err error) {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	d, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE, perm)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(dest)
		}
	}()
	_, err = io.Copy(d, s)
	if err != nil {
		return err
	}
	return nil
}
