//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/shim"

	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/errdefs"
	"github.com/containerd/typeurl/v2"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

const (
	// addrFmt is the format of the address used for containerd shim.
	addrFmt = "\\\\.\\pipe\\ProtectedPrefix\\Administrators\\containerd-shim-%s-%s-pipe"
)

// shimManager implements the shim.Manager interface. It is the entry-point
// used by the containerd shim runner to create and destroy shim instances.
type shimManager struct {
	name string
}

// Verify that shimManager implements shim.Manager interface
var _ shim.Manager = (*shimManager)(nil)

// newShimManager returns a shimManager with the given binary name.
func newShimManager(name string) *shimManager {
	return &shimManager{
		name: name,
	}
}

// newCommand builds the exec.Cmd that will be used to spawn the long-running
// "serve" child process.
func newCommand(ctx context.Context,
	id,
	containerdAddress,
	socketAddr string,
	stderr io.Writer,
) (*exec.Cmd, error) {
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
	// Limit Go runtime parallelism in the child to avoid excessive CPU usage.
	cmd.Env = append(os.Environ(), "GOMAXPROCS=4")
	// Place the child in its own process group so OS signals (e.g. Ctrl-C)
	// sent to the parent are not automatically forwarded to the child.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr
	cmd.Stderr = stderr

	return cmd, nil
}

// Name returns the name of the shim
func (m *shimManager) Name() string {
	return m.name
}

// Start starts a shim instance for 'containerd-shim-lcow-v1'.
// This shim relies on containerd's Sandbox API to start a sandbox.
// There can be following scenarios that will launch a shim-
//
// 1. Containerd Sandbox Controller calls the Start command to start
// the sandbox for the pod. All the container create requests will
// set the SandboxID via `WithSandbox` ContainerOpts. Thereby, the
// container create request within the pod will be routed directly to the
// shim without calling the start command again.
//
// NOTE: This shim will not support routing the create request to an existing
// shim based on annotations like `io.kubernetes.cri.sandbox-id`.
func (m *shimManager) Start(ctx context.Context, id string, opts shim.StartOpts) (_ shim.BootstrapParams, retErr error) {
	// We cant write anything to stdout/stderr for this cmd.
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

	// Create an event on which we will listen to know when the shim is ready to accept connections.
	// The child serve process signals this event once its TTRPC server is fully initialized.
	eventName, _ := windows.UTF16PtrFromString(fmt.Sprintf("%s-%s", ns, id))

	// Create the named event
	handle, err := windows.CreateEvent(nil, 0, 0, eventName)
	if err != nil {
		log.Fatalf("Failed to create event: %v", err)
	}
	defer windows.CloseHandle(handle)

	// address is the named pipe address that the shim will use to serve the ttrpc service.
	address := fmt.Sprintf(addrFmt, ns, id)

	// Create the serve command.
	cmd, err := newCommand(ctx, id, opts.Address, address, f)
	if err != nil {
		return params, err
	}

	if err = cmd.Start(); err != nil {
		return params, err
	}

	defer func() {
		if retErr != nil {
			cmd.Process.Kill()
		}
	}()

	// Block until the child signals the event.
	_, _ = windows.WaitForSingleObject(handle, windows.INFINITE)

	params.Address = address
	return params, nil
}

// Stop tears down a running shim instance identified by id.
// It reads and logs any panic messages written to panic.log, then tries to
// terminate the associated HCS compute system and waits up to 30 seconds for
// it to exit.
func (m *shimManager) Stop(ctx context.Context, id string) (resp shim.StopStatus, err error) {
	ctx, span := oc.StartSpan(context.Background(), "delete")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var bundlePath string
	if opts, ok := ctx.Value(shim.OptsKey{}).(shim.Opts); ok {
		bundlePath = opts.BundlePath
	}

	if bundlePath == "" {
		return resp, fmt.Errorf("bundle path not found in context")
	}

	// hcsshim shim writes panic logs in the bundle directory in a file named "panic.log"
	// log those messages (if any) on stderr so that it shows up in containerd's log.
	// This should be done as the first thing so that we don't miss any panic logs even if
	// something goes wrong during delete op.
	// The file can be very large so read only first 1MB of data.
	readLimit := int64(memory.MiB) // 1MB
	logBytes, err := limitedRead(filepath.Join(bundlePath, "panic.log"), readLimit)
	if err == nil && len(logBytes) > 0 {
		if int64(len(logBytes)) == readLimit {
			logrus.Warnf("shim panic log file %s is larger than 1MB, logging only first 1MB", filepath.Join(bundlePath, "panic.log"))
		}
		logrus.WithField("log", string(logBytes)).Warn("found shim panic logs during delete")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		logrus.WithError(err).Warn("failed to open shim panic log")
	}

	// Attempt to find the hcssystem for this bundle and terminate it.
	if sys, _ := hcs.OpenComputeSystem(ctx, id); sys != nil {
		defer sys.Close()
		if err := sys.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "failed to terminate '%s': %v", id, err)
		} else {
			ch := make(chan error, 1)
			go func() { ch <- sys.Wait() }()
			t := time.NewTimer(time.Second * 30)
			select {
			case <-t.C:
				sys.Close()
				return resp, fmt.Errorf("timed out waiting for '%s' to terminate", id)
			case err := <-ch:
				t.Stop()
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to wait for '%s' to terminate: %v", id, err)
				}
			}
		}
	}

	resp = shim.StopStatus{
		ExitedAt:   time.Now(),
		ExitStatus: 255,
	}
	return resp, nil
}

// limitedRead reads at max `readLimitBytes` bytes from the file at path `filePath`. If the file has
// more than `readLimitBytes` bytes of data then first `readLimitBytes` will be returned.
// Read at most readLimitBytes so delete does not flood logs.
func limitedRead(filePath string, readLimitBytes int64) ([]byte, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("limited read failed to open file: %s: %w", filePath, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return []byte{}, fmt.Errorf("limited read failed during file stat: %s: %w", filePath, err)
	}
	if fi.Size() < readLimitBytes {
		readLimitBytes = fi.Size()
	}
	buf := make([]byte, readLimitBytes)
	_, err = f.Read(buf)
	if err != nil {
		return []byte{}, fmt.Errorf("limited read failed during file read: %s: %w", filePath, err)
	}
	return buf, nil
}

// Info returns runtime information about this shim including its name, version,
// git commit, OCI spec version, and any runtime options decoded from optionsR.
func (m *shimManager) Info(ctx context.Context, optionsR io.Reader) (*types.RuntimeInfo, error) {
	var v []string
	if version != "" {
		v = append(v, version)
	}
	if gitCommit != "" {
		v = append(v, fmt.Sprintf("commit: %s", gitCommit))
	}
	v = append(v, fmt.Sprintf("spec: %s", specs.Version))

	info := &types.RuntimeInfo{
		Name: m.name,
		Version: &types.RuntimeVersion{
			Version: strings.Join(v, "\n"),
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
