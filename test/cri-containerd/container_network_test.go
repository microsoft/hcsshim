// +build functional

package cri_containerd

import (
	"bufio"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

func createAndStartContainer(t *testing.T, sandboxRequest *runtime.RunPodSandboxRequest, request *runtime.CreateContainerRequest) {
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

	// wait a while for container to write to stdout
	time.Sleep(3 * time.Second)
}

func Test_Container_Network_LCOW(t *testing.T) {
	image := imageLcowAlpine

	//create a directory and log file
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("failed deleting temp dir: %v", err)
		}
	}()
	log := filepath.Join(dir, "ping.txt")

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
				"ping",
				"-q", //-q outputs ping stats only.
				"-c",
				"10",
				"google.com",
			},
			LogPath: log,
			Linux:   &runtime.LinuxContainerConfig{},
		},
	}

	createAndStartContainer(t, sandboxRequest, request)

	//open the log and test for any packet loss
	logFile, err := os.Open(log)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	s := bufio.NewScanner(logFile)
	for s.Scan() {
		v := strings.Fields(s.Text())
		t.Logf("ping output: %v", v)

		if v != nil && v[len(v)-1] == "loss" && v[len(v)-3] != "0%" {
			t.Fatalf("expected 0%% packet loss, got %v packet loss", v[len(v)-3])
		}
	}
}
