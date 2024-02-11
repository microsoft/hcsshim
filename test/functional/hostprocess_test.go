//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	ctrdoci "github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/jobcontainers"
	"github.com/Microsoft/hcsshim/internal/sync"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/Microsoft/hcsshim/osversion"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	testcontainer "github.com/Microsoft/hcsshim/test/internal/container"
	testlayers "github.com/Microsoft/hcsshim/test/internal/layers"
	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
)

// TODO:
// - Environment
// - working directory
// - "microsoft.com/hostprocess-rootfs-location" and check that rootfs location exists
// - bind suppport?

const (
	system       = `NT AUTHORITY\System`
	localService = `NT AUTHORITY\Local Service`
)

func TestHostProcess_whoami(t *testing.T) {
	requireFeatures(t, featureContainer, featureWCOW, featureHostProcess)
	require.Build(t, osversion.RS5)

	ctx := util.Context(namespacedContext(context.Background()), t)
	ls := windowsImageLayers(ctx, t)

	username := getCurrentUsername(ctx, t)
	t.Logf("current username: %s", username)

	// theres probably a better way to test for this *shrug*
	isSystem := strings.EqualFold(username, system)

	for _, tt := range []struct {
		name   string
		user   ctrdoci.SpecOpts
		whoiam string
	}{
		// Logging in as the current user may require a password.
		// Theres noo guarantee that Administrator, DefaultAccount, or Guest are enabled, so
		// we cannot use them.
		// Best bet is to login into a service user account, which is only possible if we are already
		// running from `NT AUTHORITY\System`.
		{
			name:   "username",
			user:   ctrdoci.WithUser(system),
			whoiam: system,
		},
		{
			name:   "username",
			user:   ctrdoci.WithUser(localService),
			whoiam: localService,
		},
		{
			name:   "inherit",
			user:   testoci.HostProcessInheritUser(),
			whoiam: username,
		},
	} {
		t.Run(tt.name+" "+tt.whoiam, func(t *testing.T) {
			if strings.HasPrefix(strings.ToLower(tt.whoiam), `nt authority\`) && !isSystem {
				t.Skipf("starting HostProcess with account %q as requires running tests as %q", tt.whoiam, system)
			}

			cID := testName(t, "container")
			scratch := testlayers.WCOWScratchDir(ctx, t, "")
			spec := testoci.CreateWindowsSpec(ctx, t, cID,
				testoci.DefaultWindowsSpecOpts(cID,
					ctrdoci.WithProcessCommandLine("whoami.exe"),
					testoci.WithWindowsLayerFolders(append(ls, scratch)),
					testoci.AsHostProcessContainer(),
					tt.user,
				)...)

			c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
			t.Cleanup(cleanup)

			io := testcmd.NewBufferedIO()
			init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, io)
			t.Cleanup(func() {
				testcontainer.Kill(ctx, t, c)
				testcontainer.Wait(ctx, t, c)
			})

			testcmd.WaitExitCode(ctx, t, init, 0)

			io.TestOutput(t, tt.whoiam, nil)
		})
	}

	t.Run("newgroup", func(t *testing.T) {
		// CreateProcessAsUser needs SE_INCREASE_QUOTA_NAME and SE_ASSIGNPRIMARYTOKEN_NAME
		// privileges, which we is not guaranteed for Administrators to have.
		// So, if not System or LocalService, skip.
		//
		// https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessasuserw
		if !isSystem {
			t.Skipf("starting HostProcess within a new localgroup requires running tests as %q", system)
		}

		cID := testName(t, "container")

		groupName := testName(t)
		newLocalGroup(ctx, t, groupName)

		scratch := testlayers.WCOWScratchDir(ctx, t, "")
		spec := testoci.CreateWindowsSpec(ctx, t, cID,
			testoci.DefaultWindowsSpecOpts(cID,
				ctrdoci.WithProcessCommandLine("whoami.exe"),
				testoci.WithWindowsLayerFolders(append(ls, scratch)),
				testoci.AsHostProcessContainer(),
				ctrdoci.WithUser(groupName),
			)...)

		c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
		t.Cleanup(cleanup)

		io := testcmd.NewBufferedIO()
		init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, io)
		t.Cleanup(func() {
			testcontainer.Kill(ctx, t, c)
			testcontainer.Wait(ctx, t, c)
		})

		testcmd.WaitExitCode(ctx, t, init, 0)

		hostname := getHostname(ctx, t)
		expectedUser := cID[:winapi.UserNameCharLimit]
		// whoami returns domain\username
		io.TestOutput(t, hostname+`\`+expectedUser, nil)

		checkLocalGroupMember(ctx, t, groupName, expectedUser)
	})
}

func TestHostProcess_hostname(t *testing.T) {
	requireFeatures(t, featureContainer, featureWCOW, featureHostProcess)
	require.Build(t, osversion.RS5)

	ctx := util.Context(namespacedContext(context.Background()), t)
	ls := windowsImageLayers(ctx, t)

	hostname := getHostname(ctx, t)
	t.Logf("current hostname: %s", hostname)

	cID := testName(t, "container")

	scratch := testlayers.WCOWScratchDir(ctx, t, "")
	spec := testoci.CreateWindowsSpec(ctx, t, cID,
		testoci.DefaultWindowsSpecOpts(cID,
			ctrdoci.WithProcessCommandLine("hostname.exe"),
			testoci.WithWindowsLayerFolders(append(ls, scratch)),
			testoci.AsHostProcessContainer(),
			testoci.HostProcessInheritUser(),
		)...)

	c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
	t.Cleanup(cleanup)

	io := testcmd.NewBufferedIO()
	init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, io)
	t.Cleanup(func() {
		testcontainer.Kill(ctx, t, c)
		testcontainer.Wait(ctx, t, c)
	})

	testcmd.WaitExitCode(ctx, t, init, 0)

	io.TestOutput(t, hostname, nil)
}

// validate if we see the same volumes on the host as in the container.
func TestHostProcess_mountvol(t *testing.T) {
	requireFeatures(t, featureContainer, featureWCOW, featureHostProcess)
	require.Build(t, osversion.RS5)

	ctx := util.Context(namespacedContext(context.Background()), t)
	ls := windowsImageLayers(ctx, t)

	cID := testName(t, "container")

	scratch := testlayers.WCOWScratchDir(ctx, t, "")
	spec := testoci.CreateWindowsSpec(ctx, t, cID,
		testoci.DefaultWindowsSpecOpts(cID,
			ctrdoci.WithProcessCommandLine("mountvol.exe"),
			testoci.WithWindowsLayerFolders(append(ls, scratch)),
			testoci.AsHostProcessContainer(),
			testoci.HostProcessInheritUser(),
		)...)

	c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
	t.Cleanup(cleanup)

	io := testcmd.NewBufferedIO()
	init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, io)
	t.Cleanup(func() {
		testcontainer.Kill(ctx, t, c)
		testcontainer.Wait(ctx, t, c)
	})

	testcmd.WaitExitCode(ctx, t, init, 0)

	// container has been launched as the containers scratch space is a new volume
	volumes, err := exec.CommandContext(ctx, "mountvol.exe").Output()
	t.Logf("host mountvol.exe output:\n%s", string(volumes))
	if err != nil {
		t.Fatalf("failed to exec mountvol: %v", err)
	}

	io.TestOutput(t, string(volumes), nil)
}

func TestHostProcess_VolumeMount(t *testing.T) {
	requireFeatures(t, featureContainer, featureWCOW, featureHostProcess)
	require.Build(t, osversion.RS5)

	ctx := util.Context(namespacedContext(context.Background()), t)
	ls := windowsImageLayers(ctx, t)

	dir := t.TempDir()
	containerDir := `C:\hcsshim_test\path\in\container`

	tmpfileName := "tmpfile"
	containerTmpfile := filepath.Join(containerDir, tmpfileName)

	tmpfile := filepath.Join(dir, tmpfileName)
	if err := os.WriteFile(tmpfile, []byte("test"), 0600); err != nil {
		t.Fatalf("could not create temp file: %v", err)
	}

	for _, tt := range []struct {
		name            string
		hostPath        string
		containerPath   string
		cmd             string
		needsBindFilter bool
	}{
		// CRI is responsible for adding `C:` to the start, and converting `/` to `\`,
		// so here we make everything how Windows wants it
		{
			name:            "dir absolute",
			hostPath:        dir,
			containerPath:   containerDir,
			cmd:             fmt.Sprintf(`dir.exe %s`, containerDir),
			needsBindFilter: true,
		},
		{
			name:          "dir relative",
			hostPath:      dir,
			containerPath: containerDir,
			cmd:           fmt.Sprintf(`dir.exe %s`, strings.ReplaceAll(containerDir, `C:`, `%CONTAINER_SANDBOX_MOUNT_POINT%`)),
		},
		{
			name:            "file absolute",
			hostPath:        tmpfile,
			containerPath:   containerTmpfile,
			cmd:             fmt.Sprintf(`cmd.exe /c type %s`, containerTmpfile),
			needsBindFilter: true,
		},
		{
			name:          "file relative",
			hostPath:      tmpfile,
			containerPath: containerTmpfile,
			cmd:           fmt.Sprintf(`cmd.exe /c type %s`, strings.ReplaceAll(containerTmpfile, `C:`, `%CONTAINER_SANDBOX_MOUNT_POINT%`)),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needsBindFilter && !jobcontainers.FileBindingSupported() {
				t.Skip("bind filter support is required")
			}

			// hpc mount will create the directory on the host, so remove it after test
			t.Cleanup(func() { _ = util.RemoveAll(containerDir) })

			cID := testName(t, "container")

			scratch := testlayers.WCOWScratchDir(ctx, t, "")
			spec := testoci.CreateWindowsSpec(ctx, t, cID,
				testoci.DefaultWindowsSpecOpts(cID,
					ctrdoci.WithProcessCommandLine(tt.cmd),
					ctrdoci.WithMounts([]specs.Mount{
						{
							Source:      tt.hostPath,
							Destination: tt.containerPath,
						},
					}),
					testoci.WithWindowsLayerFolders(append(ls, scratch)),
					testoci.AsHostProcessContainer(),
					testoci.HostProcessInheritUser(),
				)...)

			c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
			t.Cleanup(cleanup)

			io := testcmd.NewBufferedIO() // dir.exe and type.exe will error if theres stdout/err to write to
			init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, io)
			t.Cleanup(func() {
				testcontainer.Kill(ctx, t, c)
				testcontainer.Wait(ctx, t, c)
			})

			if ee := testcmd.Wait(ctx, t, init); ee != 0 {
				out, err := io.Output()
				if out != "" {
					t.Logf("stdout:\n%s", out)
				}
				if err != nil {
					t.Logf("stderr:\n%v", err)
				}
				t.Errorf("got exit code %d, wanted %d", ee, 0)
			}
		})
	}
}

func newLocalGroup(ctx context.Context, tb testing.TB, name string) {
	tb.Helper()

	c := exec.CommandContext(ctx, "net", "localgroup", name, "/add")
	if output, err := c.CombinedOutput(); err != nil {
		tb.Logf("command %q output:\n%s", c.String(), strings.TrimSpace(string(output)))
		tb.Fatalf("failed to create localgroup %q with: %v", name, err)
	}
	tb.Logf("created localgroup: %s", name)

	tb.Cleanup(func() {
		deleteLocalGroup(ctx, tb, name)
	})
}

func deleteLocalGroup(ctx context.Context, tb testing.TB, name string) {
	tb.Helper()

	c := exec.CommandContext(ctx, "net", "localgroup", name, "/delete")
	if output, err := c.CombinedOutput(); err != nil {
		tb.Logf("command %q output:\n%s", c.String(), strings.TrimSpace(string(output)))
		tb.Fatalf("failed to delete localgroup %q: %v", name, err)
	}
	tb.Logf("deleted localgroup: %s", name)
}

// Checks if userName is present in the group `groupName`.
func checkLocalGroupMember(ctx context.Context, tb testing.TB, groupName, userName string) {
	tb.Helper()

	c := exec.CommandContext(ctx, "net", "localgroup", groupName)
	b, err := c.CombinedOutput()
	output := strings.TrimSpace(string(b))
	tb.Logf("command %q output:\n%s", c.String(), output)
	if err != nil {
		tb.Fatalf("failed to check members for localgroup %q: %v", groupName, err)
	}
	if !strings.Contains(strings.ToLower(output), strings.ToLower(userName)) {
		tb.Fatalf("user %s not present in the local group %s", userName, groupName)
	}
}

func getCurrentUsername(_ context.Context, tb testing.TB) string {
	tb.Helper()

	u, err := user.Current() // cached, so no need to save on lookup
	if err != nil {
		tb.Fatalf("could not lookup current user: %v", err)
	}
	return u.Username
}

var hostnameOnce = sync.OnceValue(os.Hostname)

func getHostname(_ context.Context, tb testing.TB) string {
	tb.Helper()

	n, err := hostnameOnce()
	if err != nil {
		tb.Fatalf("could not get hostname: %v", err)
	}
	return n
}
