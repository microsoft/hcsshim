//go:build windows && functional

package functional

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	ctrdoci "github.com/containerd/containerd/oci"
	"github.com/google/go-cmp/cmp"
	"github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/windows"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/sync"
	"github.com/Microsoft/hcsshim/osversion"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	testcontainer "github.com/Microsoft/hcsshim/test/internal/container"
	"github.com/Microsoft/hcsshim/test/internal/layers"
	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

// Hyper-V socket tests based on pipe tests in golang source. Ie, they re-exec the testing binary
// from within the uVM/container to run the other portion of the tests.
// Otherwise, a dedicated binary would be needed for these tests to run.
//
// See:
// https://cs.opensource.google/go/go/+/master:src/os/pipe_test.go;l=266-273;drc=0dfb22ed70749a2cd6d95ec6eee63bb213a940d4

// Since these test run on Windows, the tests (which exec-ing the testing binary from inside the guest)
// only work for WCOW.
// This is fine since Linux guests can only dial out over vsock.

const (
	// total timeout for an hvsock test.
	hvsockTestTimeout = 10 * time.Second
	// how long to wait when dialing over hvsock.
	hvsockDialTimeout = 3 * time.Second
	// how long to wait when accepting new hvsock connections.
	hvsockAcceptTimeout = 3 * time.Second
)

//
// uVM tests
//

func TestHVSock_UVM_HostBind(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureUVM, featureHVSocket)

	ctx := util.Context(context.Background(), t)

	for _, tc := range hvsockHostBindTestCases {
		t.Run(tc.name, func(t *testing.T) {
			msg1 := "hello from " + t.Name()
			msg2 := "echo from " + t.Name()
			svcGUID := getHVSockServiceGUID(t)

			// guest code

			util.RunInReExec(ctx, t, hostBindReExecFunc(svcGUID, tc.guestDialErr, msg1, msg2))

			// host code

			vm := testuvm.CreateAndStart(ctx, t, defaultWCOWOptions(ctx, t))

			if tc.hvsockConfig != nil {
				t.Logf("updating hvsock config for service %v: %#+v", svcGUID, tc.hvsockConfig)
				if err := vm.UpdateHvSocketService(ctx, svcGUID.String(), tc.hvsockConfig); err != nil {
					t.Fatalf("could not update service config: %v", err)
				}
			}

			// create deadline for rest of the test (excluding the uVM creation)
			ctx, cancel := context.WithTimeout(ctx, hvsockTestTimeout) //nolint:govet // ctx is shadowed
			t.Cleanup(cancel)

			// bind to guest (or wildcard) address
			id := vm.RuntimeID()
			t.Logf("guest uVM runtime ID: %v", id)
			if tc.useWildcard {
				id = tc.wildcard
			}
			addr := &winio.HvsockAddr{
				VMID:      id,
				ServiceID: svcGUID,
			}

			t.Logf("listening to hvsock address: %v", addr)
			l, err := winio.ListenHvsock(addr)
			if !errors.Is(err, tc.hostListenErr) {
				t.Fatalf("expected listen error %v; got: %v", tc.hostListenErr, err)
			}
			if err != nil {
				// expected an error, cant do much else
				return
			}

			t.Cleanup(func() {
				if err := l.Close(); err != nil {
					t.Errorf("could not close listener on address %v: %v", addr, err)
				}
			})

			var hostConn net.Conn
			acceptErrCh := goBlockT(func() (err error) {
				// don't want to call t.Error here, the error could be due to the hv socket lister
				// being closed after timeing out
				hostConn, err = l.Accept()
				if err != nil {
					t.Logf("accept failed: %v", err)
				} else {
					t.Cleanup(func() { hostConn.Close() })
				}
				return err
			})

			guestPath := filepath.Join(`C:\`, filepath.Base(os.Args[0]))
			testuvm.Share(ctx, t, vm, os.Args[0], guestPath, true)

			reexecCmd := fmt.Sprintf(`%s -test.run=%s`, guestPath, util.TestNameRegex(t))
			if testing.Verbose() {
				reexecCmd += " -test.v"
			}

			ps := testoci.CreateWindowsSpec(ctx, t, vm.ID(),
				testoci.DefaultWindowsSpecOpts(vm.ID(),
					ctrdoci.WithUsername(`NT AUTHORITY\SYSTEM`),
					ctrdoci.WithEnv([]string{util.ReExecEnv + "=1"}),
					ctrdoci.WithProcessCommandLine(reexecCmd),
				)...).Process

			cmdIO := testcmd.NewBufferedIO()
			c := testcmd.Create(ctx, t, vm, ps, cmdIO)

			testcmd.Start(ctx, t, c)
			t.Cleanup(func() {
				testcmd.WaitExitCode(ctx, t, c, 0)

				s, _ := cmdIO.Output()
				t.Logf("guest exec:\n%s", s)
			})

			select {
			case <-time.After(hvsockAcceptTimeout):
				if tc.guestDialErr != nil {
					// expected guest to error while dialing hv socket connection
					t.Logf("timed out waiting for guest to connect")
					return
				}
				t.Fatalf("timed out waiting for hvsock connection")
			case err := <-acceptErrCh:
				if err != nil {
					t.Fatalf("accept failed: %v", err)
				}
			}

			t.Logf("accepted connection: %v->%v", hostConn.LocalAddr(), hostConn.RemoteAddr())
			verifyLocalHvSockConn(ctx, t, hostConn, winio.HvsockAddr{
				ServiceID: svcGUID,
				VMID:      vm.RuntimeID(),
			})

			got := readConn(ctx, t, hostConn)
			if got != msg1 {
				t.Fatalf("got %q, wanted %q", got, msg1)
			}

			writeConn(ctx, t, hostConn, []byte(msg2))

			if got2 := readConn(ctx, t, hostConn); got2 != "" {
				t.Logf("read was not empty: %s", got2)
			}
		})
	}
}

func TestHVSock_UVM_GuestBind(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureUVM, featureHVSocket)

	ctx := util.Context(context.Background(), t)

	for _, tc := range hvsockGuestBindTestCases {
		t.Run(tc.name, func(t *testing.T) {
			msg1 := "hello from " + t.Name()
			msg2 := "echo from " + t.Name()
			svcGUID := getHVSockServiceGUID(t)

			// guest code

			util.RunInReExec(ctx, t, guestBindReExecFunc(svcGUID, false, tc.guestListenErr, tc.hostDialErr != nil, msg1, msg2))

			// host code

			vm := testuvm.CreateAndStart(ctx, t, defaultWCOWOptions(ctx, t))

			if tc.hvsockConfig != nil {
				t.Logf("updating hvsock config for service %v: %#+v", svcGUID, tc.hvsockConfig)
				if err := vm.UpdateHvSocketService(ctx, svcGUID.String(), tc.hvsockConfig); err != nil {
					t.Fatalf("could not update service config: %v", err)
				}
			}

			// create deadline for rest of the test (excluding the uVM creation)
			ctx, cancel := context.WithTimeout(ctx, hvsockTestTimeout) //nolint:govet // ctx is shadowed
			t.Cleanup(cancel)

			guestPath := filepath.Join(`C:\`, filepath.Base(os.Args[0]))
			testuvm.Share(ctx, t, vm, os.Args[0], guestPath, true)

			reexecCmd := fmt.Sprintf(`%s -test.run=%s`, guestPath, util.TestNameRegex(t))
			if testing.Verbose() {
				reexecCmd += " -test.v"
			}

			ps := testoci.CreateWindowsSpec(ctx, t, vm.ID(),
				testoci.DefaultWindowsSpecOpts(vm.ID(),
					ctrdoci.WithUsername(`NT AUTHORITY\SYSTEM`),
					ctrdoci.WithEnv([]string{util.ReExecEnv + "=1"}),
					ctrdoci.WithProcessCommandLine(reexecCmd),
				)...).Process

			cmdIO := testcmd.NewBufferedIO()
			c := testcmd.Create(ctx, t, vm, ps, cmdIO)

			testcmd.Start(ctx, t, c)
			t.Cleanup(func() {
				testcmd.WaitExitCode(ctx, t, c, 0)

				s, _ := cmdIO.Output()
				t.Logf("guest exec:\n%s", s)
			})

			// bind to uVM (or wildcard) address
			id := vm.RuntimeID()
			t.Logf("guest uVM runtime ID: %v", id)
			if tc.useWildcard {
				id = tc.wildcard
			}
			addr := &winio.HvsockAddr{
				VMID:      id,
				ServiceID: svcGUID,
			}

			// wait a bit just to make sure exec in guest has had long enough to start and listen on hvsock
			time.Sleep(10 * time.Millisecond)

			dialCtx, dialCancel := context.WithTimeout(ctx, hvsockDialTimeout)
			t.Cleanup(dialCancel)

			t.Logf("dialing guest on: %v", addr)
			hostConn, err := winio.Dial(dialCtx, addr)
			if !errors.Is(err, tc.hostDialErr) {
				t.Fatalf("expected dial error %v; got: %v", tc.hostDialErr, err)
			}
			if err != nil {
				// expected an error, cant do much else
				t.Logf("dial failed: %v", err)
				return
			}

			t.Cleanup(func() {
				if err := hostConn.Close(); err != nil {
					t.Errorf("could not close connection on address %v: %v", hostConn.LocalAddr(), err)
				}
			})
			t.Logf("dialled connection: %v->%v", hostConn.LocalAddr(), hostConn.RemoteAddr())

			verifyRemoteHvSockConn(ctx, t, hostConn, winio.HvsockAddr{
				ServiceID: svcGUID,
				VMID:      vm.RuntimeID(),
			})

			writeConn(ctx, t, hostConn, []byte(msg1))

			got := readConn(ctx, t, hostConn)
			if got != msg2 {
				t.Fatalf("got %q, wanted %q", got, msg2)
			}
		})
	}
}

//
// container tests
//

// ! NOTE:
// (v2 Xenon) containers (currently) inherit uVM HyperV socket settings
// since the HCS document they are create with has an empty `HvSock` field.
// These tests will fail if that is no longer the case.
//
// See:
//  - internal\hcsoci.createWindowsContainerDocument
//  - internal\hcs\schema2.Container.HvSockets

func TestHVSock_Container_HostBind(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureUVM, featureContainer, featureHVSocket)

	ctx := util.Context(context.Background(), t)

	for _, tc := range hvsockHostBindTestCases {
		t.Run(tc.name, func(t *testing.T) {
			msg1 := "hello from " + t.Name()
			msg2 := "echo from " + t.Name()
			svcGUID := getHVSockServiceGUID(t)

			// guest code

			util.RunInReExec(ctx, t, hostBindReExecFunc(svcGUID, tc.guestDialErr, msg1, msg2))

			// host code

			vm := testuvm.CreateAndStart(ctx, t, defaultWCOWOptions(ctx, t))

			if tc.hvsockConfig != nil {
				t.Logf("updating hvsock config for service %v: %#+v", svcGUID, tc.hvsockConfig)
				if err := vm.UpdateHvSocketService(ctx, svcGUID.String(), tc.hvsockConfig); err != nil {
					t.Fatalf("could not update service config: %v", err)
				}
			}

			guestPath := filepath.Join(`C:\`, filepath.Base(os.Args[0]))
			reexecCmd := fmt.Sprintf(`%s -test.run=%s`, guestPath, util.TestNameRegex(t))
			if testing.Verbose() {
				reexecCmd += " -test.v"
			}

			cID := vm.ID() + "-container"
			scratch := layers.WCOWScratchDir(ctx, t, "")
			spec := testoci.CreateWindowsSpec(ctx, t, cID,
				testoci.DefaultWindowsSpecOpts(cID,
					testoci.WithWindowsLayerFolders(append(windowsImageLayers(ctx, t), scratch)),
					ctrdoci.WithUsername(`NT AUTHORITY\SYSTEM`),
					ctrdoci.WithEnv([]string{util.ReExecEnv + "=1"}),
					ctrdoci.WithProcessCommandLine(reexecCmd),
					ctrdoci.WithMounts([]specs.Mount{{
						Source:      os.Args[0],
						Destination: guestPath,
						Options:     []string{"ro"},
					}}),
				)...)

			ctr, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cID, hcsOwner)
			t.Cleanup(cleanup)

			// create deadline for rest of the test (excluding the uVM and container creation)
			ctx, cancel := context.WithTimeout(ctx, hvsockTestTimeout) //nolint:govet // ctx is shadowed
			t.Cleanup(cancel)

			// bind to guest (or wildcard) address
			id := vm.RuntimeID()
			t.Logf("guest uVM runtime ID: %v", id)
			if tc.useWildcard {
				id = tc.wildcard
			}
			addr := &winio.HvsockAddr{
				VMID:      id,
				ServiceID: svcGUID,
			}

			t.Logf("listening to hvsock address: %v", addr)
			l, err := winio.ListenHvsock(addr)
			if !errors.Is(err, tc.hostListenErr) {
				t.Fatalf("expected listen error %v; got: %v", tc.hostListenErr, err)
			}
			if err != nil {
				// expected an error, cant do much else
				return
			}

			t.Cleanup(func() {
				if err := l.Close(); err != nil {
					t.Errorf("could not close listener on address %v: %v", addr, err)
				}
			})

			var hostConn net.Conn
			acceptErrCh := goBlockT(func() (err error) {
				// don't want to call t.Error here, the error could be due to the hv socket lister
				// being closed after timeing out
				hostConn, err = l.Accept()
				if err != nil {
					t.Logf("accept failed: %v", err)
				} else {
					t.Cleanup(func() { hostConn.Close() })
				}
				return err
			})

			// start the container (and its init process)
			cmdIO := testcmd.NewBufferedIO()
			c := testcontainer.StartWithSpec(ctx, t, ctr, spec.Process, cmdIO)
			t.Cleanup(func() {
				testcmd.WaitExitCode(ctx, t, c, 0)

				s, _ := cmdIO.Output()
				t.Logf("guest exec:\n%s", s)

				testcontainer.Kill(ctx, t, ctr)
				testcontainer.Wait(ctx, t, ctr)
			})

			select {
			case <-time.After(hvsockAcceptTimeout):
				if tc.guestDialErr != nil {
					// expected guest to error while dialing hv socket connection
					t.Logf("timed out waiting for guest to connect")
					return
				}
				t.Fatalf("timed out waiting for hvsock connection")
			case err := <-acceptErrCh:
				if err != nil {
					t.Fatalf("accept failed: %v", err)
				}
			}

			t.Logf("accepted connection: %v->%v", hostConn.LocalAddr(), hostConn.RemoteAddr())
			verifyLocalHvSockConn(ctx, t, hostConn, winio.HvsockAddr{
				ServiceID: svcGUID,
				VMID:      vm.RuntimeID(),
			})

			got := readConn(ctx, t, hostConn)
			if got != msg1 {
				t.Fatalf("got %q, wanted %q", got, msg1)
			}

			writeConn(ctx, t, hostConn, []byte(msg2))

			if got2 := readConn(ctx, t, hostConn); got2 != "" {
				t.Logf("read was not empty: %s", got2)
			}
		})
	}
}

func TestHVSock_Container_GuestBind(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureUVM, featureContainer, featureHVSocket)

	ctx := util.Context(context.Background(), t)

	for _, tc := range hvsockGuestBindTestCases {
		t.Run(tc.name, func(t *testing.T) {
			msg1 := "hello from " + t.Name()
			msg2 := "echo from " + t.Name()
			svcGUID := getHVSockServiceGUID(t)

			// guest code

			util.RunInReExec(ctx, t, guestBindReExecFunc(svcGUID, true, tc.guestListenErr, tc.hostDialErr != nil, msg1, msg2))

			// host code

			vm := testuvm.CreateAndStart(ctx, t, defaultWCOWOptions(ctx, t))

			if tc.hvsockConfig != nil {
				t.Logf("updating hvsock config for service %v: %#+v", svcGUID, tc.hvsockConfig)
				if err := vm.UpdateHvSocketService(ctx, svcGUID.String(), tc.hvsockConfig); err != nil {
					t.Fatalf("could not update service config: %v", err)
				}
			}

			guestPath := filepath.Join(`C:\`, filepath.Base(os.Args[0]))
			reexecCmd := fmt.Sprintf(`%s -test.run=%s`, guestPath, util.TestNameRegex(t))
			if testing.Verbose() {
				reexecCmd += " -test.v"
			}

			cID := vm.ID() + "-container"
			scratch := layers.WCOWScratchDir(ctx, t, "")
			spec := testoci.CreateWindowsSpec(ctx, t, cID,
				testoci.DefaultWindowsSpecOpts(cID,
					testoci.WithWindowsLayerFolders(append(windowsImageLayers(ctx, t), scratch)),
					ctrdoci.WithUsername(`NT AUTHORITY\SYSTEM`),
					ctrdoci.WithEnv([]string{util.ReExecEnv + "=1"}),
					ctrdoci.WithProcessCommandLine(reexecCmd),
					ctrdoci.WithMounts([]specs.Mount{{
						Source:      os.Args[0],
						Destination: guestPath,
						Options:     []string{"ro"},
					}}),
				)...)

			ctr, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cID, hcsOwner)
			t.Cleanup(cleanup)

			// create deadline for rest of the test (excluding the uVM and container creation)
			ctx, cancel := context.WithTimeout(ctx, hvsockTestTimeout) //nolint:govet // ctx is shadowed
			t.Cleanup(cancel)

			// start the container (and its init process)
			cmdIO := testcmd.NewBufferedIO()
			c := testcontainer.StartWithSpec(ctx, t, ctr, spec.Process, cmdIO)

			t.Cleanup(func() {
				testcmd.WaitExitCode(ctx, t, c, 0)

				s, _ := cmdIO.Output()
				t.Logf("guest exec:\n%s", s)

				testcontainer.Kill(ctx, t, ctr)
				testcontainer.Wait(ctx, t, ctr)
			})

			// bind to uVM (or wildcard) address
			id := vm.RuntimeID()
			t.Logf("guest uVM runtime ID: %v", id)
			if tc.useWildcard {
				id = tc.wildcard
			}
			addr := &winio.HvsockAddr{
				VMID:      id,
				ServiceID: svcGUID,
			}

			// wait a bit just to make sure exec in guest has had long enough to start and listen on hvsock
			time.Sleep(10 * time.Millisecond)

			dialCtx, dialCancel := context.WithTimeout(ctx, hvsockDialTimeout)
			t.Cleanup(dialCancel)

			t.Logf("dialing guest on: %v", addr)
			hostConn, err := winio.Dial(dialCtx, addr)
			if !errors.Is(err, tc.hostDialErr) {
				t.Fatalf("expected dial error %v; got: %v", tc.hostDialErr, err)
			}
			if err != nil {
				// expected an error, cant do much else
				t.Logf("dial failed: %v", err)
				return
			}

			t.Cleanup(func() {
				if err := hostConn.Close(); err != nil {
					t.Errorf("could not close connection on address %v: %v", hostConn.LocalAddr(), err)
				}
			})
			t.Logf("dialled connection: %v->%v", hostConn.LocalAddr(), hostConn.RemoteAddr())

			verifyRemoteHvSockConn(ctx, t, hostConn, winio.HvsockAddr{
				ServiceID: svcGUID,
				VMID:      vm.RuntimeID(),
			})

			writeConn(ctx, t, hostConn, []byte(msg1))

			got := readConn(ctx, t, hostConn)
			if got != msg2 {
				t.Fatalf("got %q, wanted %q", got, msg2)
			}
		})
	}
}

//
// test cases
//

// Common SDDL strings.
const (
	allowElevatedSDDL = "D:P(A;;FA;;;SY)(A;;FA;;;BA)"
	denyAllSDDL       = "D:P(D;;FA;;;WD)"
)

var hvsockHostBindTestCases = []struct {
	name string

	useWildcard bool
	wildcard    guid.GUID

	hvsockConfig *hcsschema.HvSocketServiceConfig

	hostListenErr error
	guestDialErr  error
}{
	//
	// defaults
	//

	{
		name: "default",
	},
	{
		name:         "wildcard default",
		useWildcard:  true,
		wildcard:     winio.HvsockGUIDWildcard(),
		guestDialErr: windows.WSAECONNREFUSED,
	},
	{
		name:         "wildcard children default",
		useWildcard:  true,
		wildcard:     winio.HvsockGUIDChildren(),
		guestDialErr: windows.WSAECONNREFUSED,
	},

	//
	// connection allowed
	//

	{
		name: "vm id allowed",
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    allowElevatedSDDL,
			ConnectSecurityDescriptor: denyAllSDDL,
			AllowWildcardBinds:        false,
		},
	},
	{
		name:        "wildcard allowed",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDWildcard(),
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    allowElevatedSDDL,
			ConnectSecurityDescriptor: denyAllSDDL,
			AllowWildcardBinds:        true,
		},
	},
	{
		name:        "wildcard children allowed",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDChildren(),
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    allowElevatedSDDL,
			ConnectSecurityDescriptor: denyAllSDDL,
			AllowWildcardBinds:        true,
		},
	},

	//
	// connection denied
	//

	{
		name: "vm id denied",
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    denyAllSDDL,
			ConnectSecurityDescriptor: denyAllSDDL,
			AllowWildcardBinds:        false,
		},
		hostListenErr: windows.WSAEACCES,
	},
	{
		name:        "wildcard denied",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDWildcard(),
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    allowElevatedSDDL,
			ConnectSecurityDescriptor: denyAllSDDL,
			AllowWildcardBinds:        false,
		},
		guestDialErr: windows.WSAECONNREFUSED,
	},
	{
		name:        "wildcard children denied",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDChildren(),
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    allowElevatedSDDL,
			ConnectSecurityDescriptor: denyAllSDDL,
			AllowWildcardBinds:        false,
		},
		guestDialErr: windows.WSAECONNREFUSED,
	},

	//
	// connection disabled
	//

	{
		name: "vm id disabled",
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    allowElevatedSDDL,
			ConnectSecurityDescriptor: denyAllSDDL,
			Disabled:                  true,
		},

		// windows.WSAEINVAL, returned from windows.Listen, implies that the socket was not bound prior.
		//
		// See:
		// https://learn.microsoft.com/en-us/windows/win32/api/winsock2/nf-winsock2-listen
		hostListenErr: windows.WSAEINVAL,
	},
	{
		name:        "wildcard disabled",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDWildcard(),
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    allowElevatedSDDL,
			ConnectSecurityDescriptor: denyAllSDDL,
			AllowWildcardBinds:        true,
			Disabled:                  true,
		},
		guestDialErr: windows.WSAECONNREFUSED,
	},
	{
		name:        "wildcard children disabled",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDChildren(),
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    allowElevatedSDDL,
			ConnectSecurityDescriptor: denyAllSDDL,
			AllowWildcardBinds:        true,
			Disabled:                  true,
		},
		guestDialErr: windows.WSAECONNREFUSED,
	},
}

var hvsockGuestBindTestCases = []struct {
	name string

	useWildcard bool
	wildcard    guid.GUID

	hvsockConfig *hcsschema.HvSocketServiceConfig

	hostDialErr    error
	guestListenErr error
}{
	//
	// defaults
	//

	{
		name: "default",
	},
	{
		name:        "wildcard default",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDWildcard(),
		hostDialErr: windows.WSAEADDRNOTAVAIL,
	},
	{
		name:        "wildcard children default",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDChildren(),
		hostDialErr: windows.WSAEADDRNOTAVAIL,
	},

	//
	// connection allowed
	//

	{
		name: "vm id allowed",
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    denyAllSDDL,
			ConnectSecurityDescriptor: allowElevatedSDDL,
			AllowWildcardBinds:        false,
		},
	},
	{
		name:        "wildcard allowed",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDWildcard(),
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    denyAllSDDL,
			ConnectSecurityDescriptor: allowElevatedSDDL,
			AllowWildcardBinds:        false,
		},
		hostDialErr: windows.WSAEADDRNOTAVAIL,
	},
	{
		name:        "wildcard children allowed",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDChildren(),
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    denyAllSDDL,
			ConnectSecurityDescriptor: allowElevatedSDDL,
			AllowWildcardBinds:        false,
		},
		hostDialErr: windows.WSAEADDRNOTAVAIL,
	},

	//
	// connection denied
	//

	{
		name: "vm id denied",
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    denyAllSDDL,
			ConnectSecurityDescriptor: denyAllSDDL,
			AllowWildcardBinds:        false,
		},
		hostDialErr: windows.WSAEACCES,
	},
	{
		name:        "wildcard denied",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDWildcard(),
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    denyAllSDDL,
			ConnectSecurityDescriptor: denyAllSDDL,
			AllowWildcardBinds:        false,
		},
		hostDialErr: windows.WSAEADDRNOTAVAIL,
	},
	{
		name:        "wildcard children denied",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDChildren(),
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    denyAllSDDL,
			ConnectSecurityDescriptor: denyAllSDDL,
			AllowWildcardBinds:        false,
		},
		hostDialErr: windows.WSAEADDRNOTAVAIL,
	},

	//
	// connection disabled
	//

	{
		name: "vm id disabled",
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    denyAllSDDL,
			ConnectSecurityDescriptor: allowElevatedSDDL,
			AllowWildcardBinds:        false,
			Disabled:                  true,
		},

		hostDialErr: windows.WSAEINVAL,
	},
	{
		name:        "wildcard disabled",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDWildcard(),
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    denyAllSDDL,
			ConnectSecurityDescriptor: allowElevatedSDDL,
			AllowWildcardBinds:        false,
			Disabled:                  true,
		},
		hostDialErr: windows.WSAEADDRNOTAVAIL,
	},
	{
		name:        "wildcard children disabled",
		useWildcard: true,
		wildcard:    winio.HvsockGUIDChildren(),
		hvsockConfig: &hcsschema.HvSocketServiceConfig{
			BindSecurityDescriptor:    denyAllSDDL,
			ConnectSecurityDescriptor: allowElevatedSDDL,
			AllowWildcardBinds:        false,
			Disabled:                  true,
		},
		hostDialErr: windows.WSAEADDRNOTAVAIL,
	},
}

//
// functions to be run from within guest
//

// code to run from guest when host is binding to hyper-v socket address.
func hostBindReExecFunc(svcGUID guid.GUID, dialErr error, msg1, msg2 string) func(context.Context, testing.TB) {
	return func(ctx context.Context, tb testing.TB) { //nolint:thelper
		ctx, cancel := context.WithTimeout(ctx, hvsockTestTimeout)
		tb.Cleanup(cancel)

		dialCtx, dialCancel := context.WithTimeout(ctx, hvsockDialTimeout)
		tb.Cleanup(dialCancel)

		addr := &winio.HvsockAddr{
			VMID:      winio.HvsockGUIDParent(),
			ServiceID: svcGUID,
		}

		tb.Logf("dialing host on: %v", addr)
		conn, err := winio.Dial(dialCtx, addr)
		if !errors.Is(err, dialErr) {
			tb.Fatalf("expected dial error %v; got: %v", dialErr, err)
		}
		if err != nil {
			// expected an error, cant do much else
			tb.Logf("dial failed: %v", err)
			return
		}

		tb.Cleanup(func() {
			if err := conn.Close(); err != nil {
				tb.Errorf("could not close connection on address %v: %v", conn.LocalAddr(), err)
			}
		})

		tb.Logf("dialled connection: %v->%v", conn.LocalAddr(), conn.RemoteAddr())
		verifyRemoteHvSockConn(ctx, tb, conn, winio.HvsockAddr{
			ServiceID: svcGUID,
			VMID:      winio.HvsockGUIDParent(),
		})

		writeConn(ctx, tb, conn, []byte(msg1))

		got := readConn(ctx, tb, conn)
		if got != msg2 {
			tb.Fatalf("got %q, wanted %q", got, msg2)
		}
	}
}

// code to run from guest when guest is binding to hyper-v socket address.
func guestBindReExecFunc(
	svcGUID guid.GUID,
	inContainer bool,
	listenErr error,
	hostDialErr bool,
	msg1, msg2 string,
) func(context.Context, testing.TB) {
	return func(ctx context.Context, tb testing.TB) { //nolint:thelper
		ctx, cancel := context.WithTimeout(ctx, hvsockTestTimeout)
		tb.Cleanup(cancel)

		// when listening from inside a container, the parent is the uVM
		// so change VM ID to everyone (there is no grandparent wildcard 😢)
		vmID := winio.HvsockGUIDParent()
		if inContainer {
			vmID = winio.HvsockGUIDWildcard()
		}
		addr := &winio.HvsockAddr{
			VMID:      vmID,
			ServiceID: svcGUID,
		}

		tb.Logf("listening to hvsock address: %v", addr)
		l, err := winio.ListenHvsock(addr)
		if !errors.Is(err, listenErr) {
			tb.Fatalf("expected listen error %v; got: %v", listenErr, err)
		}
		if err != nil {
			// expected an error, cant do much else
			return
		}
		tb.Cleanup(func() {
			if err := l.Close(); err != nil {
				tb.Errorf("could not close listener on address %v: %v", addr, err)
			}
		})

		var conn net.Conn
		acceptErrCh := goBlockT(func() (err error) {
			// don't want to call t.Error here, the error could be due to the hv socket lister
			// being closed after timeing out
			conn, err = l.Accept()
			if err != nil {
				tb.Logf("accept failed: %v", err)
			} else {
				tb.Cleanup(func() { conn.Close() })
			}
			return err
		})

		select {
		case <-time.After(hvsockAcceptTimeout):
			if hostDialErr {
				// expected host to error while dialing hv socket connection
				tb.Logf("timed out waiting for host to connect")
				return
			}
			tb.Fatalf("timed out waiting for hvsock connection")
		case err := <-acceptErrCh:
			if err != nil {
				tb.Fatalf("accept failed: %v", err)
			}
		}

		tb.Logf("accepted connection: %v->%v", conn.LocalAddr(), conn.RemoteAddr())
		verifyLocalHvSockConn(ctx, tb, conn, winio.HvsockAddr{
			ServiceID: svcGUID,
			VMID:      winio.HvsockGUIDParent(),
		})

		got := readConn(ctx, tb, conn)
		if got != msg1 {
			tb.Fatalf("got %q, wanted %q", got, msg1)
		}

		writeConn(ctx, tb, conn, []byte(msg2))

		if got2 := readConn(ctx, tb, conn); got2 != "" {
			tb.Logf("read was not empty: %s", got2)
		}
	}
}

//
// hvsock helper functions
//

func readConn(ctx context.Context, tb testing.TB, conn net.Conn) string {
	tb.Helper()

	b := make([]byte, 1024) // hopefully a KiB is enough for a full read

	var n int
	var err error
	waitGoContext(ctx, tb, func() {
		n, err = conn.Read(b)
	})

	if err != nil && !errors.Is(err, io.EOF) {
		tb.Fatalf("read on %v failed: %v", conn.LocalAddr(), err)
	}

	s := string(b[:n])
	tb.Logf("read: %s", s)
	return s
}

func writeConn(ctx context.Context, tb testing.TB, conn net.Conn, b []byte) {
	tb.Helper()

	n := len(b) // write shouldn't modify the len of b, but just in case ...
	var nn int
	var err error
	waitGoContext(ctx, tb, func() {
		nn, err = conn.Write(b)
	})

	if errors.Is(err, windows.WSAESHUTDOWN) {
		tb.Fatalf("write on closed connection (%v)", conn.LocalAddr())
	} else if err != nil {
		tb.Fatalf("write to %v failed: %v", conn.LocalAddr(), err)
	} else if nn != n {
		tb.Fatalf("expected to write %d byte; wrote %d", nn, n)
	}

	tb.Logf("wrote: %s", string(b))
}

func verifyLocalHvSockConn(ctx context.Context, tb testing.TB, conn net.Conn, want winio.HvsockAddr) {
	tb.Helper()

	hvConn, ok := conn.(*winio.HvsockConn)
	if !ok {
		tb.Fatalf("connection is not a of type (*winio.HvsockConn): %T", conn)
	}

	verifyHvSockAddr(ctx, tb, hvConn.LocalAddr(), want)
}

func verifyRemoteHvSockConn(ctx context.Context, tb testing.TB, conn net.Conn, want winio.HvsockAddr) {
	tb.Helper()

	hvConn, ok := conn.(*winio.HvsockConn)
	if !ok {
		tb.Fatalf("connection is not a of type (*winio.HvsockConn): %T", conn)
	}
	verifyHvSockAddr(ctx, tb, hvConn.RemoteAddr(), want)
}

func verifyHvSockAddr(_ context.Context, tb testing.TB, addr net.Addr, want winio.HvsockAddr) {
	tb.Helper()

	got, ok := addr.(*winio.HvsockAddr)
	if !ok {
		tb.Fatalf("address is not a of type (*winio.HvsockAddr): %T", addr)
	}
	tb.Logf("address: %v", got)
	if diff := cmp.Diff(*got, want); diff != "" {
		tb.Fatalf("address mismatch (-want +got):\n%s", diff)
	}
}

var hvsockServiceGUID = sync.OnceValue(func() (guid.GUID, error) {
	return guid.NewV5(guid.GUID{}, []byte(hcsOwner))
})

func getHVSockServiceGUID(tb testing.TB) guid.GUID {
	tb.Helper()

	g, err := hvsockServiceGUID()
	if err != nil {
		tb.Fatalf("could not create Hyper-V socket service ID: %v", err)
	}
	return g
}

//
// misc helpers
//

func waitGoContext(ctx context.Context, tb testing.TB, f func()) {
	tb.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()

	select {
	case <-done:
	case <-ctx.Done():
		tb.Fatalf("context cancelled: %v", ctx.Err())
	}
}

// goBlockT launches f in a go routine and returns a channel to wait on for f's completion.
func goBlockT[T any](f func() T) <-chan T {
	ch := make(chan T)
	go func() {
		defer close(ch)

		ch <- f()
	}()

	return ch
}
