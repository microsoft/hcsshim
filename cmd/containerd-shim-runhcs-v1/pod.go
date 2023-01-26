package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/runtime"
	"github.com/containerd/containerd/runtime/v2/task"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

// shimPod represents the logical grouping of all tasks in a single set of
// shared namespaces. The pod sandbox (container) is represented by the task
// that matches the `shimPod.ID()`
type shimPod interface {
	// ID is the id of the task representing the pause (sandbox) container.
	ID() string
	// CreateTask creates a workload task within this pod named `tid` with
	// settings `s`.
	//
	// If `tid==ID()` or `tid` is the same as any other task in this pod, this
	// pod MUST return `errdefs.ErrAlreadyExists`.
	CreateTask(ctx context.Context, req *task.CreateTaskRequest, s *specs.Spec) (shimTask, error)
	// GetTask returns a task in this pod that matches `tid`.
	//
	// If `tid` is not found, this pod MUST return `errdefs.ErrNotFound`.
	GetTask(tid string) (shimTask, error)
	// KillTask sends `signal` to task that matches `tid`.
	//
	// If `tid` is not found, this pod MUST return `errdefs.ErrNotFound`.
	//
	// If `tid==ID() && eid == "" && all == true` this pod will send `signal` to
	// all tasks in the pod and lastly send `signal` to the sandbox itself.
	//
	// If `all == true && eid != ""` this pod MUST return
	// `errdefs.ErrFailedPrecondition`.
	//
	// A call to `KillTask` is only valid when the exec found by `tid,eid` is in
	// the `shimExecStateRunning, shimExecStateExited` states. If the exec is
	// not in this state this pod MUST return `errdefs.ErrFailedPrecondition`.
	KillTask(ctx context.Context, tid, eid string, signal uint32, all bool) error
}

func createPod(ctx context.Context, events publisher, req *task.CreateTaskRequest, s *specs.Spec) (shimPod, error) {
	log.G(ctx).WithField("tid", req.ID).Debug("createPod")

	if osversion.Build() < osversion.RS5 {
		return nil, errors.Wrapf(errdefs.ErrFailedPrecondition, "pod support is not available on Windows versions previous to RS5 (%d)", osversion.RS5)
	}

	ct, sid, err := oci.GetSandboxTypeAndID(s.Annotations)
	if err != nil {
		return nil, err
	}
	if ct != oci.KubernetesContainerTypeSandbox {
		return nil, errors.Wrapf(
			errdefs.ErrFailedPrecondition,
			"expected annotation: '%s': '%s' got '%s'",
			oci.KubernetesContainerTypeAnnotation,
			oci.KubernetesContainerTypeSandbox,
			ct)
	}
	if sid != req.ID {
		return nil, errors.Wrapf(
			errdefs.ErrFailedPrecondition,
			"expected annotation '%s': '%s' got '%s'",
			oci.KubernetesSandboxIDAnnotation,
			req.ID,
			sid)
	}

	owner := filepath.Base(os.Args[0])
	isWCOW := oci.IsWCOW(s)

	p := pod{
		events: events,
		id:     req.ID,
	}

	var parent *uvm.UtilityVM
	var lopts *uvm.OptionsLCOW
	if oci.IsIsolated(s) {
		// Create the UVM parent
		opts, err := oci.SpecToUVMCreateOpts(ctx, s, fmt.Sprintf("%s@vm", req.ID), owner)
		if err != nil {
			return nil, err
		}
		switch opts.(type) {
		case *uvm.OptionsLCOW:
			lopts = (opts).(*uvm.OptionsLCOW)
			parent, err = uvm.CreateLCOW(ctx, lopts)
			if err != nil {
				return nil, err
			}
		case *uvm.OptionsWCOW:
			wopts := (opts).(*uvm.OptionsWCOW)

			// In order for the UVM sandbox.vhdx not to collide with the actual
			// nested Argon sandbox.vhdx we append the \vm folder to the last
			// entry in the list.
			layersLen := len(s.Windows.LayerFolders)
			layers := make([]string, layersLen)
			copy(layers, s.Windows.LayerFolders)

			vmPath := filepath.Join(layers[layersLen-1], "vm")
			err := os.MkdirAll(vmPath, 0)
			if err != nil {
				return nil, err
			}
			layers[layersLen-1] = vmPath
			wopts.LayerFolders = layers

			parent, err = uvm.CreateWCOW(ctx, wopts)
			if err != nil {
				return nil, err
			}
		}
		err = parent.Start(ctx)
		if err != nil {
			parent.Close()
			return nil, err
		}

		if lopts != nil {
			err := parent.SetSecurityPolicy(ctx, lopts.SecurityPolicy)
			if err != nil {
				return nil, errors.Wrap(err, "unable to set security policy")
			}
		}
	} else if oci.IsJobContainer(s) {
		// If we're making a job container fake a task (i.e reuse the wcowPodSandbox logic)
		p.sandboxTask = newWcowPodSandboxTask(ctx, events, req.ID, req.Bundle, parent, "")
		if err := events.publishEvent(
			ctx,
			runtime.TaskCreateEventTopic,
			&eventstypes.TaskCreate{
				ContainerID: req.ID,
				Bundle:      req.Bundle,
				Rootfs:      req.Rootfs,
				IO: &eventstypes.TaskIO{
					Stdin:    req.Stdin,
					Stdout:   req.Stdout,
					Stderr:   req.Stderr,
					Terminal: req.Terminal,
				},
				Checkpoint: "",
				Pid:        0,
			}); err != nil {
			return nil, err
		}
		p.jobContainer = true
		return &p, nil
	} else if !isWCOW {
		return nil, errors.Wrap(errdefs.ErrFailedPrecondition, "oci spec does not contain WCOW or LCOW spec")
	}

	defer func() {
		// clean up the uvm if we fail any further operations
		if err != nil && parent != nil {
			parent.Close()
		}
	}()

	p.host = parent
	if parent != nil {
		cid := req.ID
		if id, ok := s.Annotations[oci.AnnotationNcproxyContainerID]; ok {
			cid = id
		}
		caAddr := fmt.Sprintf(uvm.ComputeAgentAddrFmt, cid)
		if err := parent.CreateAndAssignNetworkSetup(ctx, caAddr, cid); err != nil {
			return nil, err
		}
	}

	// TODO: JTERRY75 - There is a bug in the compartment activation for Windows
	// Process isolated that requires us to create the real pause container to
	// hold the network compartment open. This is not required for Windows
	// Hypervisor isolated. When we have a build that supports this for Windows
	// Process isolated make sure to move back to this model.

	// For WCOW we fake out the init task since we dont need it. We only
	// need to provision the guest network namespace if this is hypervisor
	// isolated. Process isolated WCOW gets the namespace endpoints
	// automatically.
	nsid := ""
	if isWCOW && parent != nil {
		if s.Windows != nil && s.Windows.Network != nil {
			nsid = s.Windows.Network.NetworkNamespace
		}

		if nsid != "" {
			if err := parent.ConfigureNetworking(ctx, nsid); err != nil {
				return nil, errors.Wrapf(err, "failed to setup networking for pod %q", req.ID)
			}
		}
		p.sandboxTask = newWcowPodSandboxTask(ctx, events, req.ID, req.Bundle, parent, nsid)
		// Publish the created event. We only do this for a fake WCOW task. A
		// HCS Task will event itself based on actual process lifetime.
		if err := events.publishEvent(
			ctx,
			runtime.TaskCreateEventTopic,
			&eventstypes.TaskCreate{
				ContainerID: req.ID,
				Bundle:      req.Bundle,
				Rootfs:      req.Rootfs,
				IO: &eventstypes.TaskIO{
					Stdin:    req.Stdin,
					Stdout:   req.Stdout,
					Stderr:   req.Stderr,
					Terminal: req.Terminal,
				},
				Checkpoint: "",
				Pid:        0,
			}); err != nil {
			return nil, err
		}
	} else {
		if isWCOW {
			defaultArgs := "c:\\windows\\system32\\cmd.exe"
			// For the default pause image, the  entrypoint
			// used is pause.exe
			// If the default pause image is not used for pause containers,
			// the activation will immediately exit on Windows
			// because there is no command. We forcibly update the command here
			// to keep it alive only for non-default pause images.
			// TODO: This override can be completely removed from containerd/1.7
			if (len(s.Process.Args) == 1 && strings.EqualFold(s.Process.Args[0], defaultArgs)) ||
				strings.EqualFold(s.Process.CommandLine, defaultArgs) {
				log.G(ctx).Warning("Detected CMD override for pause container entrypoint." +
					"Please consider switching to a pause image with an explicit cmd set")
				s.Process.CommandLine = "cmd /c ping -t 127.0.0.1 > nul"
			}
		}
		// LCOW (and WCOW Process Isolated for the time being) requires a real
		// task for the sandbox.
		lt, err := newHcsTask(ctx, events, parent, true, req, s)
		if err != nil {
			return nil, err
		}
		p.sandboxTask = lt
	}
	return &p, nil
}

var _ = (shimPod)(&pod{})

type pod struct {
	events publisher
	// id is the id of the sandbox task when the pod is created.
	//
	// It MUST be treated as read only in the lifetime of the pod.
	id string
	// sandboxTask is the task that represents the sandbox.
	//
	// Note: The invariant `id==sandboxTask.ID()` MUST be true.
	//
	// It MUST be treated as read only in the lifetime of the pod.
	sandboxTask shimTask
	// host is the UtilityVM that is hosting `sandboxTask` if the task is
	// hypervisor isolated.
	//
	// It MUST be treated as read only in the lifetime of the pod.
	host *uvm.UtilityVM

	// jobContainer specifies whether this pod is for WCOW job containers only.
	//
	// It MUST be treated as read only in the lifetime of the pod.
	jobContainer bool

	workloadTasks sync.Map
}

func (p *pod) ID() string {
	return p.id
}

func (p *pod) GetCloneAnnotations(ctx context.Context, s *specs.Spec) (bool, string, error) {
	isTemplate, templateID, err := oci.ParseCloneAnnotations(ctx, s)
	if err != nil {
		return false, "", err
	} else if (isTemplate || templateID != "") && p.host == nil {
		return false, "", fmt.Errorf("save as template and creating clones is only supported for hyper-v isolated containers")
	}
	return isTemplate, templateID, nil
}

func (p *pod) CreateTask(ctx context.Context, req *task.CreateTaskRequest, s *specs.Spec) (_ shimTask, err error) {
	if req.ID == p.id {
		return nil, errors.Wrapf(errdefs.ErrAlreadyExists, "task with id: '%s' already exists", req.ID)
	}
	e, _ := p.sandboxTask.GetExec("")
	if e.State() != shimExecStateRunning {
		return nil, errors.Wrapf(errdefs.ErrFailedPrecondition, "task with id: '%s' cannot be created in pod: '%s' which is not running", req.ID, p.id)
	}

	_, ok := p.workloadTasks.Load(req.ID)
	if ok {
		return nil, errors.Wrapf(errdefs.ErrAlreadyExists, "task with id: '%s' already exists id pod: '%s'", req.ID, p.id)
	}

	if p.jobContainer {
		// This is a short circuit to make sure that all containers in a pod will have
		// the same IP address/be added to the same compartment.
		//
		// There will need to be OS work needed to support this scenario, so for now we need to block on
		// this.
		if !oci.IsJobContainer(s) {
			return nil, errors.New("cannot create a normal process isolated container if the pod sandbox is a job container")
		}
	}

	ct, sid, err := oci.GetSandboxTypeAndID(s.Annotations)
	if err != nil {
		return nil, err
	}
	if ct != oci.KubernetesContainerTypeContainer {
		return nil, errors.Wrapf(
			errdefs.ErrFailedPrecondition,
			"expected annotation: '%s': '%s' got '%s'",
			oci.KubernetesContainerTypeAnnotation,
			oci.KubernetesContainerTypeContainer,
			ct)
	}
	if sid != p.id {
		return nil, errors.Wrapf(
			errdefs.ErrFailedPrecondition,
			"expected annotation '%s': '%s' got '%s'",
			oci.KubernetesSandboxIDAnnotation,
			p.id,
			sid)
	}

	_, templateID, err := p.GetCloneAnnotations(ctx, s)
	if err != nil {
		return nil, err
	}

	var st shimTask
	if templateID != "" {
		st, err = newClonedHcsTask(ctx, p.events, p.host, false, req, s, templateID)
	} else {
		st, err = newHcsTask(ctx, p.events, p.host, false, req, s)
	}
	if err != nil {
		return nil, err
	}

	p.workloadTasks.Store(req.ID, st)
	return st, nil
}

func (p *pod) GetTask(tid string) (shimTask, error) {
	if tid == p.id {
		return p.sandboxTask, nil
	}
	raw, loaded := p.workloadTasks.Load(tid)
	if !loaded {
		return nil, errors.Wrapf(errdefs.ErrNotFound, "task with id: '%s' not found", tid)
	}
	return raw.(shimTask), nil
}

func (p *pod) KillTask(ctx context.Context, tid, eid string, signal uint32, all bool) error {
	t, err := p.GetTask(tid)
	if err != nil {
		return err
	}
	if all && eid != "" {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "cannot signal all with non empty ExecID: '%s'", eid)
	}
	eg := errgroup.Group{}
	if all && tid == p.id {
		// We are in a kill all on the sandbox task. Signal everything.
		p.workloadTasks.Range(func(key, value interface{}) bool {
			wt := value.(shimTask)
			eg.Go(func() error {
				return wt.KillExec(ctx, eid, signal, all)
			})

			// Iterate all. Returning false stops the iteration. See:
			// https://pkg.go.dev/sync#Map.Range
			return true
		})
	}
	eg.Go(func() error {
		return t.KillExec(ctx, eid, signal, all)
	})
	return eg.Wait()
}
