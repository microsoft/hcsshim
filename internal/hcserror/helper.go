package hcserror

import "errors"

// IsAny returns true if errors.Is is true for any of the provided errors, errs.
func IsAny(err error, errs ...error) bool {
	for _, e := range errs {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
