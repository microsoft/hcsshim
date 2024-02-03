//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/test/pkg/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// This test requires compiling a helper logging binary which can be found
// at test/cri-containerd/helpers/log.go. Copy log.exe as "sample-logging-driver.exe"
// to ContainerPlat install directory or set "TEST_BINARY_ROOT" environment variable,
// which this test will use to construct logPath for CreateContainerRequest and as
// the location of stdout artifacts created by the binary
func Test_Run_Container_With_Binary_Logger(t *testing.T) {
	requireAnyFeature(t, featureWCOWProcess, featureWCOWHypervisor)
	binaryPath := require.Binary(t, "sample-logging-driver.exe")

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logPath := "binary:///" + binaryPath

	type config struct {
		name             string
		containerName    string
		requiredFeatures []string
		runtimeHandler   string
		sandboxImage     string
		containerImage   string
		cmd              []string
		expectedContent  string
	}

	tests := []config{
		{
			name:             "WCOW_Process",
			containerName:    t.Name() + "-Container-WCOW_Process",
			requiredFeatures: []string{featureWCOWProcess},
			runtimeHandler:   wcowProcessRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			containerImage:   imageWindowsNanoserver,
			cmd:              []string{"ping", "-t", "127.0.0.1"},
			expectedContent:  "Pinging 127.0.0.1 with 32 bytes of data",
		},
		{
			name:             "WCOW_Hypervisor",
			containerName:    t.Name() + "-Container-WCOW_Hypervisor",
			requiredFeatures: []string{featureWCOWHypervisor},
			runtimeHandler:   wcowHypervisorRuntimeHandler,
			sandboxImage:     imageWindowsNanoserver,
			containerImage:   imageWindowsNanoserver,
			cmd:              []string{"ping", "-t", "127.0.0.1"},
			expectedContent:  "Pinging 127.0.0.1 with 32 bytes of data",
		},
	}

	// Positive tests
	for _, test := range tests {
		t.Run(test.name+"_Positive", func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)

			requiredImages := []string{test.sandboxImage, test.containerImage}
			pullRequiredImages(t, requiredImages)

			podReq := getRunPodSandboxRequest(t, test.runtimeHandler)
			podID := runPodSandbox(t, client, ctx, podReq)
			defer removePodSandbox(t, client, ctx, podID)

			logFileName := fmt.Sprintf(`%s\stdout-%s.txt`, filepath.Dir(binaryPath), test.name)
			conReq := getCreateContainerRequest(podID, test.containerName, test.containerImage, test.cmd, podReq.Config)
			conReq.Config.LogPath = logPath + fmt.Sprintf("?%s", logFileName)

			createAndRunContainer(t, client, ctx, conReq)

			if _, err := os.Stat(logFileName); os.IsNotExist(err) {
				t.Fatalf("log file was not created: %s", logFileName)
			}
			defer os.Remove(logFileName)

			ok, err := assertFileContent(logFileName, test.expectedContent)
			if err != nil {
				t.Fatalf("failed to read log file: %s", err)
			}

			if !ok {
				t.Fatalf("file content validation failed: %s", test.expectedContent)
			}
		})
	}

	// Negative tests
	for _, test := range tests {
		t.Run(test.name+"_Negative", func(t *testing.T) {
			requireFeatures(t, test.requiredFeatures...)

			requiredImages := []string{test.sandboxImage, test.containerImage}
			pullRequiredImages(t, requiredImages)

			podReq := getRunPodSandboxRequest(t, test.runtimeHandler)
			podID := runPodSandbox(t, client, ctx, podReq)
			defer removePodSandbox(t, client, ctx, podID)

			nonExistentPath := "/does/not/exist/log.txt"
			conReq := getCreateContainerRequest(podID, test.containerName, test.containerImage, test.cmd, podReq.Config)
			conReq.Config.LogPath = logPath + fmt.Sprintf("?%s", nonExistentPath)

			containerID := createContainer(t, client, ctx, conReq)
			defer removeContainer(t, client, ctx, containerID)

			// This should fail, since the filepath doesn't exist
			_, err := client.StartContainer(ctx, &runtime.StartContainerRequest{
				ContainerId: containerID,
			})
			if err == nil {
				t.Fatal("container start should fail")
			}

			if !strings.Contains(err.Error(), "failed to start binary logger") {
				t.Fatalf("expected 'failed to start binary logger' error, got: %s", err)
			}
		})
	}
}

func createAndRunContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, conReq *runtime.CreateContainerRequest) {
	t.Helper()
	containerID := createContainer(t, client, ctx, conReq)
	defer removeContainer(t, client, ctx, containerID)

	startContainer(t, client, ctx, containerID)
	defer stopContainer(t, client, ctx, containerID)

	// Let stdio kick in
	time.Sleep(time.Second * 1)
}

func assertFileContent(path string, content string) (bool, error) {
	fileContent, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	return strings.Contains(string(fileContent), content), nil
}
