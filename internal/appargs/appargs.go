// Package appargs provides argument validation routines for use with
// github.com/urfave/cli.
package appargs

import (
	"errors"

	"github.com/urfave/cli"
)

// Validator is an argument validator function. It returns the number of
// arguments consumed or -1 on error.
type Validator = func([]string) int

// Required is a validator for a single required parameter.
func Required(args []string) int {
	if len(args) == 0 {
		return -1
	}
	return 1
}

// Required is a validator for a single required parameter that must not be
// empty.
func RequiredNonEmpty(args []string) int {
	if len(args) == 0 || args[0] == "" {
		return -1
	}
	return 1
}

// Optional is a validator for an optional parameter.
func Optional(args []string) int {
	if len(args) == 0 {
		return 0
	}
	return 1
}

// Rest is a validator that consumes the rest of the arguments without validation.
func Rest(args []string) int {
	return len(args)
}

// ErrInvalidUsage is returned when there is a validation error.
var ErrInvalidUsage = errors.New("invalid command usage")

// Validate can be used as a command's Before function to validate the arguments
// to the command.
func Validate(vs ...Validator) cli.BeforeFunc {
	return func(context *cli.Context) error {
		remaining := context.Args()
		for _, v := range vs {
			consumed := v(remaining)
			if consumed < 0 {
				return ErrInvalidUsage
			}
			remaining = remaining[consumed:]
		}

		if len(remaining) > 0 {
			return ErrInvalidUsage
		}

		return nil
	}
}
