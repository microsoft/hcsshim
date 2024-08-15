package hcs

import (
	"context"
	"fmt"
	"github.com/Microsoft/hcsshim/internal/computecore"
	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/Microsoft/hcsshim/internal/log"
	"sync"
)

type Operation struct {
	handle computecore.HCSOperation

	mutex sync.RWMutex

	operationType computecore.HCSOperationType
	operationJobs []*jobobject.JobObject
}

type JobResource struct {
	Name string
	Uri  computecore.HCSResourceUri
}
type CreateOperationOptions struct {
	JobResources []JobResource
}

func CreateOperation(ctx context.Context) (*Operation, error) {
	handle, err := computecore.HcsCreateEmptyOperation(ctx)
	if err != nil {
		return nil, err
	}

	op := &Operation{
		handle: handle,
	}

	return op, nil
}

func (op *Operation) AddResource(ctx context.Context, resource JobResource) (err error) {
	op.mutex.Lock()
	defer op.mutex.Unlock()

	jobOpts := jobobject.Options{
		Name:          resource.Name,
		InheritHandle: true,
	}

	log.G(ctx).WithField("jobName", resource.Name).Debug("opening job object")
	job, err := jobobject.Open(ctx, &jobOpts)
	if err != nil {
		return err
	}

	jobHandle := job.Handle()
	defer func() {
		if err != nil {
			if cErr := job.Close(); cErr != nil {
				log.G(ctx).WithError(cErr).Error("failed to close job object")
			}
			return
		}
		op.operationJobs = append(op.operationJobs, job)
	}()

	return computecore.AddResourceToOperation(ctx, op.handle, computecore.ResourceTypeJob, resource.Uri, jobHandle)
}

func (op *Operation) Close(ctx context.Context) error {
	op.mutex.Lock()
	defer op.mutex.Unlock()

	for _, job := range op.operationJobs {
		if err := job.Close(); err != nil {
			log.G(ctx).WithError(err).Error("failed to close job object")
		}
	}
	if err := op.handle.Close(); err != nil {
		return fmt.Errorf("failed to close operation: %w", err)
	}
	op.handle = 0
	return nil
}

func (op *Operation) Handle() computecore.HCSOperation {
	return op.handle
}

func NewMemoryResource(name string) JobResource {
	return JobResource{
		Name: name,
		Uri:  computecore.HCSMemoryJobUri,
	}
}

func NewCPUResource(name string) JobResource {
	return JobResource{
		Name: name,
		Uri:  computecore.HCSWorkerJobUri,
	}
}
