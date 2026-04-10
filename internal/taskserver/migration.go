package taskserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"sync"

	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/core"
	"github.com/Microsoft/hcsshim/internal/core/linuxvm"
	"github.com/Microsoft/hcsshim/internal/layers"
	lmproto "github.com/Microsoft/hcsshim/internal/lm/proto"
	"github.com/Microsoft/hcsshim/internal/log"
	statepkg "github.com/Microsoft/hcsshim/internal/state"
	"github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/runtime"
	"github.com/containerd/typeurl/v2"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type migrationState struct {
	l          windows.Handle
	c          windows.Handle
	migratable core.Migratable
	migrator   core.Migrator
	migrated   core.Migrated
	taskState  map[string]*statepkg.TaskState
	newID      string

	// Latest-only state for streaming
	latest    atomic.Value // stores *lmproto.TransferSandboxResponse
	seqMu     sync.Mutex
	seqCond   *sync.Cond
	seq       uint64 // incremented on every new latest
	completed bool   // set true when migration ends (FAILED or COMPLETE)

	// One-time start guard
	startOnce sync.Once
}

var _ lmproto.MigrationService = (*service)(nil)

func (s *service) PrepareSandbox(ctx context.Context, req *lmproto.PrepareSandboxRequest) (*lmproto.PrepareSandboxResponse, error) {
	if err := enableChecksumValidationRegKeys(); err != nil {
        logrus.WithError(err).Warn("failed to set checksum validation registry keys")
    }
	sandboxState, resources, err := s.sandbox.Sandbox.(core.Migratable).LMPrepare(ctx, req.InitializeOptions)
	if err != nil {
		return nil, fmt.Errorf("prepare sandbox for migration: %w", err)
	}
	state := &statepkg.TaskServerState{
		Sandbox: sandboxState,
		Tasks:   make(map[string]*statepkg.TaskState),
	}
	state.Tasks[s.sandbox.TaskID] = &statepkg.TaskState{
		TaskId: s.sandbox.TaskID,
		Pid:    s.sandbox.Pid,
	}
	for id, t := range s.sandbox.Tasks {
		state.Tasks[id] = &statepkg.TaskState{
			TaskId:   t.TaskID,
			ExecId:   t.ExecID,
			Terminal: t.Terminal,
			Pid:      t.Pid,
		}
	}
	stateAny, err := anypb.New(state)
	if err != nil {
		return nil, fmt.Errorf("marshal state as any: %w", err)
	}
	var outResources []*lmproto.SourceRootFS
	for _, r := range resources.Layers {
		outResources = append(outResources, &lmproto.SourceRootFS{Id: r.ResourceID, TaskId: r.ContainerID})
	}
	s.migState = &migrationState{
		migratable: s.sandbox.Sandbox.(core.Migratable),
		migrator:   s.sandbox.Sandbox.(core.Migratable),
	}
	s.mCond = sync.NewCond(&s.m)
	return &lmproto.PrepareSandboxResponse{
		SessionId: req.SessionId, // Session ID is not super important at this point.
		Config:    stateAny,
		Resources: &lmproto.SourceResources{TaskRootfs: outResources},
	}, nil
}

func (s *service) newSandboxLM(ctx context.Context, shimOpts *runhcsopts.Options, req *task.CreateTaskRequest) error {
	spec, err := getSandboxLMSpec(ctx, req.Bundle)
	if err != nil {
		return err
	}
	configRaw, err := typeurl.UnmarshalAny(spec.Config)
	if err != nil {
		return err
	}
	config, ok := configRaw.(*statepkg.TaskServerState)
	if !ok {
		return fmt.Errorf("expected TaskServerState, got %T instead", configRaw)
	}

	var resources core.Resources
	var replacements []*core.LayersReplacement
	for _, resource := range spec.Resources.TaskRootfs {
		l, err := layers.GetLCOWLayers([]*types.Mount{resource.Rootfs}, nil)
		if err != nil {
			return err
		}
		l2 := layers.GetLCOWLayers2(l)
		replacements = append(replacements, &core.LayersReplacement{ResourceID: resource.Id, Layers: l2})
		resources.Layers = append(resources.Layers, &core.LayersResource{ResourceID: resource.Id, ContainerID: resource.TaskId})
	}

	migrator, err := linuxvm.NewMigrator(ctx, req.ID, config.Sandbox, spec.Netns, spec.Annotations, &core.Replacements{Layers: replacements}, resources)
	if err != nil {
		return err
	}
	s.migState = &migrationState{
		newID:     req.ID,
		migrator:  migrator,
		taskState: config.Tasks,
	}
	return nil
}

func getSandboxLMSpec(ctx context.Context, bundle string) (*lmproto.SandboxLMSpec, error) {
	rawSpec, err := os.ReadFile(filepath.Join(bundle, "config.json"))
	if err != nil {
		return nil, err
	}
	var spec lmproto.SandboxLMSpec
	if err := (proto.UnmarshalOptions{}).Unmarshal(rawSpec, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

func (s *service) TransferSandbox(ctx context.Context, req *lmproto.TransferSandboxRequest, stream lmproto.Migration_TransferSandboxServer) error {
	if s.sandbox != nil {
		logrus.Info("aborting task waits")
		s.sandbox.waitCancel()
	}
	logrus.Info("TransferSandbox called")

	// We will wait for channel to be ready.
	timeout := time.Minute * 10
	if req.Timeout != nil {
		timeout = req.Timeout.AsDuration()
	}

	s.migState.startOnce.Do(func() {
		go s.runMigration(context.Background(), req.GetSessionId(), timeout)
	})

	s.ensureSeqCond()

	// Track what we have observed
	// Send current latest immediately if present
	if v := s.migState.latest.Load(); v != nil {
		msg := v.(*lmproto.TransferSandboxResponse)
		if err := stream.Send(msg); err != nil {
			return fmt.Errorf("initial stream send failed: %w", err)
		}
	}

	// Track observed generation
	s.migState.seqMu.Lock()
	seenSeq := s.migState.seq
	s.migState.seqMu.Unlock()

	// Loop: wait for next seq change, then read latest and send
	for {
		// Respect client cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		s.migState.seqMu.Lock()
		// Wait until a new message arrives or migration is completed
		for s.migState.seq == seenSeq && !s.migState.completed {
			s.migState.seqCond.Wait()
		}

		// If completed and no new message, we are done
		if s.migState.completed && s.migState.seq == seenSeq {
			s.migState.seqMu.Unlock()
			return s.terminalErrorIfAny()
		}

		// We have a new message (seq advanced)
		seenSeq = s.migState.seq
		s.migState.seqMu.Unlock()

		v := s.migState.latest.Load()
		if v == nil {
			// Shouldn’t happen: seq advanced but latest nil. Be defensive.
			continue
		}
		msg := v.(*lmproto.TransferSandboxResponse)

		if err := stream.Send(msg); err != nil {
			return fmt.Errorf("stream send failed: %w", err)
		}
	}
}

// must be called with s.migState.seqMu held if you need consistency with `completed` and `seq`
// but it's fine to load `latest` atomically outside the lock too.
func (s *service) terminalErrorIfAny() error {
	v := s.migState.latest.Load()
	if v == nil {
		return nil
	}
	msg := v.(*lmproto.TransferSandboxResponse)
	if msg.Status == lmproto.TransferStatus_TRANSFER_STATUS_FAILED {
		// prefer ErrorMessage; fallback to a generic error if empty
		if em := msg.ErrorMessage; em != "" {
			return fmt.Errorf(em)
		}
		return fmt.Errorf("migration failed")
	}
	return nil
}

// initialize seqCond lazily/safely
func (s *service) ensureSeqCond() {
	if s.migState.seqCond == nil {
		s.migState.seqCond = sync.NewCond(&s.migState.seqMu)
	}
}

func (s *service) runMigration(ctx context.Context, sessionID string, timeout time.Duration) {
	s.ensureSeqCond()

	channelReadyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if s.mCond == nil {
		s.mCond = sync.NewCond(&s.m)
	}
	done := make(chan struct{})
	go func() {
		s.m.Lock()
		defer s.m.Unlock()

		for s.migState == nil || s.migState.c == 0 {
			s.mCond.Wait()
		}
		close(done)
	}()

	select {
	case <-channelReadyCtx.Done():
		logrus.Warn("ChannelReady: context canceled or timed out")
		return
	case <-done:
		logrus.Infof("ChannelReady: Channel is ready %v", s.migState.c)
	}

	defer func() {
		if s.migState.c != 0 {
			windows.Closesocket(s.migState.c)
		}
	}()

	// Emit helper: write latest, bump seq, broadcast
	emit := func(msg *lmproto.TransferSandboxResponse) {
		s.migState.latest.Store(msg)
		s.migState.seqMu.Lock()
		s.migState.seq++
		s.migState.seqCond.Broadcast()
		s.migState.seqMu.Unlock()
	}

	start := time.Now()

	// Brownout
	emit(&lmproto.TransferSandboxResponse{
		SessionId:  sessionID,
		MessageId:  1,
		Status:     lmproto.TransferStatus_TRANSFER_STATUS_BROWNOUT_IN_PROGRESS,
		StartTime:  timestamppb.New(start),
		UpdateTime: timestamppb.Now(),
	})

	migrated, err := s.migState.migrator.LMTransfer(ctx, uintptr(s.migState.c))
	if err != nil {
		emit(&lmproto.TransferSandboxResponse{
			SessionId:    sessionID,
			MessageId:    2,
			Status:       lmproto.TransferStatus_TRANSFER_STATUS_FAILED,
			ErrorMessage: err.Error(),
			StartTime:    timestamppb.New(start),
			UpdateTime:   timestamppb.Now(),
		})
		// Mark completed and broadcast one more time to let readers exit.
		s.migState.seqMu.Lock()
		s.migState.completed = true
		s.migState.seqCond.Broadcast()
		s.migState.seqMu.Unlock()
		return
	}

	time.Sleep(time.Second * 1) // Emulate actual scenario
	logrus.Info("LM transfer complete")

	// Blackout in progress
	emit(&lmproto.TransferSandboxResponse{
		SessionId:  sessionID,
		MessageId:  2,
		Status:     lmproto.TransferStatus_TRANSFER_STATUS_BLACKOUT_IN_PROGRESS,
		StartTime:  timestamppb.New(start),
		UpdateTime: timestamppb.Now(),
		Progress:   0.5,
	})

	time.Sleep(time.Second * 1) // Emulate actual scenario
	// Complete
	emit(&lmproto.TransferSandboxResponse{
		SessionId:  sessionID,
		MessageId:  3,
		Status:     lmproto.TransferStatus_TRANSFER_STATUS_COMPLETE,
		StartTime:  timestamppb.New(start),
		UpdateTime: timestamppb.Now(),
		Progress:   1,
	})

	// Mark completed
	s.migState.seqMu.Lock()
	s.migState.completed = true
	s.migState.seqCond.Broadcast()
	s.migState.seqMu.Unlock()

	s.migState.migrated = migrated
}

func (s *service) FinalizeSandbox(ctx context.Context, req *lmproto.FinalizeSandboxRequest) (*lmproto.FinalizeSandboxResponse, error) {
	if s.migState.migrated == nil {
		return nil, fmt.Errorf("no migrated sandbox is present")
	}
	switch req.Action {
	case lmproto.FinalizeAction_RESUME:
		sandbox, err := s.migState.migrated.LMComplete(ctx)
		if err != nil {
			return nil, err
		}
		waitCtx, waitCancel := context.WithCancel(context.Background())
		s.sandbox = &Sandbox{
			State: &State{
				TaskID: s.migState.newID,
				waitCh: make(chan struct{}),
			},
			Sandbox:    sandbox,
			Tasks:      make(map[string]*Task),
			waitCtx:    waitCtx,
			waitCancel: waitCancel,
		}
		go waitContainer(s.sandbox.waitCtx, s.sandbox.Sandbox, s.sandbox.State, s.publisher)
	case lmproto.FinalizeAction_STOP:
		// Cancel waitContainer goroutines before killing the VM. Without this,
		// LMKill tears down the HCS system, the waitContainer goroutines detect
		// the VM exit and race to shut down the shim. By the time containerd
		// calls Kill/Delete (via StopPodSandbox), the shim's ttrpc pipe is
		// dead, causing "ttrpc: closed" errors that surface as
		// StopSourceVMFailure.
		s.sandbox.waitCancel()

		if err := s.migState.migrated.LMKill(ctx); err != nil {
			return nil, err
		}
		for _, t := range s.sandbox.Tasks {
			t.setExited(255)
			close(t.waitCh)
			if err := s.publisher.PublishEvent(ctx, runtime.TaskExitEventTopic, &events.TaskExit{
				ContainerID: t.TaskID,
				ID:          t.ExecID,
				Pid:         t.Pid,
				ExitStatus:  t.ExitStatus,
				ExitedAt:    timestamppb.New(t.ExitedAt),
			}); err != nil {
				log.G(ctx).WithError(err).Info("PublishEvent failed")
			}
		}
		s.sandbox.setExited(255)
		close(s.sandbox.waitCh)
		if err := s.publisher.PublishEvent(ctx, runtime.TaskExitEventTopic, &events.TaskExit{
			ContainerID: s.sandbox.TaskID,
			Pid:         s.sandbox.Pid,
			ExitStatus:  s.sandbox.ExitStatus,
			ExitedAt:    timestamppb.New(s.sandbox.ExitedAt),
		}); err != nil {
			log.G(ctx).WithError(err).Info("PublishEvent failed")
		}
		// Do not nil s.sandbox — containerd will call Kill and Delete on the
		// task ttrpc service after this returns. Those handlers need s.sandbox
		// to be present. The sandbox is already marked as exited, so Kill will
		// call Terminate which returns nil for an already-stopped system, and
		// Delete will return the cached exit state.
		s.migState = nil
	default:
		return nil, fmt.Errorf("unsupported action: %v", req.Action)
	}
	return &lmproto.FinalizeSandboxResponse{
		SessionId: req.SessionId,
	}, nil
}

func (s *service) Cancel(ctx context.Context, req *lmproto.CancelRequest) (*lmproto.CancelResponse, error) {
	if s.migState.l != 0 {
		windows.Closesocket(s.migState.l)
	}
	if s.migState.c != 0 {
		windows.Closesocket(s.migState.c)
	}
	s.migState = nil
	return &lmproto.CancelResponse{}, nil
}

func (s *service) newRestoreContainer(ctx context.Context, shimOpts *runhcsopts.Options, req *task.CreateTaskRequest) (err error) {
	spec, err := getRestoreContainerSpec(ctx, req.Bundle)
	if err != nil {
		return err
	}

	io, err := cmd.NewUpstreamIO(ctx, req.ID, req.Stdout, req.Stderr, req.Stdin, req.Terminal, 0)
	if err != nil {
		return err
	}

	taskState, ok := s.migState.taskState[spec.OriginalId]
	if !ok {
		return fmt.Errorf("cannot restore unknown container: %s", spec.OriginalId)
	}
	if req.Terminal != taskState.Terminal {
		return fmt.Errorf("terminal setting must match original container")
	}

	ctr, err := s.sandbox.Sandbox.(core.Migratable).RestoreLinuxContainer(ctx, spec.OriginalId, taskState.Pid, io)
	if err != nil {
		return err
	}
	t := &Task{
		State: restoredTaskState(req, taskState.Pid),
		Ctr:   ctr,
		Execs: make(map[string]*Exec),
	}
	s.sandbox.Tasks[req.ID] = t

	// TODO: Don't assume it's already started here.
	go waitContainer(s.sandbox.waitCtx, ctr, t.State, s.publisher)

	return nil
}

func getRestoreContainerSpec(ctx context.Context, bundle string) (*lmproto.ContainerRestoreSpec, error) {
	rawSpec, err := os.ReadFile(filepath.Join(bundle, "config.json"))
	if err != nil {
		return nil, err
	}
	var spec lmproto.ContainerRestoreSpec
	if err := (proto.UnmarshalOptions{}).Unmarshal(rawSpec, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

var wsasocket = windows.WSASocket
var getsockopt = windows.Getsockopt

func (s *service) CreateDuplicateSocket(ctx context.Context, req *lmproto.CreateDuplicateSocketRequest) (*lmproto.CreateDuplicateSocketResponse, error) {
	if s.migState.c != 0 {
		logrus.Warn("duplicate socket already exists")

		return &lmproto.CreateDuplicateSocketResponse{
			SessionId: req.SessionId,
		}, nil
	}
	if req.ProtocolInfo == nil {
		logrus.Error("no protocol info provided")
		return nil, fmt.Errorf("no protocol info provided")
	}
	var info windows.WSAProtocolInfo
	bytesRead := len(req.ProtocolInfo)
	if bytesRead < int(binary.Size(info)) {
		logrus.Error("protocol info too short")
		return nil, fmt.Errorf("protocol info too short, expected at least %d bytes, got %d", binary.Size(info), bytesRead)
	}
	reader := bytes.NewReader(req.ProtocolInfo)
	err := binary.Read(reader, binary.LittleEndian, &info)
	if err != nil {
		logrus.WithError(err).Error("error deserializing WSAProtocolInfo")
		return nil, fmt.Errorf("error deserializing WSAProtocolInfo: %w", err)
	}
	logrus.Info("WSAProtocolInfo deserialized successfully")
	logrus.Infof("read %d bytes from named pipe\n", bytesRead)
	logrus.Info("WSAProtocolInfo:", info)
	var wsaData windows.WSAData
	err = windows.WSAStartup(uint32(0x0202), &wsaData)
	if err != nil {
		logrus.WithError(err).Error("WSAStartup failed")
		return nil, fmt.Errorf("WSAStartup failed: %w", err)
	}
	defer windows.WSACleanup()
	newSock, err := wsasocket(info.AddressFamily, info.SocketType, info.Protocol, &info, 0, 0)
	if err != nil {
		logrus.WithError(err).Error("WSASocket failed")
		return nil, fmt.Errorf("WSASocket failed: %w", err)
	}
	logrus.Infof("new socket created successfully with handle: %v", newSock)
	var connectTime uint32
	optLen := int32(4)
	err = getsockopt(newSock, windows.SOL_SOCKET, 0x700C, (*byte)(unsafe.Pointer(&connectTime)), &optLen)
	if err != nil {
		logrus.WithError(err).Error("getsockopt SO_CONNECT_TIME failed")
		windows.Closesocket(newSock)
		return nil, fmt.Errorf("getsockopt SO_CONNECT_TIME failed: %w", err)
	}
	if connectTime == 0xFFFFFFFF {
		logrus.Error("Socket is not connected")
		windows.Closesocket(newSock)
		return nil, fmt.Errorf("duplicated socket is not connected")
	}
	s.m.Lock()
	defer s.m.Unlock()

	if s.mCond == nil {
		s.mCond = sync.NewCond(&s.m)
	}
	s.migState.c = newSock
	logrus.Infof("new socket handle set in migration state: %v", s.migState.c)
	s.mCond.Broadcast()

	return &lmproto.CreateDuplicateSocketResponse{
		SessionId: req.SessionId,
	}, nil
}

func enableChecksumValidationRegKeys() error {
    k, _, err := registry.CreateKey(
        registry.LOCAL_MACHINE,
        `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Virtualization\Migration`,
        registry.SET_VALUE,
    )
    if err != nil {
        return fmt.Errorf("open migration registry key: %w", err)
    }
    defer k.Close()

    if err := k.SetDWordValue("Test_UseSkippedForProtectionBitmapsInCrcCheck", 0); err != nil {
        return fmt.Errorf("set Test_UseSkippedForProtectionBitmapsInCrcCheck: %w", err)
    }
    if err := k.SetDWordValue("Test_TransferMemoryAfterVdevPowerOff", 1); err != nil {
        return fmt.Errorf("set Test_TransferMemoryAfterVdevPowerOff: %w", err)
    }
    return nil
}
