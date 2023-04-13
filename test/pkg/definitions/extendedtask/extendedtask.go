package extendedtask

import (
	internalextended "github.com/Microsoft/hcsshim/internal/extendedtask"
)

var NewExtendedTaskClient = internalextended.NewExtendedTaskClient

type ComputeProcessorInfoRequest = internalextended.ComputeProcessorInfoRequest
type ComputeProcessorInfoResponse = internalextended.ComputeProcessorInfoResponse
