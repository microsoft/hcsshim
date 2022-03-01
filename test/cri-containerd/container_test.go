//go:build functional
// +build functional

package cri_containerd

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/sirupsen/logrus"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func runLogRotationContainer(t *testing.T, sandboxRequest *runtime.RunPodSandboxRequest, request *runtime.CreateContainerRequest, log string, logArchive string) {
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request.PodSandboxId = podID
	request.SandboxConfig = sandboxRequest.Config

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// Give some time for log output to accumulate.
	time.Sleep(3 * time.Second)

	// Rotate the logs. This is done by first renaming the existing log file,
	// then calling ReopenContainerLog to cause containerd to start writing to
	// a new log file.

	if err := os.Rename(log, logArchive); err != nil {
		t.Fatalf("failed to rename log: %v", err)
	}

	if _, err := client.ReopenContainerLog(ctx, &runtime.ReopenContainerLogRequest{ContainerId: containerID}); err != nil {
		t.Fatalf("failed to reopen log: %v", err)
	}

	// Give some time for log output to accumulate.
	time.Sleep(3 * time.Second)
}

func runContainerLifetime(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID string) {
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	stopContainer(t, client, ctx, containerID)
}

func Test_RotateLogs_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	image := "alpine:latest"
	dir := t.TempDir()
	log := filepath.Join(dir, "log.txt")
	logArchive := filepath.Join(dir, "log-archive.txt")

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, image})
	logrus.SetLevel(logrus.DebugLevel)

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: image,
			},
			Command: []string{
				"ash",
				"-c",
				"i=0; while true; do echo $i; i=$(expr $i + 1); sleep .1; done",
			},
			LogPath: log,
			Linux:   &runtime.LinuxContainerConfig{},
		},
	}

	runLogRotationContainer(t, sandboxRequest, request, log, logArchive)

	// Make sure we didn't lose any values while rotating. First set of output
	// should be in logArchive, followed by the output in log.

	logArchiveFile, err := os.Open(logArchive)
	if err != nil {
		t.Fatal(err)
	}
	defer logArchiveFile.Close()

	logFile, err := os.Open(log)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	s := bufio.NewScanner(io.MultiReader(logArchiveFile, logFile))
	expected := 0
	for s.Scan() {
		v := strings.Fields(s.Text())
		n, err := strconv.Atoi(v[len(v)-1])
		if err != nil {
			t.Fatalf("failed to parse log value as integer: %v", err)
		}
		if n != expected {
			t.Fatalf("missing expected output value: %v (got %v)", expected, n)
		}
		expected++
	}
}

func Test_RunContainer_Events_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})
	client := newTestRuntimeClient(t)

	podctx, podcancel := context.WithCancel(context.Background())
	defer podcancel()
	targetNamespace := "k8s.io"

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	podID := runPodSandbox(t, client, podctx, sandboxRequest)
	defer removePodSandbox(t, client, podctx, podID)
	defer stopPodSandbox(t, client, podctx, podID)

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},
			Linux: &runtime.LinuxContainerConfig{},
		},
		PodSandboxId:  podID,
		SandboxConfig: sandboxRequest.Config,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	topicNames, filters := getTargetRunTopics()
	eventService := newTestEventService(t)
	stream, errs := eventService.Subscribe(ctx, filters...)

	containerID := createContainer(t, client, podctx, request)
	runContainerLifetime(t, client, podctx, containerID)

	for _, topic := range topicNames {
		select {
		case env := <-stream:
			if topic != env.Topic {
				t.Fatalf("event topic %v does not match expected topic %v", env.Topic, topic)
			}
			if targetNamespace != env.Namespace {
				t.Fatalf("event namespace %v does not match expected namespace %v", env.Namespace, targetNamespace)
			}
			t.Logf("event topic seen: %v", env.Topic)

			id, _, err := convertEvent(env.Event)
			if err != nil {
				t.Fatalf("topic %v event: %v", env.Topic, err)
			}
			if id != containerID {
				t.Fatalf("event topic %v belongs to container %v, not targeted container %v", env.Topic, id, containerID)
			}
		case e := <-errs:
			t.Fatalf("event subscription err %v", e)
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("event %v deadline exceeded", topic)
			}
		}
	}
}

func Test_RunContainer_ForksThenExits_ShowsAsExited_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	podRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)
	podID := runPodSandbox(t, client, ctx, podRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	containerRequest := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				// Fork a background process (that runs forever), then exit.
				"ash",
				"-c",
				"ash -c 'while true; do echo foo; sleep 1; done' &",
			},
			Linux: &runtime.LinuxContainerConfig{},
		},
		PodSandboxId:  podID,
		SandboxConfig: podRequest.Config,
	}
	containerID := createContainer(t, client, ctx, containerRequest)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// Give the container init time to exit.
	time.Sleep(5 * time.Second)

	// Validate that the container shows as exited. Once the container init
	// dies, the forked background process should be killed off.
	statusResponse, err := client.ContainerStatus(ctx, &runtime.ContainerStatusRequest{ContainerId: containerID})
	if err != nil {
		t.Fatalf("failed to get container status: %v", err)
	}
	if statusResponse.Status.State != runtime.ContainerState_CONTAINER_EXITED {
		t.Fatalf("container expected to be exited but is in state %s", statusResponse.Status.State)
	}
}

func Test_RunContainer_ZeroVPMEM_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.PreferredRootFSType: "initrd",
			annotations.VPMemCount:          "0",
		}),
	)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	runContainerLifetime(t, client, ctx, containerID)
}

func Test_RunContainer_ZeroVPMEM_Multiple_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(
		t,
		lcowRuntimeHandler,
		WithSandboxAnnotations(map[string]string{
			annotations.PreferredRootFSType: "initrd",
			annotations.VPMemCount:          "0",
		}),
	)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: "",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	request.Config.Metadata.Name = "Container-1"
	containerIDOne := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerIDOne)
	startContainer(t, client, ctx, containerIDOne)
	defer stopContainer(t, client, ctx, containerIDOne)

	request.Config.Metadata.Name = "Container-2"
	containerIDTwo := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerIDTwo)
	startContainer(t, client, ctx, containerIDTwo)
	defer stopContainer(t, client, ctx, containerIDTwo)
}

func Test_RunContainer_SandboxDevice_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)
	sandboxRequest.Config.Linux = &runtime.LinuxPodSandboxConfig{
		SecurityContext: &runtime.LinuxSandboxSecurityContext{
			Privileged: true,
		},
	}

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},

			Devices: []*runtime.Device{
				{
					HostPath: "/dev/fuse",
				},
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	cmd := []string{"ls", "/dev/fuse"}
	containerExecReq := &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     20,
	}
	r := execSync(t, client, ctx, containerExecReq)
	if r.ExitCode != 0 {
		t.Fatalf("failed with exit code %d: %s", r.ExitCode, string(r.Stderr))
	}
	if string(r.Stdout) == "" {
		t.Fatal("did not find expected device /dev/fuse in container")
	}
}

func Test_RunContainer_NonDefault_User(t *testing.T) {
	requireFeatures(t, featureLCOW)

	type config struct {
		containerSecCtx *runtime.LinuxContainerSecurityContext
		name            string
	}
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	podReq := getRunPodSandboxRequest(t, lcowRuntimeHandler)
	podID := runPodSandbox(t, client, ctx, podReq)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	tests := []config{
		{
			containerSecCtx: &runtime.LinuxContainerSecurityContext{
				RunAsUsername: "guest",
			},
			name: "RunAsUsername",
		},
		{
			containerSecCtx: &runtime.LinuxContainerSecurityContext{
				RunAsUser: &runtime.Int64Value{
					Value: 10001,
				},
			},
			name: "RunAsUserUID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			conReq := &runtime.CreateContainerRequest{
				Config: &runtime.ContainerConfig{
					Metadata: &runtime.ContainerMetadata{
						Name: t.Name() + "-Container",
					},
					Image: &runtime.ImageSpec{
						Image: imageLcowAlpine,
					},
					Command: []string{
						"top",
					},
					Linux: &runtime.LinuxContainerConfig{
						SecurityContext: test.containerSecCtx,
					},
				},
				PodSandboxId:  podID,
				SandboxConfig: podReq.Config,
			}

			containerID := createContainer(t, client, ctx, conReq)
			defer removeContainer(t, client, ctx, containerID)
			startContainer(t, client, ctx, containerID)
			defer stopContainer(t, client, ctx, containerID)
		})
	}
}

func Test_RunContainer_ShareScratch_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},
			Annotations: map[string]string{
				"containerd.io/snapshot/io.microsoft.container.storage.reuse-scratch": "true",
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// Verify that we only have one disk mounted. Generally we'd have sda + sdb for two containers
	// (Pause + 1 workload container).
	cmd := []string{"ls", "/dev/sdb"}
	containerExecReq := &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     20,
	}
	r := execSync(t, client, ctx, containerExecReq)
	if r.ExitCode == 0 {
		t.Fatalf("expected /dev/sdb to not be present %d: %s", r.ExitCode, string(r.Stdout))
	}
}

func findOverlaySize(t *testing.T, ctx context.Context, client runtime.RuntimeServiceClient, cid string) []string {
	cmd := []string{"df"}
	containerExecReq := &runtime.ExecSyncRequest{
		ContainerId: cid,
		Cmd:         cmd,
		Timeout:     20,
	}
	r := execSync(t, client, ctx, containerExecReq)

	if r.ExitCode != 0 {
		t.Fatalf("failed with exit code %d: %s", r.ExitCode, string(r.Stderr))
	}

	// Format of output for df is below
	// Filesystem           1K-blocks      Used Available Use% Mounted on
	// overlay               20642524        36  19577528   0% /
	// tmpfs                    65536         0     65536   0% /dev
	var (
		scanner = bufio.NewScanner(strings.NewReader(string(r.Stdout)))
		cols    []string
		found   bool
	)
	for scanner.Scan() {
		outputLine := scanner.Text()
		if cols = strings.Fields(outputLine); cols[0] == "overlay" && cols[5] == "/" {
			found = true
			t.Log(outputLine)
			break
		}
	}

	if !found {
		t.Fatalf("could not find the correct output line for overlay mount on / n: error: %v, exitcode: %d", string(r.Stdout), r.ExitCode)
	}
	return cols
}

func Test_RunContainer_ShareScratch_CheckSize_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: "",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			Command: []string{
				"top",
			},
			Annotations: map[string]string{
				"containerd.io/snapshot/io.microsoft.container.storage.reuse-scratch": "true",
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	request.Config.Metadata.Name = "Container-1"
	containerIDOne := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerIDOne)
	startContainer(t, client, ctx, containerIDOne)
	defer stopContainer(t, client, ctx, containerIDOne)

	request.Config.Metadata.Name = "Container-2"
	containerIDTwo := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerIDTwo)
	startContainer(t, client, ctx, containerIDTwo)
	defer stopContainer(t, client, ctx, containerIDTwo)

	cols := findOverlaySize(t, ctx, client, containerIDOne)
	availableSizeContainerOne := cols[3]

	cols = findOverlaySize(t, ctx, client, containerIDTwo)
	availableSizeContainerTwo := cols[3]

	// Check the initial size for both containers.
	if availableSizeContainerOne != availableSizeContainerTwo {
		t.Fatalf("expected available rootfs size to be the same, got: %s and %s", availableSizeContainerOne, availableSizeContainerTwo)
	}

	// Write a 10MB file and then check size available on the root fs in both containers.
	// It should be the same amount as it's being backed by the same disk.
	cmd := []string{"dd", "if=/dev/urandom", "of=testfile", "bs=10MB", "count=1"}
	containerExecReq := &runtime.ExecSyncRequest{
		ContainerId: containerIDOne,
		Cmd:         cmd,
		Timeout:     20,
	}
	r := execSync(t, client, ctx, containerExecReq)

	if r.ExitCode != 0 {
		t.Fatalf("failed with exit code %d: %s", r.ExitCode, string(r.Stderr))
	}

	// Grab the size available after the first write
	cols = findOverlaySize(t, ctx, client, containerIDOne)
	availableSizeContainerOne = cols[3]

	// Now the same for container two
	cols = findOverlaySize(t, ctx, client, containerIDTwo)
	availableSizeContainerTwo = cols[3]

	if availableSizeContainerOne != availableSizeContainerTwo {
		t.Fatalf("expected available rootfs size to be the same, got: %s and %s", availableSizeContainerOne, availableSizeContainerTwo)
	}
}

func Test_CreateContainer_DevShmSize(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podReq := getRunPodSandboxRequest(t, lcowRuntimeHandler)
	podID := runPodSandbox(t, client, ctx, podReq)
	defer removePodSandbox(t, client, ctx, podID)

	cmd := []string{"ash", "-c", "while true; do sleep 1; done"}
	contReq1 := getCreateContainerRequest(podID, "test-container-devshm-256", imageLcowAlpine, cmd, podReq.Config)
	// the /dev/shm size is expected to be in KB, set it to 256 MB
	size := 256 * 1024
	contReq1.Config.Annotations = map[string]string{
		annotations.LCOWDevShmSizeInKb: strconv.Itoa(size),
	}
	containerID1 := createContainer(t, client, ctx, contReq1)
	defer removeContainer(t, client, ctx, containerID1)

	startContainer(t, client, ctx, containerID1)
	defer stopContainer(t, client, ctx, containerID1)

	contReq2 := getCreateContainerRequest(podID, "test-container-devshm-default", imageLcowAlpine, cmd, podReq.Config)
	containerID2 := createContainer(t, client, ctx, contReq2)
	defer removeContainer(t, client, ctx, containerID2)
	startContainer(t, client, ctx, containerID2)
	defer stopContainer(t, client, ctx, containerID2)

	// check /dev/shm size on container 1, should be set to 256 MB
	execResponse1 := execSync(t, client, ctx, &runtime.ExecSyncRequest{
		ContainerId: containerID1,
		Cmd:         []string{"df", "-h", "/dev/shm"},
	})
	if !strings.Contains(string(execResponse1.Stdout), "256.0M") {
		t.Fatalf("expected the size of /dev/shm to be 256MB. Got output instead: %s", string(execResponse1.Stdout))
	}

	// check /dev/shm size on container 2, should be set to default 64 MB
	execResponse2 := execSync(t, client, ctx, &runtime.ExecSyncRequest{
		ContainerId: containerID2,
		Cmd:         []string{"df", "-h", "/dev/shm"},
	})
	if !strings.Contains(string(execResponse2.Stdout), "64.0M") {
		t.Fatalf("expected the size of /dev/shm to be 64MB. Got output instead: %s", string(execResponse1.Stdout))
	}
}

func Test_CreateContainer_HugePageMount_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowAlpine})

	annots := map[string]string{
		annotations.FullyPhysicallyBacked: "true",
		annotations.MemorySizeInMB:        "2048",
		annotations.KernelBootOptions:     "hugepagesz=2M hugepages=10",
	}
	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler, WithSandboxAnnotations(annots))

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowAlpine,
			},
			// Hold this command open until killed
			Command: []string{
				"top",
			},
			Mounts: []*runtime.Mount{
				{
					HostPath:      "hugepages://2M/hugepage2M",
					ContainerPath: "/mnt/hugepage2M",
					Readonly:      false,
					Propagation:   runtime.MountPropagation_PROPAGATION_BIDIRECTIONAL,
				},
			},
		},
	}

	request.PodSandboxId = podID
	request.SandboxConfig = sandboxRequest.Config

	containerId := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerId)
	startContainer(t, client, ctx, containerId)
	defer stopContainer(t, client, ctx, containerId)

	execCommand := []string{"grep", "-i", "/mnt/hugepage2M", "/proc/mounts"}

	output, errorMsg, exitCode := execContainer(t, client, ctx, containerId, execCommand)
	if exitCode != 0 || len(errorMsg) > 0 {
		t.Fatalf("failed to exec in hugepage container errorMsg: %s, exitcode: %v\n", errorMsg, exitCode)
	}

	if !strings.Contains(output, "hugetlbfs") {
		t.Fatalf("output is supposed to contain hugetlbfs, output: %s", output)
	}

	if !strings.Contains(output, "pagesize=2M") {
		t.Fatalf("output is supposed to contain pagesize=2M, output: %s", output)
	}
}

func Test_RunContainer_ExecUser_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowCustomUser})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	cmd := []string{"sh", "-c", "while true; do sleep 1; done"}
	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowCustomUser,
			},
			Command: cmd,
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// The `imageLcowCustomUser` image has a user created in the image named test that is set to run the init process as. This tests that
	// any execed processes will honor the user set for the container also.
	cmd = []string{"whoami"}
	containerExecReq := &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     20,
	}
	r := execSync(t, client, ctx, containerExecReq)
	if r.ExitCode != 0 {
		t.Fatalf("failed with exit code %d: %s", r.ExitCode, string(r.Stderr))
	}

	if !strings.Contains(string(r.Stdout), "test") {
		t.Fatalf("expected user for exec to be 'test', got %q", string(r.Stdout))
	}
}

func Test_RunContainer_ExecUser_Root_LCOW(t *testing.T) {
	requireFeatures(t, featureLCOW)

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause, imageLcowCustomUser})

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sandboxRequest := getRunPodSandboxRequest(t, lcowRuntimeHandler)

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer removePodSandbox(t, client, ctx, podID)
	defer stopPodSandbox(t, client, ctx, podID)

	// Overide what user to run the container as and see if the exec also runs as root now.
	cmd := []string{"sh", "-c", "while true; do sleep 1; done"}
	request := &runtime.CreateContainerRequest{
		PodSandboxId: podID,
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageLcowCustomUser,
			},
			Command: cmd,
			Linux: &runtime.LinuxContainerConfig{
				SecurityContext: &runtime.LinuxContainerSecurityContext{
					RunAsUsername: "root",
				},
			},
		},
		SandboxConfig: sandboxRequest.Config,
	}

	containerID := createContainer(t, client, ctx, request)
	defer removeContainer(t, client, ctx, containerID)
	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// The `imageLcowCustomUser` image has a user created in the image named test that is set to run the init process as. This tests that
	// any execed processes will honor the user set for the container also.
	cmd = []string{"whoami"}
	containerExecReq := &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     20,
	}
	r := execSync(t, client, ctx, containerExecReq)
	if r.ExitCode != 0 {
		t.Fatalf("failed with exit code %d: %s", r.ExitCode, string(r.Stderr))
	}

	if !strings.Contains(string(r.Stdout), "root") {
		t.Fatalf("expected user for exec to be 'root', got %q", string(r.Stdout))
	}
}
