package jobcontainers

import (
	"unicode/utf16"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// defaultEnvBlock will return a new environment block in the context of the user token
// `token`.
//
// This is almost a direct copy of the go stdlib implementation with some slight changes
// to force a valid token to be passed.
// https://github.com/golang/go/blob/f21be2fdc6f1becdbed1592ea0b245cdeedc5ac8/src/internal/syscall/execenv/execenv_windows.go#L24
func defaultEnvBlock(token windows.Token) (env []string, err error) {
	if token == 0 {
		return nil, errors.New("invalid token for creating environment block")
	}

	var block *uint16
	if err := windows.CreateEnvironmentBlock(&block, token, false); err != nil {
		return nil, err
	}
	defer windows.DestroyEnvironmentBlock(block)

	blockp := uintptr(unsafe.Pointer(block))
	for {
		// find NUL terminator
		end := unsafe.Pointer(blockp)
		for *(*uint16)(end) != 0 {
			end = unsafe.Pointer(uintptr(end) + 2)
		}

		n := (uintptr(end) - uintptr(unsafe.Pointer(blockp))) / 2
		if n == 0 {
			// environment block ends with empty string
			break
		}
		entry := (*[(1 << 30) - 1]uint16)(unsafe.Pointer(blockp))[:n:n]
		env = append(env, string(utf16.Decode(entry)))
		blockp += 2 * (uintptr(len(entry)) + 1)
	}
	return
}
