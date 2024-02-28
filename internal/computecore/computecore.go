//go:build windows

package computecore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/Microsoft/hcsshim/internal/interop"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/timeout"

	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"golang.org/x/sys/windows"
)

//go:generate go run github.com/Microsoft/go-winio/tools/mkwinsyscall -output zsyscall_windows.go ./*.go

//sys hcsCreateComputeSystem(id string, configuration string, operation HCSOperation, security_descriptor *uint32, computeSystem *HCSSystem) (hr error) = computecore.HcsCreateComputeSystem?

type (
	HCSSystem syscall.Handle
)

func HcsCreateComputeSystem(ctx context.Context, id string, configuration string, operation HCSOperation, securityDescriptor *uint32) (computeSystem HCSSystem, result string, hr error) {
	log.G(ctx).WithField("id", id).Debug("computecore.HcsCreateComputeSystem")
	ctx, span := oc.StartSpan(ctx, "computecore::HcsCreateComputeSystem")
	defer func() {
		if result != "" {
			span.AddAttributes(trace.StringAttribute("resultDocument", result))
		}
		if !errors.Is(hr, windows.ERROR_VMCOMPUTE_OPERATION_PENDING) {
			oc.SetSpanStatus(span, hr)
		}
		span.End()
	}()
	span.AddAttributes(
		trace.StringAttribute("id", id),
		trace.StringAttribute("configuration", configuration),
	)

	log.G(ctx).WithField("id", id).Debug("computecore.execute")
	return computeSystem, result, execute(ctx, timeout.SystemCreate, func() error {
		var resultp *uint16
		err := hcsCreateComputeSystem(id, configuration, operation, securityDescriptor, &computeSystem)
		if resultp != nil {
			result = interop.ConvertAndFreeCoTaskMemString(resultp)
		}

		log.G(ctx).Debug("waiting for operation result")
		// FIXME: here we synchronously wait for operation result, but in the future we should probably switch
		//   to notification model.
		if opResult, opErr := operation.WaitForResult(ctx); opErr != nil {
			log.G(ctx).WithError(opErr).Error("Failed to wait for operation result")
			return opErr
		} else {
			log.G(ctx).WithField("result", opResult).Debug("operation completed")
		}
		return err
	})
}

func bufferToString(buffer *uint16) string {
	if buffer == nil {
		return ""
	}
	return interop.ConvertAndFreeCoTaskMemString(buffer)
}

func encode(v any) (string, error) {
	// TODO: pool of encoders/buffers
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "")

	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("json encoding: %w", err)
	}

	// encoder.Encode appends a newline to the end
	return strings.TrimSpace(buf.String()), nil
}

func execute(ctx context.Context, timeout time.Duration, f func() error) error {
	now := time.Now()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	deadline, ok := ctx.Deadline()
	trueTimeout := timeout
	if ok {
		trueTimeout = deadline.Sub(now)
		log.G(ctx).WithFields(logrus.Fields{
			logfields.Timeout: trueTimeout,
			"desiredTimeout":  timeout,
		}).Trace("Executing syscall with deadline")
	}

	done := make(chan error, 1)
	go func() {
		done <- f()
	}()

	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			log.G(ctx).WithField(logfields.Timeout, trueTimeout).
				Warning("Syscall did not complete within operation timeout. This may indicate a platform issue. " +
					"If it appears to be making no forward progress, obtain the stacks and see if there is a syscall " +
					"stuck in the platform API for a significant length of time.")
		}
		return ctx.Err()
	case err := <-done:
		return err
	}

}
