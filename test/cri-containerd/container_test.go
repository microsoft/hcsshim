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

	if _, err := client.ReopenContainerLog(ctx, &runtime.ReopenContainerLogRequest{containerID}); err != nil {
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

func Test_RunContainer_Events_LCOW(t *testing.T) {
	pullRequiredLcowImages(t, []string{imageLcowK8sPause, imageLcowAlpine})
	client := newTestRuntimeClient(t)

	podctx, podcancel := context.WithCancel(context.Background())
	defer podcancel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	targetNamespace := "k8s.io"

	sandboxRequest := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name(),
				Uid:       "0",
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: lcowRuntimeHandler,
	}

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
