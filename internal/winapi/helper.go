package winapi

import (
	"errors"

	"golang.org/x/sys/windows"
)

// helper functions for calling WIN32 APIS

// retryBuffer creates a []byte buffer, b, with size lo and passes it to f, with *l = len(b).
// If f returns windows.ERROR_INSUFFICIENT_BUFFER or windows.ERROR_BUFFER_OVERFLOW, it creates
// a buffer sized to the value set in the second parameter, l.
func retryBuffer(lo int, f func(b *byte, l *uint32) error) (b []byte, err error) {
	l := uint32(maxInt(1, lo))
	for i := 0; i < 2; i++ {
		b = make([]byte, l)
		err = f(&b[0], &l)
		if bufferTooSmall(l, len(b), err) {
			continue
		}
		break
	}

	if err != nil {
		return b[:0], err
	}
	return b[:l], nil
}

// retryLStr is similar to retryBuffer, but with a uint16 buffer
func retryLStr(lo int, f func(s *uint16, l *uint32) error) (b []uint16, err error) {
	// todo: make this generic
	l := uint32(maxInt(1, lo))
	for i := 0; i < 2; i++ {
		b = make([]uint16, l)
		err = f(&b[0], &l)
		if bufferTooSmall(l, len(b), err) {
			continue
		}
		break
	}

	if err != nil {
		return b[:0], err
	}
	return b[:l], nil
}

func bufferTooSmall(len uint32, buffLen int, err error) bool { //nolint:predeclared
	return int(len) > buffLen &&
		(errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) ||
			errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) ||
			errors.Is(err, windows.ERROR_INVALID_PARAMETER) ||
			errors.Is(err, windows.ERROR_INVALID_USER_BUFFER))
}

func maxInt(a, b int) int {
	if a < b {
		return b
	}
	return a
}
