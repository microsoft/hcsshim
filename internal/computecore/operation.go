//go:build windows

package computecore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"syscall"
	"time"

	"github.com/Microsoft/hcsshim/internal/jobobject"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"

	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"golang.org/x/sys/windows"
)

type HCSOperation windows.Handle

func (op HCSOperation) String() string {
	return "0x" + strconv.FormatInt(int64(op), 16)
}

// The workflow is as follows:
//   1. Create an operation
//   2. Add resources to the operation
//   3. Wait for operation result
//   4. Create compute system using operation
//   5. Close operation
// The compute system can be created without operation set, so it's optional.
// Presumably memory/cpu jobs can be added separately.
// Should adding resources be part of operation package?

//go:generate go run golang.org/x/tools/cmd/stringer -type=HCSOperationType -trimprefix=OperationType operation.go

type HCSOperationType int32

const (
	OperationTypeNone                 = HCSOperationType(-1)
	OperationTypeEnumerate            = HCSOperationType(0)
	OperationTypeCreate               = HCSOperationType(1)
	OperationTypeStart                = HCSOperationType(2)
	OperationTypeShutdown             = HCSOperationType(3)
	OperationTypePause                = HCSOperationType(4)
	OperationTypeResume               = HCSOperationType(5)
	OperationTypeSave                 = HCSOperationType(6)
	OperationTypeTerminate            = HCSOperationType(7)
	OperationTypeModify               = HCSOperationType(8)
	OperationTypeGetProperties        = HCSOperationType(9)
	OperationTypeCreateProcess        = HCSOperationType(10)
	OperationTypeSignalProcess        = HCSOperationType(11)
	OperationTypeGetProcessInfo       = HCSOperationType(12)
	OperationTypeGetProcessProperties = HCSOperationType(13)
	OperationTypeModifyProcess        = HCSOperationType(14)
	OperationTypeCrash                = HCSOperationType(15)
)

// !Note: documentation has callback and context reversed, but source (and testing) has context first
//	 HCS_OPERATION WINAPI
//	 HcsCreateOperation(
//	     _In_opt_ void*                    context
//	     _In_opt_ HCS_OPERATION_COMPLETION callback
//	     )
//sys hcsCreateOperation(context uintptr, callback hcsOperationCompletionUintptr) (op HCSOperation, err error) = computecore.HcsCreateOperation?

type HCSOperationOpt func(ctx context.Context, op HCSOperation) error

type CreateOperationOptions struct {
	JobResources []HCSJobResource
}

func HcsCreateOperation(ctx context.Context, hcsContext uintptr, callback hcsOperationCompletionUintptr, options *CreateOperationOptions) (op HCSOperation, err error) {
	op, err = createOperation(ctx, hcsContext, callback)
	if err != nil {
		return 0, err
	}

	if options != nil {
		if len(options.JobResources) > 0 {
			for _, jr := range options.JobResources {
				if err := op.addJobResource(ctx, jr); err != nil {
					log.G(ctx).WithError(err).Debug("failed to add job to resource")
					return 0, err
				}
			}
		}
	}

	return op, nil
}

func HcsCreateEmptyOperation(ctx context.Context) (op HCSOperation, err error) {
	return HcsCreateOperation(ctx, 0, 0, nil)
}

func createOperation(ctx context.Context, hcsContext uintptr, callback hcsOperationCompletionUintptr) (op HCSOperation, err error) {
	_, span := oc.StartSpan(ctx, "computecore::HcsCreateOperation", oc.WithClientSpanKind)
	defer func() {
		span.AddAttributes(trace.StringAttribute("operation", op.String()))
		oc.SetSpanStatus(span, err)
		span.End()
	}()
	span.AddAttributes(
		trace.Int64Attribute("context", int64(hcsContext)),
		trace.Int64Attribute("callback", int64(callback)),
	)
	return hcsCreateOperation(hcsContext, callback)
}

//   HRESULT WINAPI
//   HcsAddResourceToOperation(
//       _In_ HCS_OPERATION Operation,
//       HCS_RESOURCE_TYPE  Type,
//       _In_ PCWSTR        Uri,
//       HANDLE             Handle
//       )
//sys hcsAddResourceToOperation(operation HCSOperation, rtype uint32, uri string, handle syscall.Handle) (hr error) = computecore.HcsAddResourceToOperation?

func (op HCSOperation) addJobResource(ctx context.Context, jr HCSJobResource) (err error) {
	jobOpts := jobobject.Options{
		Name: jr.Name,
	}
	log.G(ctx).WithField("jobName", jr.Name).Debug("opening job object")
	job, err := jobobject.Open(ctx, &jobOpts)
	if err != nil {
		return err
	}
	defer func() {
		log.G(ctx).WithField("jobName", jr.Name).Debug("closing job object")
		if err := job.Close(); err != nil {
			log.G(ctx).WithError(err).Error("failed to close job object")
		}
	}()

	rHandle := job.Handle()

	return op.addResource(ctx, ResourceTypeJob, jr.Uri, syscall.Handle(rHandle))
}

func (op HCSOperation) addResource(ctx context.Context, rtype HCSResourceType, uri HCSResourceUri, handle syscall.Handle) (err error) {
	_, span := oc.StartSpan(ctx, "computecore::HcsAddResourceToOperation", oc.WithClientSpanKind)
	defer func() {
		span.AddAttributes(trace.StringAttribute("operation", op.String()))
		oc.SetSpanStatus(span, err)
		span.End()
	}()
	span.AddAttributes(
		trace.Int64Attribute("rtype", int64(rtype)),
		trace.StringAttribute("uri", string(uri)),
	)

	return hcsAddResourceToOperation(op, uint32(rtype), string(uri), handle)
}

//   void WINAPI
//   HcsCloseOperation(
//	     _In_ HCS_OPERATION operation
//	     )
//sys hcsCloseOperation(operation HCSOperation) (hr error) = computecore.HcsCloseOperation?

func (op HCSOperation) Close() error {
	return hcsCloseOperation(op)
}

//   HCS_OPERATION_TYPE WINAPI
//   HcsGetOperationType(
//       HCS_OPERATION Operation
//       )
//sys hcsGetOperationType(operation HCSOperation) (t HCSOperationType, err error) = computecore.HcsGetOperationType?

func (op HCSOperation) Type() (HCSOperationType, error) {
	return hcsGetOperationType(op)
}

//   UINT64 WINAPI
//   HcsGetOperationId(
//       HCS_OPERATION Operation
//       )
//sys hcsGetOperationId(operation HCSOperation) (id uint64, err error) = computecore.HcsGetOperationId?

func (op HCSOperation) ID() (uint64, error) {
	return hcsGetOperationId(op)
}

//   HRESULT WINAPI
//   HcsGetOperationResult(
//       HCS_OPERATION       Operation,
//       _Outptr_opt_ PWSTR* ResultDocument
//       )
//sys hcsGetOperationResult(operation HCSOperation, resultDocument **uint16) (hr error) = computecore.HcsGetOperationResult?

// TODO (maksiman): needs implementation

//   HRESULT WINAPI
//   HcsWaitForOperationResult(
//       HCS_OPERATION       Operation,
//       DWORD               TimeoutMs,
//       _Outptr_opt_ PWSTR* ResultDocument
//       )
//sys hcsWaitForOperationResult(operation HCSOperation, timeoutMs uint32, resultDocument **uint16) (hr error) = computecore.HcsWaitForOperationResult?

func (op HCSOperation) WaitForResult(ctx context.Context) (result string, err error) {
	ctx, span := oc.StartSpan(ctx, "computecore::HcsWaitForOperationResult", oc.WithClientSpanKind)
	defer func() {
		if result != "" {
			span.AddAttributes(trace.StringAttribute("resultDocument", result))
		}
		oc.SetSpanStatus(span, err)
		span.End()
	}()

	milli := contextTimeoutMs(ctx)
	span.AddAttributes(
		trace.StringAttribute("operation", op.String()),
		trace.Int64Attribute("timeoutMs", int64(milli)),
	)

	var opErr error
	var resultp *uint16
	done := make(chan struct{})
	go func() {
		defer close(done)

		log.G(ctx).Trace("waiting on operation")
		opErr = hcsWaitForOperationResult(op, milli, &resultp)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return processResults(ctx, bufferToString(resultp), opErr)
}

func contextTimeoutMs(ctx context.Context) uint32 {
	deadline, ok := ctx.Deadline()
	if !ok {
		return math.MaxUint32
	}

	timeout := time.Until(deadline).Milliseconds()
	if timeout > math.MaxUint32 {
		return math.MaxUint32 - 1
	}
	if timeout < 0 {
		return 0
	}
	return uint32(timeout)
}

// if err != nil, try to parse result as an [hcsschema.ResultError].
func processResults(ctx context.Context, result string, err error) (string, error) {
	if err == nil || result == "" {
		// if there is not error or if the result document is empty, do nothing
		return result, err
	}

	re := hcsschema.ResultError{}
	if jErr := json.Unmarshal([]byte(result), &re); jErr != nil {
		// if unmarshalling fails, ignore it and move on
		// not really a worthy enough error to raise it or log at Info or above
		log.G(ctx).WithError(jErr).Debugf("failed to unmarshal result as a %T", re)
		return result, err
	}

	// resultDocument will be set to "", so log it here in case its needed to debug error parsing
	log.G(ctx).WithField("resultDocument", result).Tracef("parsed operation result document as %T", re)

	// err should be (🤞) the same as `re.Error_`, but validate just in case
	if eno := windows.Errno(0x0); errors.As(err, &eno) {
		eno64 := uint64(eno)
		// convert to uint32 first to prevent leftpadding
		hr64 := uint64(uint32(re.Error_))
		if hr64 != eno64 {
			log.G(ctx).WithFields(logrus.Fields{
				"operationError": strconv.FormatUint(eno64, 16),
				"resultError":    strconv.FormatUint(hr64, 16),
			}).Warning("error mismatch between operation error and result error; overriding result error value")
			re.Error_ = int32(math.MaxUint32 & eno)
			re.ErrorMessage = eno.Error()
		}
	} else {
		log.G(ctx).WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"expectedType":  fmt.Sprintf("%T", eno),
			"receivedType":  fmt.Sprintf("%T", err),
		}).Warning("unexpected expected error type")
	}

	// return ResultError instead of err
	return "", &re
}
