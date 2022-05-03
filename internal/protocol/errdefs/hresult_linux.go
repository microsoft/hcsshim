//go:build linux

package errdefs

func (r HResult) AsError() error {
	if r.IsError() {
		return r.error()
	}
	return nil
}
