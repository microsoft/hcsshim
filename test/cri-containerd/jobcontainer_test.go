//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Microsoft/go-winio/vhd"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/test/pkg/definitions/winapi"
	"github.com/Microsoft/hcsshim/test/pkg/require"
)

func getJobContainerPodRequestWCOW(t *testing.T) *runtime.RunPodSandboxRequest {
	t.Helper()
	p := getRunPodSandboxRequest(t, wcowProcessRuntimeHandler)
	if p.Config.Windows == nil {
		p.Config.Windows = &runtime.WindowsPodSandboxConfig{}
	}
	if p.Config.Windows.SecurityContext == nil {
		p.Config.Windows.SecurityContext = &runtime.WindowsSandboxSecurityContext{}
	}
	p.Config.Windows.SecurityContext.HostProcess = true
	return p
}

func getJobContainerRequestWCOW(t *testing.T, podID string, podConfig *runtime.PodSandboxConfig, image string, user string, mounts []*runtime.Mount) *runtime.CreateContainerRequest {
	t.Helper()
	inheritUser := "true"
	if user != "" {
		inheritUser = "false"
	}
	return &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: image,
			},
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Mounts: mounts,
			Annotations: map[string]string{
				annotations.HostProcessInheritUser: inheritUser,
			},
			Windows: &runtime.WindowsContainerConfig{
				SecurityContext: &runtime.WindowsContainerSecurityContext{
					RunAsUsername: user,
					HostProcess:   true,
				},
			},
		},
		PodSandboxId:  podID,
		SandboxConfig: podConfig,
	}
}

func Test_RunContainer_InheritUser_JobContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	username := "nt authority\\system"
	podctx := context.Background()
	sandboxRequest := getJobContainerPodRequestWCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	containerRequest := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageWindowsNanoserver, "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	execResponse := execSync(t, client, ctx, &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         []string{"whoami"},
	})
	stdout := strings.Trim(string(execResponse.Stdout), " \r\n")
	if !strings.Contains(stdout, username) {
		t.Fatalf("expected user: '%s', got '%s'", username, stdout)
	}
}

func Test_RunContainer_Hostname_JobContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	// This test validates that the hostname we see on the host and in the container are the same, and they
	// should be as the container is just a process on the host.
	hostname, err := exec.Command("hostname").Output()
	if err != nil {
		t.Fatalf("failed to get hostname: %s", err)
	}

	podctx := context.Background()
	sandboxRequest := getJobContainerPodRequestWCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	containerRequest := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageWindowsNanoserver, "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	execResponse := execSync(t, client, ctx, &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         []string{"hostname"},
	})
	containerStdout := strings.Trim(string(execResponse.Stdout), " \r\n")
	hostStdout := strings.Trim(string(hostname), " \r\n")
	if hostStdout != containerStdout {
		t.Fatalf("expected hostname to be the same within job container. got %s but expected %s", hostStdout, containerStdout)
	}
}

func makeLocalGroup(name string) error {
	output, err := exec.Command("net", "localgroup", name, "/add").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create localgroup %s with %s, output %s", name, err, string(output))
	}
	return nil
}

func deleteLocalGroup(name string) error {
	output, err := exec.Command("net", "localgroup", name, "/delete").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete localgroup %s with %s, output %s", name, err, string(output))
	}
	return nil
}

// Checks if userName is present in the group `groupName`
func checkLocalGroupMember(groupName, userName string) error {
	output, err := exec.Command("net", "localgroup", groupName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check members for localgroup %s with %s, output %s", groupName, err, string(output))
	}
	if !strings.Contains(string(output), userName) {
		return fmt.Errorf("user %s not present in the local group %s", userName, groupName)
	}
	return nil
}

func Test_RunContainer_GroupName_JobContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	// This test validates that we can create a group, pass the group name to the container and have it run as a local user account in the group.
	podctx := context.Background()
	sandboxRequest := getJobContainerPodRequestWCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	groupName := "jobcontainer_test"
	// Make the local group the container will be creating a local account in.
	if err := makeLocalGroup(groupName); err != nil {
		t.Fatalf("failed to make local group: %s", err)
	}

	defer func() {
		if err := deleteLocalGroup(groupName); err != nil {
			t.Fatalf("failed to delete local group: %s", err)
		}
	}()

	containerRequest := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageWindowsNanoserver, groupName, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	execResponse := execSync(t, client, ctx, &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         []string{"whoami"},
	})
	containerStdout := strings.Trim(string(execResponse.Stdout), " \r\n")
	expectedUserName := containerID[:winapi.UserNameCharLimit]
	if !strings.Contains(containerStdout, expectedUserName) {
		t.Fatalf("expected whoami to be %s. got %s", expectedUserName, containerStdout)
	}

	// Check if user is in the group.
	if err := checkLocalGroupMember(groupName, expectedUserName); err != nil {
		t.Fatal(err)
	}
}

func Test_RunContainer_HNS_JobContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)

	pullRequiredImages(t, []string{imageJobContainerHNS})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getJobContainerPodRequestWCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	containerRequest := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageJobContainerHNS, "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	networkName := fmt.Sprintf("JobContainer-Network-%s", podID)
	containerRequest.Config.Command = []string{
		"hns.exe",
		networkName,
	}

	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// Wait a couple seconds for the network to make sure the network is come up.
	time.Sleep(time.Second * 5)
	// After the init process ends, there should be an HNS network named after os.Args[1] that we passed
	// in. Check if it exists to:
	// 1. See if it worked and if it's not present we need to fail.
	// 2. If it did work we need to delete it.
	network, err := hcn.GetNetworkByName(networkName)
	if err != nil {
		if _, ok := err.(hcn.NetworkNotFoundError); ok {
			t.Fatalf("no network/switch with name %q found: %s", networkName, err)
		}
		t.Fatalf("failed to get network/switch with name %q: %s", networkName, err)
	}

	if err := network.Delete(); err != nil {
		t.Fatalf("failed to delete HNS network: %s", err)
	}
}

func Test_RunContainer_VHD_JobContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)

	pullRequiredImages(t, []string{imageJobContainerVHD})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getJobContainerPodRequestWCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	containerRequest := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageJobContainerVHD, "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dir := t.TempDir()

	vhdPath := filepath.Join(dir, "test.vhdx")
	containerRequest.Config.Command = []string{
		"vhd.exe",
		vhdPath,
	}

	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// Wait a couple seconds for the container to start up and make the vhd.
	time.Sleep(time.Second * 3)
	// The vhd.exe binary in the container will create an NTFS formatted vhd at `vhdPath`. Verify this
	// exists and we can attach it. This is our success case.
	if _, err := os.Stat(vhdPath); os.IsNotExist(err) {
		t.Fatalf("vhd not present at %q: %s", vhdPath, err)
	}

	vhdHandle, err := vhd.OpenVirtualDisk(vhdPath, vhd.VirtualDiskAccessNone, vhd.OpenVirtualDiskFlagNone)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(vhdHandle) //nolint:errcheck

	if err := vhd.AttachVirtualDisk(syscall.Handle(vhdHandle), vhd.AttachVirtualDiskFlagNone, &vhd.AttachVirtualDiskParameters{Version: 1}); err != nil {
		t.Fatalf("failed to attach vhd at %q: %s", vhdPath, err)
	}

	if err := vhd.DetachVirtualDisk(syscall.Handle(vhdHandle)); err != nil {
		t.Fatalf("failed to detach vhd at %q: %s", vhdPath, err)
	}
}

func Test_RunContainer_ETW_JobContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)

	pullRequiredImages(t, []string{imageJobContainerETW})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getJobContainerPodRequestWCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	containerRequest := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageJobContainerETW, "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// For this test we'll launch an image that has a wprp file inside that we'll use to take an etw trace.
	// After the etl file is generated we'll use tracerpt to create a report/dump file of the trace. This is
	// just to verify that a common use case of grabbing host traces/diagnostics can be achieved.
	dir := t.TempDir()
	// Need for network name is solely because the only provider defined in the image is for HNS, so
	// we do a simple HNS operation to get some output.
	var (
		networkName = fmt.Sprintf("JobContainer-Network-%s", podID)
		etlFile     = filepath.Join(dir, "output.etl")
		dumpFile    = filepath.Join(dir, "output.xml")
	)
	containerRequest.Config.Command = []string{
		"etw.exe",
		networkName,
		etlFile,
		dumpFile,
	}

	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// Wait some time for the container to start up and manipulate the etl file.
	time.Sleep(time.Second * 10)
	if _, err := os.Stat(etlFile); os.IsNotExist(err) {
		t.Fatalf("failed to find etl file %q: %s", etlFile, err)
	}

	if _, err := os.Stat(dumpFile); os.IsNotExist(err) {
		t.Fatalf("failed to find dump file %q: %s", dumpFile, err)
	}
}

func Test_RunContainer_HostVolumes_JobContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getJobContainerPodRequestWCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	containerRequest := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageWindowsNanoserver, "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	execResponse := execSync(t, client, ctx, &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         []string{"mountvol"},
	})
	containerStdout := strings.Trim(string(execResponse.Stdout), " \r\n")

	// This test validates we see the same volumes on the host as in the container. We have to do this after the
	// container has been launched as the containers scratch space is a new volume
	volumes, err := exec.Command("mountvol").Output()
	if err != nil {
		t.Fatalf("failed to get volumes: %s", err)
	}
	hostStdout := strings.Trim(string(volumes), " \r\n")

	if hostStdout != containerStdout {
		t.Fatalf("expected volumes to be the same within job process container. got %q but expected %q", hostStdout, containerStdout)
	}
}

func Test_RunContainer_JobContainer_VolumeMount(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)
	require.ExactBuild(t, osversion.RS5)

	client := newTestRuntimeClient(t)
	dir := t.TempDir()

	tmpfn := filepath.Join(dir, "tmpfile")
	if err := os.WriteFile(tmpfn, []byte("test"), 0777); err != nil {
		t.Fatal(err)
	}

	mountDriveLetter := []*runtime.Mount{
		{
			HostPath:      dir,
			ContainerPath: "C:\\path\\in\\container",
		},
	}

	mountNoDriveLetter := []*runtime.Mount{
		{
			HostPath:      dir,
			ContainerPath: "/path/in/container",
		},
	}

	mountSingleFile := []*runtime.Mount{
		{
			HostPath:      tmpfn,
			ContainerPath: "/path/in/container/testfile",
		},
	}

	type config struct {
		name           string
		containerName  string
		sandboxImage   string
		containerImage string
		exec           []string
		mounts         []*runtime.Mount
	}

	tests := []config{
		{
			name:           "JobContainer_VolumeMount_DriveLetter",
			containerName:  t.Name() + "-Container-DriveLetter",
			sandboxImage:   imageWindowsNanoserver,
			containerImage: imageWindowsNanoserver,
			mounts:         mountDriveLetter,
			exec:           []string{"cmd", "/c", "dir", "%CONTAINER_SANDBOX_MOUNT_POINT%\\path\\in\\container\\tmpfile"},
		},
		{
			name:           "JobContainer_VolumeMount_NoDriveLetter",
			containerName:  t.Name() + "-Container-NoDriveLetter",
			sandboxImage:   imageWindowsNanoserver,
			containerImage: imageWindowsNanoserver,
			mounts:         mountNoDriveLetter,
			exec:           []string{"cmd", "/c", "dir", "%CONTAINER_SANDBOX_MOUNT_POINT%\\path\\in\\container\\tmpfile"},
		},
		{
			name:           "JobContainer_VolumeMount_SingleFile",
			containerName:  t.Name() + "-Container-SingleFile",
			sandboxImage:   imageWindowsNanoserver,
			containerImage: imageWindowsNanoserver,
			mounts:         mountSingleFile,
			exec:           []string{"cmd", "/c", "type", "%CONTAINER_SANDBOX_MOUNT_POINT%\\path\\in\\container\\testfile"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requiredImages := []string{test.sandboxImage, test.containerImage}
			pullRequiredImages(t, requiredImages)

			podctx := context.Background()
			sandboxRequest := getJobContainerPodRequestWCOW(t)

			podID := runPodSandbox(t, client, podctx, sandboxRequest)
			defer removePodSandbox(t, client, podctx, podID)
			defer stopPodSandbox(t, client, podctx, podID)

			containerRequest := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageWindowsNanoserver, "", test.mounts)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			containerID := createContainer(t, client, ctx, containerRequest)
			defer removeContainer(t, client, ctx, containerID)
			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			r := execSync(t, client, ctx, &runtime.ExecSyncRequest{
				ContainerId: containerID,
				Cmd:         test.exec,
			})
			if r.ExitCode != 0 {
				t.Fatalf("failed with exit code %d checking for job container mount: %s", r.ExitCode, string(r.Stderr))
			}
		})
	}
}

func Test_RunContainer_JobContainer_Environment(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)

	client := newTestRuntimeClient(t)

	type config struct {
		name           string
		containerName  string
		sandboxImage   string
		containerImage string
		env            []*runtime.KeyValue
		exec           []string
	}

	tests := []config{
		{
			name:           "JobContainer_Env_NoMountPoint",
			containerName:  t.Name() + "-Container-WithNoMountPoint",
			sandboxImage:   imageWindowsNanoserver,
			containerImage: imageWindowsNanoserver,
			env: []*runtime.KeyValue{
				{
					Key: "PATH", Value: "C:\\Windows\\system32;C:\\Windows",
				},
			},
			exec: []string{"cmd", "/c", "IF", "%PATH%", "==", "C:\\Windows\\system32;C:\\Windows", "( exit 0 )", "ELSE", "(exit -1)"},
		},
		{
			name:           "JobContainer_VolumeMount_WithMountPoint",
			containerName:  t.Name() + "-Container-WithMountPoint",
			sandboxImage:   imageWindowsNanoserver,
			containerImage: imageWindowsNanoserver,
			env: []*runtime.KeyValue{
				{
					Key: "PATH", Value: "%CONTAINER_SANDBOX_MOUNT_POINT%\\apps\\vim\\;C:\\Windows\\system32;C:\\Windows",
				},
			},
			exec: []string{"cmd", "/c", "IF", "%PATH%", "==", "%CONTAINER_SANDBOX_MOUNT_POINT%\\apps\\vim\\;C:\\Windows\\system32;C:\\Windows", "( exit -1 )", "ELSE", "(exit 0)"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requiredImages := []string{test.sandboxImage, test.containerImage}
			pullRequiredImages(t, requiredImages)

			podctx := context.Background()
			sandboxRequest := getJobContainerPodRequestWCOW(t)

			podID := runPodSandbox(t, client, podctx, sandboxRequest)
			defer removePodSandbox(t, client, podctx, podID)
			defer stopPodSandbox(t, client, podctx, podID)

			containerRequest := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageWindowsNanoserver, "", nil)
			containerRequest.Config.Envs = test.env
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			containerID := createContainer(t, client, ctx, containerRequest)
			defer removeContainer(t, client, ctx, containerID)
			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)

			r := execSync(t, client, ctx, &runtime.ExecSyncRequest{
				ContainerId: containerID,
				Cmd:         test.exec,
			})
			if r.ExitCode != 0 {
				t.Fatalf("failed with exit code %d checking for job container mount: %s", r.ExitCode, string(r.Stderr))
			}
		})
	}
}

func Test_RunContainer_WorkingDirectory_JobContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)
	require.ExactBuild(t, osversion.RS5)

	client := newTestRuntimeClient(t)

	type config struct {
		name           string
		containerName  string //nolint:unused // may be used in future tests
		workDir        string
		sandboxImage   string
		containerImage string
		cmd            []string
	}

	tests := []config{
		{
			name:           "JobContainer_WorkDir_DriveLetter",
			workDir:        "C:\\go\\",
			sandboxImage:   imageWindowsNanoserver,
			containerImage: imageJobContainerWorkDir,
			cmd:            []string{"src\\workdir\\workdir.exe"},
		},
		{
			name:           "JobContainer_WorkDir_NoDriveLetter",
			workDir:        "/go",
			sandboxImage:   imageWindowsNanoserver,
			containerImage: imageJobContainerWorkDir,
			cmd:            []string{"src/workdir/workdir.exe"},
		},
		{
			name:           "JobContainer_WorkDir_Default", // Just use the workdir from the image, which is C:\\go\\src\\workdir
			sandboxImage:   imageWindowsNanoserver,
			containerImage: imageJobContainerWorkDir,
			cmd:            []string{"workdir.exe"},
		},
		{
			name:           "JobContainer_WorkDir_EnvVar", // Test that putting the envvar in the workdir functions.
			workDir:        "$env:CONTAINER_SANDBOX_MOUNT_POINT\\go\\src\\workdir\\",
			sandboxImage:   imageWindowsNanoserver,
			containerImage: imageJobContainerWorkDir,
			cmd:            []string{"workdir.exe"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requiredImages := []string{test.sandboxImage, test.containerImage}
			pullRequiredImages(t, requiredImages)

			podctx := context.Background()
			sandboxRequest := getJobContainerPodRequestWCOW(t)

			podID := runPodSandbox(t, client, podctx, sandboxRequest)
			defer removePodSandbox(t, client, podctx, podID)
			defer stopPodSandbox(t, client, podctx, podID)

			containerRequest := &runtime.CreateContainerRequest{
				Config: &runtime.ContainerConfig{
					Metadata: &runtime.ContainerMetadata{
						Name: test.name,
					},
					Image: &runtime.ImageSpec{
						Image: test.containerImage,
					},
					Command:    test.cmd,
					WorkingDir: test.workDir,
					Annotations: map[string]string{
						annotations.HostProcessInheritUser: "true",
					},
					Windows: &runtime.WindowsContainerConfig{
						SecurityContext: &runtime.WindowsContainerSecurityContext{
							HostProcess: true,
						},
					},
				},
				PodSandboxId:  podID,
				SandboxConfig: sandboxRequest.Config,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			containerID := createContainer(t, client, ctx, containerRequest)
			defer removeContainer(t, client, ctx, containerID)
			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)
		})
	}
}

// Test of the fix for the behavior detailed here https://github.com/microsoft/hcsshim/issues/1199
// The underlying issue was that we would escape the args passed to us to form a commandline, and then split the commandline back into args
// to pass to exec.Cmd in the stdlib. exec.Cmd internally does escaping of its own and thus would result in double quoting for certain
// commandlines.
func Test_DoubleQuoting_JobContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)

	pullRequiredImages(t, []string{imageJobContainerCmdline})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getJobContainerPodRequestWCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	containerRequest := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageJobContainerCmdline, "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	execResponse := execSync(t, client, ctx, &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         []string{"cmdline.exe", `"quote test"`},
	})

	expected := `cmdline.exe "quote test"`
	// Check that there's no double quoting going on.
	// e.g. `cmdline.exe ""quote test"" `
	if string(execResponse.Stdout) != expected {
		t.Fatalf("expected cmdline for exec to be %q but got %q", expected, string(execResponse.Stdout))
	}
}

// Test that mounts show up at the expected destination if the host supports file binding.
func Test_BindSupport_JobContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)
	require.Build(t, osversion.V20H1)

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	podctx := context.Background()
	sandboxRequest := getJobContainerPodRequestWCOW(t)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	// Create temp directory and populate with an empty file. We're going to use this
	// as a mount for the container. The purpose is just to test that wherever the
	// destination for the mount is set to is where the mount actually shows up if we
	// have file binding support.
	testDir := t.TempDir()
	testFile := filepath.Join(testDir, "testfile.txt")
	f, err := os.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	ctrPath := `C:\jobcontainer-mount-test\`
	mount := []*runtime.Mount{
		{
			HostPath:      testDir,
			ContainerPath: ctrPath,
		},
	}

	containerRequest := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageWindowsNanoserver, "", mount)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	r := execSync(t, client, ctx, &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         []string{"cmd", "/c", `dir`, filepath.Join(ctrPath, "testfile.txt")},
	})

	exitCode := int(r.ExitCode)
	errorMsg := string(r.Stderr)
	if r.ExitCode != 0 || len(errorMsg) != 0 {
		t.Fatalf("Failed execution inside container %s with error: %s, exitCode: %d", containerID, errorMsg, exitCode)
	}
}

// Test that mounts are unique per container even if the same container path is used.
func Test_BindSupport_MultipleContainers_JobContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWProcess, featureHostProcess)
	require.Build(t, osversion.V20H1)

	pullRequiredImages(t, []string{imageWindowsNanoserver})
	client := newTestRuntimeClient(t)

	// Create temp directories and populate with empty files. We're going to do the following:
	//
	// tempDir1 (with testfile1.txt) -> container1 at C:\jobcontainer-mount-test\
	// tempDir2 (with testfile2.txt) -> container2 at C:\jobcontainer-mount-test\
	//
	// and then verify that we don't have a merged view of the mounts in both containers as they should be
	// unique per silo.
	testDir1 := t.TempDir()
	f, err := os.Create(filepath.Join(testDir1, "testfile1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	testDir2 := t.TempDir()
	f, err = os.Create(filepath.Join(testDir2, "testfile2.txt"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	ctrPath := `C:\jobcontainer-mount-test\`
	ctrMount1 := []*runtime.Mount{
		{
			HostPath:      testDir1,
			ContainerPath: ctrPath,
		},
	}
	ctrMount2 := []*runtime.Mount{
		{
			HostPath:      testDir2,
			ContainerPath: ctrPath,
		},
	}

	podctx := context.Background()
	sandboxRequest := getJobContainerPodRequestWCOW(t)
	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	container1Request := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageWindowsNanoserver, "", ctrMount1)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	container1ID := createContainer(t, client, ctx, container1Request)
	defer removeContainer(t, client, ctx, container1ID)
	startContainer(t, client, ctx, container1ID)
	defer stopContainer(t, client, ctx, container1ID)

	container2Request := getJobContainerRequestWCOW(t, podID, sandboxRequest.Config, imageWindowsNanoserver, "", ctrMount2)
	container2Request.Config.Metadata.Name += "2"
	container2ID := createContainer(t, client, ctx, container2Request)
	defer removeContainer(t, client, ctx, container2ID)
	startContainer(t, client, ctx, container2ID)
	defer stopContainer(t, client, ctx, container2ID)

	// Check that we can't see the contents of ctr1's mount.
	unexpected := filepath.Join(ctrPath, "testfile1.txt")
	r := execSync(t, client, ctx, &runtime.ExecSyncRequest{
		ContainerId: container2ID,
		Cmd:         []string{"cmd", "/c", `dir`, unexpected},
	})

	exitCode := int(r.ExitCode)
	errorMsg := string(r.Stderr)
	if exitCode == 0 || len(errorMsg) == 0 {
		t.Fatalf("Expected %s to not be available in the container, instead got: %s", unexpected, string(r.Stdout))
	}

	// Now check that we *can* see the contents of ctr2's
	expected := filepath.Join(ctrPath, "testfile2.txt")
	r = execSync(t, client, ctx, &runtime.ExecSyncRequest{
		ContainerId: container2ID,
		Cmd:         []string{"cmd", "/c", `dir`, expected},
	})

	exitCode = int(r.ExitCode)
	errorMsg = string(r.Stderr)
	if exitCode != 0 || len(errorMsg) != 0 {
		t.Fatalf("Expected %s to be available in the container, instead got: %s", expected, errorMsg)
	}
}
