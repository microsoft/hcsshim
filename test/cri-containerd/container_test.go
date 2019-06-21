// +build functional

package cri_containerd

import (
	"bufio"
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

func runLogRotationContainer(t *testing.T, sandboxRequest *runtime.RunPodSandboxRequest, request *runtime.CreateContainerRequest, log string, logArchive string) {
	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podID := runPodSandbox(t, client, ctx, sandboxRequest)
	defer func() {
		stopAndRemovePodSandbox(t, client, ctx, podID)
	}()

	request.PodSandboxId = podID
	request.SandboxConfig = sandboxRequest.Config
	containerID := createContainer(t, client, ctx, request)
	defer func() {
		stopAndRemoveContainer(t, client, ctx, containerID)
	}()
	_, err := client.StartContainer(ctx, &runtime.StartContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		t.Fatalf("failed StartContainer request for container: %s, with: %v", containerID, err)
	}

	// Give some time for log output to accumulate.
	time.Sleep(3 * time.Second)

	// Rotate the logs. This is done by first renaming the existing log file,
	// then calling ReopenContainerLog to cause containerd to start writing to
	// a new log file.

	if err := os.Rename(log, logArchive); err != nil {
		t.Fatalf("failed to rename log: %v", err)
	}

	if _, err := client.ReopenContainerLog(ctx, &runtime.ReopenContainerLogRequest{containerID}); err != nil {
		t.Fatalf("failed to reopen log: %v", err)
	}

	// Give some time for log output to accumulate.
	time.Sleep(3 * time.Second)
}

func Test_RotateLogs_LCOW(t *testing.T) {
	image := "alpine:latest"
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("failed deleting temp dir: %v", err)
		}
	}()
	log := filepath.Join(dir, "log.txt")
	logArchive := filepath.Join(dir, "log-archive.txt")

	pullRequiredLcowImages(t, []string{imageLcowK8sPause, image})
	logrus.SetLevel(logrus.DebugLevel)

	sandboxRequest := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name() + "-Sandbox",
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}

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
