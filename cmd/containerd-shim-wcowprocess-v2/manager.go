//go:build windows && wcowprocess

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	hcs "github.com/Microsoft/hcsshim/internal/hcs/v2"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/shim"
	hcsversion "github.com/Microsoft/hcsshim/internal/version"

	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/errdefs"
	"github.com/containerd/typeurl/v2"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

const (
	addrFmt = "\\\\.\\pipe\\ProtectedPrefix\\Administrators\\containerd-shim-%s-%s-pipe"

	// serveReadyEventNameFormat is the named Windows event ("<ns>-<id>") the child
	// "serve" process signals once its ttrpc server is ready.
	serveReadyEventNameFormat = "%s-%s"
)

// shimManager implements shim.Manager: containerd's entry-point to create and destroy shim instances.
type shimManager struct {
	name string
}

var _ shim.Manager = (*shimManager)(nil)

func newShimManager(name string) *shimManager {
	return &shimManager{name: name}
}

// newCommand builds the exec.Cmd used to spawn the long-running "serve" child process.
func newCommand(ctx context.Context, id, containerdAddress, socketAddr string, stderr io.Writer) (*exec.Cmd, error) {
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	args := []string{
		"-namespace", ns,
		"-id", id,
		"-address", containerdAddress,
		"-socket", socketAddr,
		"serve",
	}
	cmd := exec.Command(self, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GOMAXPROCS=4")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr
	cmd.Stderr = stderr

	return cmd, nil
}

func (m *shimManager) Name() string {
	return m.name
}

// Start starts a shim instance. The shim relies on containerd's Sandbox API; container create
// requests within the pod are routed to this shim via the sandbox id.
func (m *shimManager) Start(ctx context.Context, id string, opts shim.StartOpts) (_ shim.BootstrapParams, retErr error) {
	logrus.SetOutput(io.Discard)

	var params shim.BootstrapParams
	params.Version = 3
	params.Protocol = "ttrpc"

	cwd, err := os.Getwd()
	if err != nil {
		return params, fmt.Errorf("failed to get current working directory: %w", err)
	}

	f, err := os.Create(filepath.Join(cwd, "panic.log"))
	if err != nil {
		return params, fmt.Errorf("failed to create panic log file: %w", err)
	}
	defer f.Close()

	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return params, fmt.Errorf("failed to get namespace from context: %w", err)
	}

	// Named event the child signals once it is ready to accept connections.
	eventName, _ := windows.UTF16PtrFromString(fmt.Sprintf(serveReadyEventNameFormat, ns, id))
	handle, err := windows.CreateEvent(nil, 0, 0, eventName)
	if err != nil {
		return params, fmt.Errorf("failed to create event: %w", err)
	}
	defer func() {
		_ = windows.CloseHandle(handle)
	}()

	address := fmt.Sprintf(addrFmt, ns, id)

	cmd, err := newCommand(ctx, id, opts.Address, address, f)
	if err != nil {
		return params, err
	}

	if err = cmd.Start(); err != nil {
		return params, err
	}

	defer func() {
		if retErr != nil {
			_ = cmd.Process.Kill()
		}
	}()

	// Block until the child signals readiness.
	_, _ = windows.WaitForSingleObject(handle, windows.INFINITE)

	params.Address = address
	return params, nil
}

// Stop tears down a running shim instance. It logs any panic.log messages and best-effort
// terminates the HCS compute system for the pod's sandbox container (which shares this id).
func (m *shimManager) Stop(ctx context.Context, id string) (resp shim.StopStatus, err error) {
	ctx, span := oc.StartSpan(ctx, "delete")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var bundlePath string
	if opts, ok := ctx.Value(shim.OptsKey{}).(shim.Opts); ok {
		bundlePath = opts.BundlePath
	}
	if bundlePath == "" {
		return resp, fmt.Errorf("bundle path not found in context")
	}

	// Log any shim panic messages (first 1MB) so they show up in containerd's log.
	readLimit := int64(memory.MiB)
	logBytes, err := limitedRead(filepath.Join(bundlePath, "panic.log"), readLimit)
	if err == nil && len(logBytes) > 0 {
		if int64(len(logBytes)) == readLimit {
			logrus.Warnf("shim panic log file %s is larger than 1MB, logging only first 1MB", filepath.Join(bundlePath, "panic.log"))
		}
		logrus.WithField("log", string(logBytes)).Warn("found shim panic logs during delete")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		logrus.WithError(err).Warn("failed to open shim panic log")
	}

	if sys, _ := hcs.OpenComputeSystem(ctx, id); sys != nil {
		defer sys.Close()
		if err := sys.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "failed to terminate %q: %v", id, err)
		} else {
			ch := make(chan error, 1)
			go func() { ch <- sys.Wait() }()
			t := time.NewTimer(time.Second * 30)
			select {
			case <-t.C:
				sys.Close()
				return resp, fmt.Errorf("timed out waiting for %q to terminate", id)
			case err := <-ch:
				t.Stop()
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to wait for %q to terminate: %v", id, err)
				}
			}
		}
	}

	resp = shim.StopStatus{
		ExitedAt: time.Now(),
		// 255 exit code is used by convention to indicate unknown exit reason.
		ExitStatus: 255,
	}
	return resp, nil
}

// limitedRead reads at most readLimitBytes from the file at filePath.
func limitedRead(filePath string, readLimitBytes int64) ([]byte, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", filePath, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return []byte{}, fmt.Errorf("stat file %s: %w", filePath, err)
	}
	if fi.Size() < readLimitBytes {
		readLimitBytes = fi.Size()
	}
	buf := make([]byte, readLimitBytes)
	_, err = f.Read(buf)
	if err != nil {
		return []byte{}, fmt.Errorf("read file %s: %w", filePath, err)
	}
	return buf, nil
}

// Info returns runtime information about this shim.
func (m *shimManager) Info(_ context.Context, optionsR io.Reader) (*types.RuntimeInfo, error) {
	info := &types.RuntimeInfo{
		Name: m.name,
		Version: &types.RuntimeVersion{
			Version: fmt.Sprintf("%s\ncommit: %s\nspec: %s", hcsversion.Version, hcsversion.Commit, specs.Version),
		},
		Annotations: nil,
	}

	opts, err := shim.ReadRuntimeOptions[*runhcsopts.Options](optionsR)
	if err != nil {
		if !errors.Is(err, errdefs.ErrNotFound) {
			return nil, fmt.Errorf("failed to read runtime options (*options.Options): %w", err)
		}
	}
	if opts != nil {
		info.Options, err = typeurl.MarshalAnyToProto(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal %T: %w", opts, err)
		}
	}

	return info, nil
}
