//go:build windows

package hcsschema

import (
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

var _ error = (*ResultError)(nil)

func (e *ResultError) Error() string {
	// [strings.Builder.WriteString] always returns nil
	b := strings.Builder{}
	_, _ = b.WriteString(fmt.Sprintf("0x%x: %s", uint32(e.Error_), e.ErrorMessage))

	if len(e.ErrorEvents) > 0 {
		_, _ = b.WriteString("\nError Events:")
		for _, ev := range e.ErrorEvents {
			_, _ = b.WriteString("\n\t")
			_, _ = b.WriteString(ev.String())
		}
	}

	if len(e.Attribution) > 0 {
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "")
		// TODO: add pretty printing for attribution records
		// enc.Encode adds a trailing new line, so new line logic is a bit different
		_, _ = b.WriteString("\nAttribution Record:\n")
		for _, ar := range e.Attribution {
			_, _ = b.WriteString("\t")

			if err := enc.Encode(ar); err != nil {
				_, _ = b.WriteString("<invalid attribution record>\n")
			}
		}
	}

	return strings.TrimSpace(b.String())
}

func (e *ResultError) Unwrap() error {
	return e.HResult()
}

func (e *ResultError) HResult() windows.Errno {
	// HRESULT is a LONG (int32).
	// If we convert directly to an [windows.Errno] (uintptr), then the value is sign-extended, which can
	// be confusing, or cause issues when comparing to other Errno's.
	// Convert to a uint32 first, then to [windows.Errno]
	//
	// See: https://learn.microsoft.com/en-us/windows/win32/com/structure-of-com-error-codes

	return windows.Errno(uint32(e.Error_))
}
