package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/osversion"
	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/runtime"
	"github.com/containerd/containerd/runtime/v2/task"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

func newHcsStandaloneTask(ctx context.Context, events publisher, req *task.CreateTaskRequest, s *specs.Spec) (shimTask, error) {
	log.G(ctx).WithField("tid", req.ID).Debug("newHcsStandaloneTask")

	ct, _, err := oci.GetSandboxTypeAndID(s.Annotations)
	if err != nil {
		return nil, err
	}
	if ct != oci.KubernetesContainerTypeNone {
		return nil, errors.Wrapf(
			errdefs.ErrFailedPrecondition,
			"cannot create standalone task, expected no annotation: '%s': got '%s'",
			oci.KubernetesContainerTypeAnnotation,
			ct)
	}

	owner := filepath.Base(os.Args[0])

	var parent *uvm.UtilityVM
	if osversion.Get().Build >= osversion.RS5 && oci.IsIsolated(s) {
		// Create the UVM parent
		opts, err := oci.SpecToUVMCreateOpts(ctx, s, fmt.Sprintf("%s@vm", req.ID), owner)
		if err != nil {
			return nil, err
		}
		switch opts.(type) {
		case *uvm.OptionsLCOW:
			lopts := (opts).(*uvm.OptionsLCOW)
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
		}
	} else if !oci.IsWCOW(s) {
		return nil, errors.Wrap(errdefs.ErrFailedPrecondition, "oci spec does not contain WCOW or LCOW spec")
	}

	shim, err := newHcsTask(ctx, events, parent, true, req, s)
	if err != nil {
		if parent != nil {
			parent.Close()
		}
		return nil, err
	}
	return shim, nil
}

// newHcsTask creates a container within `parent` and its init exec process in
// the `shimExecCreated` state and returns the task that tracks its lifetime.
//
// If `parent == nil` the container is created on the host.
func newHcsTask(
	ctx context.Context,
	events publisher,
	parent *uvm.UtilityVM,
	ownsParent bool,
	req *task.CreateTaskRequest,
	s *specs.Spec) (_ shimTask, err error) {
	log.G(ctx).WithFields(logrus.Fields{
		"tid":        req.ID,
		"ownsParent": ownsParent,
	}).Debug("newHcsTask")

	owner := filepath.Base(os.Args[0])

	io, err := hcsoci.NewNpipeIO(ctx, req.Stdin, req.Stdout, req.Stderr, req.Terminal)
	if err != nil {
		return nil, err
	}

	var netNS string
	if s.Windows != nil &&
		s.Windows.Network != nil {
		netNS = s.Windows.Network.NetworkNamespace
	}
	opts := hcsoci.CreateOptions{
		ID:               req.ID,
		Owner:            owner,
		Spec:             s,
		HostingSystem:    parent,
		NetworkNamespace: netNS,
	}
	system, resources, err := hcsoci.CreateContainer(ctx, &opts)
	if err != nil {
		return nil, err
	}

	ht := &hcsTask{
		events:   events,
		id:       req.ID,
		isWCOW:   oci.IsWCOW(s),
		c:        system,
		cr:       resources,
		ownsHost: ownsParent,
		host:     parent,
		closed:   make(chan struct{}),
	}
	ht.init = newHcsExec(
		ctx,
		events,
		req.ID,
		parent,
		system,
		req.ID,
		req.Bundle,
		ht.isWCOW,
		s.Process,
		io)

	if parent != nil {
		// We have a parent UVM. Listen for its exit and forcibly close this
		// task. This is not expected but in the event of a UVM crash we need to
		// handle this case.
		go ht.waitForHostExit()
	}
	// In the normal case the `Signal` call from the caller killed this task's
	// init process.
	go ht.waitInitExit()

	// Publish the created event
	ht.events.publishEvent(
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
			Pid:        uint32(ht.init.Pid()),
		})
	return ht, nil
}

var _ = (shimTask)(&hcsTask{})

// hcsTask is a generic task that represents a WCOW Container (process or
// hypervisor isolated), or a LCOW Container. This task MAY own the UVM the
// container is in but in the case of a POD it may just track the UVM for
// container lifetime management. In the case of ownership when the init
// task/exec is stopped the UVM itself will be stopped as well.
type hcsTask struct {
	events publisher
	// id is the id of this task when it is created.
	//
	// It MUST be treated as read only in the liftetime of the task.
	id string
	// isWCOW is set to `true` if this is a task representing a Windows container.
	//
	// It MUST be treated as read only in the liftetime of the task.
	isWCOW bool
	// c is the container backing this task.
	//
	// It MUST be treated as read only in the lifetime of this task EXCEPT after
	// a Kill to the init task in which it must be shutdown.
	c cow.Container
	// cr is the container resources this task is holding.
	//
	// It MUST be treated as read only in the lifetime of this task EXCEPT after
	// a Kill to the init task in which all resources must be released.
	cr *hcsoci.Resources
	// init is the init process of the container.
	//
	// Note: the invariant `container state == init.State()` MUST be true. IE:
	// if the init process exits the container as a whole and all exec's MUST
	// exit.
	//
	// It MUST be treated as read only in the lifetime of the task.
	init shimExec
	// ownsHost is `true` if this task owns `host`. If so when this tasks init
	// exec shuts down it is required that `host` be shut down as well.
	ownsHost bool
	// host is the hosting VM for this exec if hypervisor isolated. If
	// `host==nil` this is an Argon task so no UVM cleanup is required.
	//
	// NOTE: if `osversion.Get().Build < osversion.RS5` this will always be
	// `nil`.
	host *uvm.UtilityVM

	// ecl is the exec create lock for all non-init execs and MUST be held
	// durring create to prevent ID duplication.
	ecl   sync.Mutex
	execs sync.Map

	closed    chan struct{}
	closeOnce sync.Once
	// closeHostOnce is used to close `host`. This will only be used if
	// `ownsHost==true` and `host != nil`.
	closeHostOnce sync.Once
}

func (ht *hcsTask) ID() string {
	return ht.id
}

func (ht *hcsTask) CreateExec(ctx context.Context, req *task.ExecProcessRequest, spec *specs.Process) error {
	ht.ecl.Lock()
	defer ht.ecl.Unlock()

	// If the task exists or we got a request for "" which is the init task
	// fail.
	if _, loaded := ht.execs.Load(req.ExecID); loaded || req.ExecID == "" {
		return errors.Wrapf(errdefs.ErrAlreadyExists, "exec: '%s' in task: '%s' already exists", req.ExecID, ht.id)
	}

	if ht.init.State() != shimExecStateRunning {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "exec: '' in task: '%s' must be running to create additional execs", ht.id)
	}

	io, err := hcsoci.NewNpipeIO(ctx, req.Stdin, req.Stdout, req.Stderr, req.Terminal)
	if err != nil {
		return err
	}
	he := newHcsExec(ctx, ht.events, ht.id, ht.host, ht.c, req.ExecID, ht.init.Status().Bundle, ht.isWCOW, spec, io)
	ht.execs.Store(req.ExecID, he)

	// Publish the created event
	ht.events.publishEvent(
		ctx,
		runtime.TaskExecAddedEventTopic,
		&eventstypes.TaskExecAdded{
			ContainerID: ht.id,
			ExecID:      req.ExecID,
		})

	return nil
}

func (ht *hcsTask) GetExec(eid string) (shimExec, error) {
	if eid == "" {
		return ht.init, nil
	}
	raw, loaded := ht.execs.Load(eid)
	if !loaded {
		return nil, errors.Wrapf(errdefs.ErrNotFound, "exec: '%s' in task: '%s' not found", eid, ht.id)
	}
	return raw.(shimExec), nil
}

func (ht *hcsTask) KillExec(ctx context.Context, eid string, signal uint32, all bool) error {
	e, err := ht.GetExec(eid)
	if err != nil {
		return err
	}
	if all && eid != "" {
		return errors.Wrapf(errdefs.ErrFailedPrecondition, "cannot signal all for non-empty exec: '%s'", eid)
	}
	if all {
		// We are in a kill all on the init task. Signal everything.
		ht.execs.Range(func(key, value interface{}) bool {
			err := value.(shimExec).Kill(ctx, signal)
			if err != nil {
				log.G(ctx).WithFields(logrus.Fields{
					"eid":           key,
					logrus.ErrorKey: err,
				}).Warn("failed to kill exec in task")
			}

			// iterate all
			return false
		})
	}
	if signal == 0x9 && eid == "" && ht.host != nil {
		// If this is a SIGKILL against the init process we start a background
		// timer and wait on either the timer expiring or the process exiting
		// cleanly. If the timer exires first we forcibly close the UVM as we
		// assume the guest is misbehaving for some reason.
		go func() {
			t := time.NewTimer(30 * time.Second)
			execExited := make(chan struct{})
			go func() {
				e.Wait()
				close(execExited)
			}()
			select {
			case <-execExited:
				t.Stop()
			case <-t.C:
				// Safe to call multiple times if called previously on
				// successful shutdown.
				ht.host.Close()
			}
		}()
	}
	return e.Kill(ctx, signal)
}

func (ht *hcsTask) DeleteExec(ctx context.Context, eid string) (int, uint32, time.Time, error) {
	e, err := ht.GetExec(eid)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	if eid == "" {
		// We are deleting the init exec. Forcibly exit any additional exec's.
		ht.execs.Range(func(key, value interface{}) bool {
			ex := value.(shimExec)
			if s := ex.State(); s != shimExecStateExited {
				ex.ForceExit(ctx, 1)
			}

			// iterate next
			return false
		})
	}
	switch state := e.State(); state {
	case shimExecStateCreated:
		e.ForceExit(ctx, 0)
	case shimExecStateRunning:
		return 0, 0, time.Time{}, newExecInvalidStateError(ht.id, eid, state, "delete")
	}
	status := e.Status()
	if eid != "" {
		ht.execs.Delete(eid)
	}

	// Publish the deleted event
	ht.events.publishEvent(
		ctx,
		runtime.TaskDeleteEventTopic,
		&eventstypes.TaskDelete{
			ContainerID: ht.id,
			ID:          eid,
			Pid:         status.Pid,
			ExitStatus:  status.ExitStatus,
			ExitedAt:    status.ExitedAt,
		})

	return int(status.Pid), status.ExitStatus, status.ExitedAt, nil
}

func (ht *hcsTask) Pids(ctx context.Context) ([]options.ProcessDetails, error) {
	// Map all user created exec's to pid/exec-id
	pidMap := make(map[int]string)
	ht.execs.Range(func(key, value interface{}) bool {
		ex := value.(shimExec)
		pidMap[ex.Pid()] = ex.ID()

		// Iterate all
		return false
	})
	pidMap[ht.init.Pid()] = ht.init.ID()

	// Get the guest pids
	props, err := ht.c.Properties(ctx, schema1.PropertyTypeProcessList)
	if err != nil {
		return nil, err
	}

	// Copy to pid/exec-id pair's
	pairs := make([]options.ProcessDetails, len(props.ProcessList))
	for i, p := range props.ProcessList {
		pairs[i].ImageName = p.ImageName
		pairs[i].CreatedAt = p.CreateTimestamp
		pairs[i].KernelTime_100Ns = p.KernelTime100ns
		pairs[i].MemoryCommitBytes = p.MemoryCommitBytes
		pairs[i].MemoryWorkingSetPrivateBytes = p.MemoryWorkingSetPrivateBytes
		pairs[i].MemoryWorkingSetSharedBytes = p.MemoryWorkingSetSharedBytes
		pairs[i].ProcessID = p.ProcessId
		pairs[i].UserTime_100Ns = p.KernelTime100ns

		if eid, ok := pidMap[int(p.ProcessId)]; ok {
			pairs[i].ExecID = eid
		}
	}
	return pairs, nil
}

func (ht *hcsTask) Wait() *task.StateResponse {
	<-ht.closed
	return ht.init.Wait()
}

func (ht *hcsTask) waitInitExit() {
	ctx, span := trace.StartSpan(context.Background(), "hcsTask::waitInitExit")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("tid", ht.id))

	// Wait for it to exit on its own
	ht.init.Wait()

	// Close the host and event the exit
	ht.close(ctx)
}

// waitForHostExit waits for the host virtual machine to exit. Once exited
// forcibly exits all additional exec's in this task.
//
// This MUST be called via a goroutine to wait on a background thread.
//
// Note: For Windows process isolated containers there is no host virtual
// machine so this should not be called.
func (ht *hcsTask) waitForHostExit() {
	ctx, span := trace.StartSpan(context.Background(), "hcsTask::waitForHostExit")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("tid", ht.id))

	err := ht.host.Wait()
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to wait for host virtual machine exit")
	} else {
		log.G(ctx).Debug("host virtual machine exited")
	}

	ht.execs.Range(func(key, value interface{}) bool {
		ex := value.(shimExec)
		ex.ForceExit(ctx, 1)

		// iterate all
		return false
	})
	ht.init.ForceExit(ctx, 1)
	ht.closeHost(ctx)
}

// close shuts down the container that is owned by this task and if
// `ht.ownsHost` will shutdown the hosting VM the container was placed in.
//
// NOTE: For Windows process isolated containers `ht.ownsHost==true && ht.host
// == nil`.
func (ht *hcsTask) close(ctx context.Context) {
	ht.closeOnce.Do(func() {
		log.G(ctx).Debug("hcsTask::closeOnce")

		// ht.c should never be nil for a real task but in testing we stub
		// this to avoid a nil dereference. We really should introduce a
		// method or interface for ht.c operations that we can stub for
		// testing.
		if ht.c != nil {
			// Do our best attempt to tear down the container.
			var werr error
			ch := make(chan struct{})
			go func() {
				werr = ht.c.Wait()
				close(ch)
			}()
			err := ht.c.Shutdown(ctx)
			if err != nil {
				log.G(ctx).WithError(err).Error("failed to shutdown container")
			} else {
				t := time.NewTimer(time.Second * 30)
				select {
				case <-ch:
					err = werr
					t.Stop()
					if err != nil {
						log.G(ctx).WithError(err).Error("failed to wait for container shutdown")
					}
				case <-t.C:
					log.G(ctx).WithError(hcs.ErrTimeout).Error("failed to wait for container shutdown")
				}
			}

			if err != nil {
				err = ht.c.Terminate(ctx)
				if err != nil {
					log.G(ctx).WithError(err).Error("failed to terminate container")
				} else {
					t := time.NewTimer(time.Second * 30)
					select {
					case <-ch:
						err = werr
						t.Stop()
						if err != nil {
							log.G(ctx).WithError(err).Error("failed to wait for container terminate")
						}
					case <-t.C:
						log.G(ctx).WithError(hcs.ErrTimeout).Error("failed to wait for container terminate")
					}
				}
			}

			// Release any resources associated with the container.
			if err := hcsoci.ReleaseResources(ctx, ht.cr, ht.host, true); err != nil {
				log.G(ctx).WithError(err).Error("failed to release container resources")
			}

			// Close the container handle invalidating all future access.
			if err := ht.c.Close(); err != nil {
				log.G(ctx).WithError(err).Error("failed to close container")
			}
		}
		ht.closeHost(ctx)
	})
}

// closeHost safely closes the hosting UVM if this task is the owner. Once
// closed and all resources released it events the `runtime.TaskExitEventTopic`
// for all upstream listeners.
//
// Note: If this is a process isolated task the hosting UVM is simply a `noop`.
//
// This call is idempotent and safe to call multiple times.
func (ht *hcsTask) closeHost(ctx context.Context) {
	ht.closeHostOnce.Do(func() {
		log.G(ctx).Debug("hcsTask::closeHostOnce")

		if ht.ownsHost && ht.host != nil {
			if err := ht.host.Close(); err != nil {
				log.G(ctx).WithError(err).Error("failed host vm shutdown")
			}
		}
		// Send the `init` exec exit notification always.
		exit := ht.init.Status()
		ht.events.publishEvent(
			ctx,
			runtime.TaskExitEventTopic,
			&eventstypes.TaskExit{
				ContainerID: ht.id,
				ID:          exit.ID,
				Pid:         uint32(exit.Pid),
				ExitStatus:  exit.ExitStatus,
				ExitedAt:    exit.ExitedAt,
			})
		close(ht.closed)
	})
}

func (ht *hcsTask) ExecInHost(ctx context.Context, req *shimdiag.ExecProcessRequest) (int, error) {
	if ht.host == nil {
		return 0, errors.New("task is not isolated")
	}
	return hcsoci.ExecInUvm(ctx, ht.host, req)
}

func (ht *hcsTask) DumpGuestStacks(ctx context.Context) string {
	if ht.host != nil {
		stacks, err := ht.host.DumpStacks(ctx)
		if err != nil {
			log.G(ctx).WithError(err).Warn("failed to capture guest stacks")
		} else {
			return stacks
		}
	}
	return ""
}

func (ht *hcsTask) Share(ctx context.Context, req *shimdiag.ShareRequest) error {
	if ht.host == nil {
		return errors.New("task is not isolated")
	}
	// For hyper-v isolated WCOW the task used isn't the standard hcsTask so we
	// only have to deal with the LCOW case here.
	st, err := os.Stat(req.HostPath)
	if err != nil {
		return fmt.Errorf("could not open '%s' path on host: %s", req.HostPath, err)
	}
	var (
		hostPath       string = req.HostPath
		restrictAccess bool
		fileName       string
		allowedNames   []string
	)
	if !st.IsDir() {
		hostPath, fileName = filepath.Split(hostPath)
		allowedNames = append(allowedNames, fileName)
		restrictAccess = true
	}
	_, err = ht.host.AddPlan9(ctx, hostPath, req.UvmPath, req.ReadOnly, restrictAccess, allowedNames)
	return err
}

func hcsPropertiesToWindowsStats(props *hcsschema.Properties) *stats.Statistics_Windows {
	wcs := &stats.Statistics_Windows{Windows: &stats.WindowsContainerStatistics{}}
	if props.Statistics != nil {
		wcs.Windows.Timestamp = props.Statistics.Timestamp
		wcs.Windows.ContainerStartTime = props.Statistics.ContainerStartTime
		wcs.Windows.UptimeNS = props.Statistics.Uptime100ns * 100
		if props.Statistics.Processor != nil {
			wcs.Windows.Processor = &stats.WindowsContainerProcessorStatistics{
				TotalRuntimeNS:  props.Statistics.Processor.TotalRuntime100ns * 100,
				RuntimeUserNS:   props.Statistics.Processor.RuntimeUser100ns * 100,
				RuntimeKernelNS: props.Statistics.Processor.RuntimeKernel100ns * 100,
			}
		}
		if props.Statistics.Memory != nil {
			wcs.Windows.Memory = &stats.WindowsContainerMemoryStatistics{
				MemoryUsageCommitBytes:            props.Statistics.Memory.MemoryUsageCommitBytes,
				MemoryUsageCommitPeakBytes:        props.Statistics.Memory.MemoryUsageCommitPeakBytes,
				MemoryUsagePrivateWorkingSetBytes: props.Statistics.Memory.MemoryUsagePrivateWorkingSetBytes,
			}
		}
		if props.Statistics.Storage != nil {
			wcs.Windows.Storage = &stats.WindowsContainerStorageStatistics{
				ReadCountNormalized:  props.Statistics.Storage.ReadCountNormalized,
				ReadSizeBytes:        props.Statistics.Storage.ReadSizeBytes,
				WriteCountNormalized: props.Statistics.Storage.WriteCountNormalized,
				WriteSizeBytes:       props.Statistics.Storage.WriteSizeBytes,
			}
		}
	}
	return wcs
}

func (ht *hcsTask) Stats(ctx context.Context) (*stats.Statistics, error) {
	s := &stats.Statistics{}

	props, err := ht.c.PropertiesV2(ctx, hcsschema.PTStatistics)
	if err != nil {
		return nil, err
	}
	if ht.isWCOW {
		s.Container = hcsPropertiesToWindowsStats(props)
	} else {
		s.Container = &stats.Statistics_Linux{Linux: props.Metrics}
	}
	if ht.ownsHost && ht.host != nil {
		vmStats, err := ht.host.Stats(ctx)
		if err != nil {
			return nil, err
		}
		s.VM = vmStats
	}
	return s, nil
}
