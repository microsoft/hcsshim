package hcs

import (
	gcontext "context"
	"syscall"
	"time"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/timeout"
	"go.opencensus.io/trace"
)

func execute(ctx gcontext.Context, timeout time.Duration, f func() error) error {
	ctx, cancel := gcontext.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- f()
	}()
	select {
	case <-ctx.Done():
		if ctx.Err() == gcontext.DeadlineExceeded {
			log.G(ctx).WithField(logfields.Timeout, timeout).
				Warning("Syscall did not complete within operation timeout. This may indicate a platform issue. If it appears to be making no forward progress, obtain the stacks and see if there is a syscall stuck in the platform API for a significant length of time.")
		}
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func hcsEnumerateComputeSystemsContext(ctx gcontext.Context, query string, computeSystems **uint16, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsEnumerateComputeSystems")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("query", query))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsEnumerateComputeSystems(query, computeSystems, result)
	})
}

func hcsCreateComputeSystemContext(ctx gcontext.Context, id string, configuration string, identity syscall.Handle, computeSystem *hcsSystem, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsCreateComputeSystem")
	defer span.End()
	defer func() {
		if hr != ErrVmcomputeOperationPending {
			oc.SetSpanStatus(span, hr)
		}
	}()
	span.AddAttributes(
		trace.StringAttribute("id", id),
		trace.StringAttribute("configuration", configuration))

	return execute(ctx, timeout.SystemCreate, func() error {
		return hcsCreateComputeSystem(id, configuration, identity, computeSystem, result)
	})
}

func hcsOpenComputeSystemContext(ctx gcontext.Context, id string, computeSystem *hcsSystem, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsOpenComputeSystem")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsOpenComputeSystem(id, computeSystem, result)
	})
}

func hcsCloseComputeSystemContext(ctx gcontext.Context, computeSystem hcsSystem) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsCloseComputeSystem")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsCloseComputeSystem(computeSystem)
	})
}

func hcsStartComputeSystemContext(ctx gcontext.Context, computeSystem hcsSystem, options string, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsStartComputeSystem")
	defer span.End()
	defer func() {
		if hr != ErrVmcomputeOperationPending {
			oc.SetSpanStatus(span, hr)
		}
	}()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SystemStart, func() error {
		return hcsStartComputeSystem(computeSystem, options, result)
	})
}

func hcsShutdownComputeSystemContext(ctx gcontext.Context, computeSystem hcsSystem, options string, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsShutdownComputeSystem")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsShutdownComputeSystem(computeSystem, options, result)
	})
}

func hcsTerminateComputeSystemContext(ctx gcontext.Context, computeSystem hcsSystem, options string, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsTerminateComputeSystem")
	defer span.End()
	defer func() {
		if hr != ErrVmcomputeOperationPending {
			oc.SetSpanStatus(span, hr)
		}
	}()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsTerminateComputeSystem(computeSystem, options, result)
	})
}

func hcsPauseComputeSystemContext(ctx gcontext.Context, computeSystem hcsSystem, options string, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsPauseComputeSystem")
	defer span.End()
	defer func() {
		if hr != ErrVmcomputeOperationPending {
			oc.SetSpanStatus(span, hr)
		}
	}()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SystemPause, func() error {
		return hcsPauseComputeSystem(computeSystem, options, result)
	})
}

func hcsResumeComputeSystemContext(ctx gcontext.Context, computeSystem hcsSystem, options string, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsResumeComputeSystem")
	defer span.End()
	defer func() {
		if hr != ErrVmcomputeOperationPending {
			oc.SetSpanStatus(span, hr)
		}
	}()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SystemResume, func() error {
		return hcsResumeComputeSystem(computeSystem, options, result)
	})
}

func hcsGetComputeSystemPropertiesContext(ctx gcontext.Context, computeSystem hcsSystem, propertyQuery string, properties **uint16, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsGetComputeSystemProperties")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("propertyQuery", propertyQuery))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsGetComputeSystemProperties(computeSystem, propertyQuery, properties, result)
	})
}

func hcsModifyComputeSystemContext(ctx gcontext.Context, computeSystem hcsSystem, configuration string, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsModifyComputeSystem")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("configuration", configuration))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsModifyComputeSystem(computeSystem, configuration, result)
	})
}

func hcsRegisterComputeSystemCallbackContext(ctx gcontext.Context, computeSystem hcsSystem, callback uintptr, context uintptr, callbackHandle *hcsCallback) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsRegisterComputeSystemCallback")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsRegisterComputeSystemCallback(computeSystem, callback, context, callbackHandle)
	})
}

func hcsUnregisterComputeSystemCallbackContext(ctx gcontext.Context, callbackHandle hcsCallback) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsUnregisterComputeSystemCallback")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsUnregisterComputeSystemCallback(callbackHandle)
	})
}

func hcsCreateProcessContext(ctx gcontext.Context, computeSystem hcsSystem, processParameters string, processInformation *hcsProcessInformation, process *hcsProcess, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsCreateProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("processParameters", processParameters))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsCreateProcess(computeSystem, processParameters, processInformation, process, result)
	})
}

func hcsOpenProcessContext(ctx gcontext.Context, computeSystem hcsSystem, pid uint32, process *hcsProcess, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsOpenProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.Int64Attribute("pid", int64(pid)))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsOpenProcess(computeSystem, pid, process, result)
	})
}

func hcsCloseProcessContext(ctx gcontext.Context, process hcsProcess) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsCloseProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsCloseProcess(process)
	})
}

func hcsTerminateProcessContext(ctx gcontext.Context, process hcsProcess, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsTerminateProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsTerminateProcess(process, result)
	})
}

func hcsSignalProcessContext(ctx gcontext.Context, process hcsProcess, options string, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsSignalProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("options", options))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsSignalProcess(process, options, result)
	})
}

func hcsGetProcessInfoContext(ctx gcontext.Context, process hcsProcess, processInformation *hcsProcessInformation, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsGetProcessInfo")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsGetProcessInfo(process, processInformation, result)
	})
}

func hcsGetProcessPropertiesContext(ctx gcontext.Context, process hcsProcess, processProperties **uint16, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsGetProcessProperties")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsGetProcessProperties(process, processProperties, result)
	})
}

func hcsModifyProcessContext(ctx gcontext.Context, process hcsProcess, settings string, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsModifyProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("settings", settings))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsModifyProcess(process, settings, result)
	})
}

func hcsGetServicePropertiesContext(ctx gcontext.Context, propertyQuery string, properties **uint16, result **uint16) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsGetServiceProperties")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()
	span.AddAttributes(trace.StringAttribute("propertyQuery", propertyQuery))

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsGetServiceProperties(propertyQuery, properties, result)
	})
}

func hcsRegisterProcessCallbackContext(ctx gcontext.Context, process hcsProcess, callback uintptr, context uintptr, callbackHandle *hcsCallback) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsRegisterProcessCallback")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsRegisterProcessCallback(process, callback, context, callbackHandle)
	})
}

func hcsUnregisterProcessCallbackContext(ctx gcontext.Context, callbackHandle hcsCallback) (hr error) {
	ctx, span := trace.StartSpan(ctx, "HcsUnregisterProcessCallback")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, hr) }()

	return execute(ctx, timeout.SyscallWatcher, func() error {
		return hcsUnregisterProcessCallback(callbackHandle)
	})
}
