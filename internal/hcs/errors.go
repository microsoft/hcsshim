//go:build windows

package hcs

import (
	"context"
	"errors"

	hcserrors "github.com/Microsoft/hcsshim/internal/hcs/errors"
	"github.com/Microsoft/hcsshim/internal/log"
)

func processHcsResult(ctx context.Context, result string) (events []hcserrors.ErrorEvent) {
	var err error
	events, err = hcserrors.ErrorEventsFromHcsResult(result)
	if err != nil {
		log.G(ctx).WithError(err).Warning("Could not unmarshal HCS result")
		return nil
	}
	return events
}

func makeSystemError(system *System, op string, err error, events []hcserrors.ErrorEvent) error {
	// Don't double wrap errors
	if e := (&hcserrors.SystemError{}); errors.As(err, &e) {
		return err
	}

	return &hcserrors.SystemError{
		ID: system.ID(),
		HcsError: hcserrors.HcsError{
			Op:     op,
			Err:    err,
			Events: events,
		},
	}
}

func makeProcessError(process *Process, op string, err error, events []hcserrors.ErrorEvent) error {
	// Don't double wrap errors
	if e := (&hcserrors.ProcessError{}); errors.As(err, &e) {
		return err
	}
	return &hcserrors.ProcessError{
		Pid:      process.Pid(),
		SystemID: process.SystemID(),
		HcsError: hcserrors.HcsError{
			Op:     op,
			Err:    err,
			Events: events,
		},
	}
}
