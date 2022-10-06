//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/test/internal/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// returns a request config for creating a template sandbox
func getTemplatePodConfig(name string) *runtime.RunPodSandboxRequest {
	return &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      name,
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				annotations.SaveAsTemplate: "true",
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
}

// returns a request config for creating a template container
func getTemplateContainerConfig(name string) *runtime.CreateContainerRequest {
	return &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: name,
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Do not keep the ping running on template containers.
			Command: []string{
				"ping",
				"127.0.0.1",
			},
			Annotations: map[string]string{
				annotations.SaveAsTemplate: "true",
			},
		},
	}
}

// returns a request config for creating a standard container
func getStandardContainerConfig(name string) *runtime.CreateContainerRequest {
	return &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: name,
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			Command: []string{
				"ping",
				"-t",
				"127.0.0.1",
			},
		},
	}
}

// returns a create cloned sandbox request config.
func getClonedPodConfig(uniqueID int, templateid string) *runtime.RunPodSandboxRequest {
	return &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      fmt.Sprintf("clonedpod-%d", uniqueID),
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				annotations.TemplateID: templateid + "@vm",
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
}

// returns a create cloned container request config.
func getClonedContainerConfig(uniqueID int, templateid string) *runtime.CreateContainerRequest {
	return &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: fmt.Sprintf("clonedcontainer-%d", uniqueID),
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Command for cloned containers
			Command: []string{
				"ping",
				"-t",
				"127.0.0.1",
			},
			Annotations: map[string]string{
				annotations.TemplateID: templateid,
			},
		},
	}
}

func waitForTemplateSave(ctx context.Context, t *testing.T, templatePodID string) {
	t.Helper()
	app := "hcsdiag"
	arg0 := "list"
	for {
		cmd := exec.Command(app, arg0)
		stdout, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed while waiting for save template to finish: %s", err)
		}
		if strings.Contains(string(stdout), templatePodID) && strings.Contains(string(stdout), "SavedAsTemplate") {
			break
		}
		timer := time.NewTimer(time.Millisecond * 100)
		select {
		case <-ctx.Done():
			t.Fatalf("Timelimit exceeded for wait for template saving to finish")
		case <-timer.C:
		}
	}
}

func createPodAndContainer(ctx context.Context, t *testing.T, client runtime.RuntimeServiceClient, sandboxRequest *runtime.RunPodSandboxRequest, containerRequest *runtime.CreateContainerRequest, podID, containerID *string) {
	t.Helper()
	// This is required in order to avoid leaking a pod and/or container in case somethings fails
	// during container creation of startup. The podID (and containerID if container creation was
	// successful) will be set to correct values so that the caller can correctly cleanup them even
	// in case of a failure.
	*podID = ""
	*containerID = ""
	*podID = runPodSandbox(t, client, ctx, sandboxRequest)
	containerRequest.PodSandboxId = *podID
	containerRequest.SandboxConfig = sandboxRequest.Config
	*containerID = createContainer(t, client, ctx, containerRequest)
	startContainer(t, client, ctx, *containerID)
}

// Creates a template sandbox and then a template container inside it.
// Since, template container can take time to finish the init process and then exit (at which
// point it will actually be saved as a template) this function wait until the template is
// actually saved.
// It is the callers responsibility to clean the stop and remove the cloned
// containers and pods.
func createTemplateContainer(
	ctx context.Context,
	t *testing.T,
	client runtime.RuntimeServiceClient,
	templateSandboxRequest *runtime.RunPodSandboxRequest,
	templateContainerRequest *runtime.CreateContainerRequest,
	templatePodID, templateContainerID *string,
) {
	t.Helper()
	createPodAndContainer(ctx, t, client, templateSandboxRequest, templateContainerRequest, templatePodID, templateContainerID)

	// Send a context with deadline for waitForTemplateSave function
	d := time.Now().Add(10 * time.Second)
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()
	waitForTemplateSave(ctx, t, *templatePodID)
}

// Creates a clone from the given template pod and container.
// It is the callers responsibility to clean the stop and remove the cloned
// containers and pods.
func createClonedContainer(
	ctx context.Context,
	t *testing.T,
	client runtime.RuntimeServiceClient,
	templatePodID, templateContainerID string,
	cloneNumber int,
	clonedPodID, clonedContainerID *string,
) {
	t.Helper()
	cloneSandboxRequest := getClonedPodConfig(cloneNumber, templatePodID)
	cloneContainerRequest := getClonedContainerConfig(cloneNumber, templateContainerID)
	createPodAndContainer(ctx, t, client, cloneSandboxRequest, cloneContainerRequest, clonedPodID, clonedContainerID)
}

// Runs a command inside given container and verifies if the command executes successfully.
func verifyContainerExec(ctx context.Context, t *testing.T, client runtime.RuntimeServiceClient, containerID string) {
	t.Helper()
	execCommand := []string{
		"ping",
		"127.0.0.1",
	}

	execRequest := &runtime.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         execCommand,
		Timeout:     20,
	}

	r := execSync(t, client, ctx, execRequest)
	output := strings.TrimSpace(string(r.Stdout))
	errorMsg := string(r.Stderr)
	exitCode := int(r.ExitCode)

	if exitCode != 0 || len(errorMsg) != 0 {
		t.Fatalf("Failed execution inside container %s with error: %s, exitCode: %d", containerID, errorMsg, exitCode)
	} else {
		t.Logf("Exec(container: %s) stdout: %s, stderr: %s, exitCode: %d\n", containerID, output, errorMsg, exitCode)
	}
}

func cleanupPod(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, podID *string) {
	t.Helper()
	if *podID == "" {
		// Do nothing for empty podID
		return
	}
	stopPodSandbox(t, client, ctx, *podID)
	removePodSandbox(t, client, ctx, *podID)
}

func cleanupContainer(t *testing.T, client runtime.RuntimeServiceClient, ctx context.Context, containerID *string) {
	t.Helper()
	if *containerID == "" {
		// Do nothing for empty containerID
		return
	}
	stopContainer(t, client, ctx, *containerID)
	removeContainer(t, client, ctx, *containerID)
}

// A simple test to just create a template container and then create one
// cloned container from that template.
func Test_CloneContainer_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	require.Build(t, osversion.V20H2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)
	var templatePodID, templateContainerID string
	var clonedPodID, clonedContainerID string

	// send pointers so that immediate evaluation of arguments doesn't evaluate to empty strings
	defer cleanupPod(t, client, ctx, &templatePodID)
	defer cleanupContainer(t, client, ctx, &templateContainerID)
	defer cleanupPod(t, client, ctx, &clonedPodID)
	defer cleanupContainer(t, client, ctx, &clonedContainerID)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	createTemplateContainer(ctx, t, client, getTemplatePodConfig("templatepod"), getTemplateContainerConfig("templatecontainer"), &templatePodID, &templateContainerID)
	requireContainerState(ctx, t, client, templateContainerID, runtime.ContainerState_CONTAINER_EXITED)
	createClonedContainer(ctx, t, client, templatePodID, templateContainerID, 1, &clonedPodID, &clonedContainerID)
	verifyContainerExec(ctx, t, client, clonedContainerID)
}

// A test for creating multiple clones(3 clones) from one template container.
func Test_MultiplClonedContainers_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	require.Build(t, osversion.V20H2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)
	nClones := 3
	var templatePodID, templateContainerID string
	clonedPodIDs := make([]string, nClones)
	clonedContainerIDs := make([]string, nClones)

	defer cleanupPod(t, client, ctx, &templatePodID)
	defer cleanupContainer(t, client, ctx, &templateContainerID)
	for i := 0; i < nClones; i++ {
		defer cleanupPod(t, client, ctx, &clonedPodIDs[i])
		defer cleanupContainer(t, client, ctx, &clonedContainerIDs[i])
	}

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	// create template pod & container
	createTemplateContainer(ctx, t, client, getTemplatePodConfig("templatepod"), getTemplateContainerConfig("templatecontainer"), &templatePodID, &templateContainerID)
	requireContainerState(ctx, t, client, templateContainerID, runtime.ContainerState_CONTAINER_EXITED)

	// create multiple clones
	for i := 0; i < nClones; i++ {
		createClonedContainer(ctx, t, client, templatePodID, templateContainerID, i, &clonedPodIDs[i], &clonedContainerIDs[i])
	}

	for i := 0; i < nClones; i++ {
		verifyContainerExec(ctx, t, client, clonedContainerIDs[i])
	}
}

// Test if a normal container can be created inside a clond pod alongside the cloned
// container.
func Test_NormalContainerInClonedPod_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	require.Build(t, osversion.V20H2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)
	var templatePodID, templateContainerID string
	var clonedPodID, clonedContainerID string
	var stdContainerID string
	pullRequiredImages(t, []string{imageWindowsNanoserver})

	defer cleanupPod(t, client, ctx, &templatePodID)
	defer cleanupContainer(t, client, ctx, &templateContainerID)
	defer cleanupPod(t, client, ctx, &clonedPodID)
	defer cleanupContainer(t, client, ctx, &clonedContainerID)
	defer cleanupContainer(t, client, ctx, &stdContainerID)

	// create template pod & container
	createTemplateContainer(ctx, t, client, getTemplatePodConfig("templatepod"), getTemplateContainerConfig("templatecontainer"), &templatePodID, &templateContainerID)
	requireContainerState(ctx, t, client, templateContainerID, runtime.ContainerState_CONTAINER_EXITED)

	// create a cloned pod and a cloned container
	cloneSandboxRequest := getClonedPodConfig(1, templatePodID)
	cloneContainerRequest := getClonedContainerConfig(1, templateContainerID)
	createPodAndContainer(ctx, t, client, cloneSandboxRequest, cloneContainerRequest, &clonedPodID, &clonedContainerID)

	// create a normal container in cloned pod
	stdContainerRequest := getStandardContainerConfig("standard-container")
	stdContainerRequest.PodSandboxId = clonedPodID
	stdContainerRequest.SandboxConfig = cloneSandboxRequest.Config
	stdContainerID = createContainer(t, client, ctx, stdContainerRequest)
	startContainer(t, client, ctx, stdContainerID)

	verifyContainerExec(ctx, t, client, clonedContainerID)
	verifyContainerExec(ctx, t, client, stdContainerID)
}

// A test for cloning multiple pods first and then cloning one container in each
// of those pods.
func Test_CloneContainersWithClonedPodPool_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	require.Build(t, osversion.V20H2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)
	nClones := 3
	var templatePodID, templateContainerID string
	clonedPodIDs := make([]string, nClones)
	clonedContainerIDs := make([]string, nClones)

	defer cleanupPod(t, client, ctx, &templatePodID)
	defer cleanupContainer(t, client, ctx, &templateContainerID)
	for i := 0; i < nClones; i++ {
		defer cleanupPod(t, client, ctx, &clonedPodIDs[i])
		defer cleanupContainer(t, client, ctx, &clonedContainerIDs[i])
	}

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	// create template pod & container
	createTemplateContainer(ctx, t, client, getTemplatePodConfig("templatepod"), getTemplateContainerConfig("templatecontainer"), &templatePodID, &templateContainerID)
	requireContainerState(ctx, t, client, templateContainerID, runtime.ContainerState_CONTAINER_EXITED)

	// create multiple pods
	clonedSandboxRequests := []*runtime.RunPodSandboxRequest{}
	for i := 0; i < nClones; i++ {
		cloneSandboxRequest := getClonedPodConfig(i, templatePodID)
		clonedPodIDs[i] = runPodSandbox(t, client, ctx, cloneSandboxRequest)
		clonedSandboxRequests = append(clonedSandboxRequests, cloneSandboxRequest)
	}

	// create multiple clones
	for i := 0; i < nClones; i++ {
		cloneContainerRequest := getClonedContainerConfig(i, templateContainerID)

		cloneContainerRequest.PodSandboxId = clonedPodIDs[i]
		cloneContainerRequest.SandboxConfig = clonedSandboxRequests[i].Config
		clonedContainerIDs[i] = createContainer(t, client, ctx, cloneContainerRequest)
		startContainer(t, client, ctx, clonedContainerIDs[i])
	}

	for i := 0; i < nClones; i++ {
		verifyContainerExec(ctx, t, client, clonedContainerIDs[i])
	}
}

func Test_ClonedContainerRunningAfterDeletingTemplate(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	require.Build(t, osversion.V20H2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)
	var templatePodID, templateContainerID string
	var clonedPodID, clonedContainerID string

	// send pointers so that immediate evaluation of arguments doesn't evaluate to empty strings
	defer cleanupPod(t, client, ctx, &templatePodID)
	defer cleanupContainer(t, client, ctx, &templateContainerID)
	defer cleanupPod(t, client, ctx, &clonedPodID)
	defer cleanupContainer(t, client, ctx, &clonedContainerID)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	createTemplateContainer(ctx, t, client, getTemplatePodConfig("templatepod"), getTemplateContainerConfig("templatecontainer"), &templatePodID, &templateContainerID)
	requireContainerState(ctx, t, client, templateContainerID, runtime.ContainerState_CONTAINER_EXITED)

	createClonedContainer(ctx, t, client, templatePodID, templateContainerID, 1, &clonedPodID, &clonedContainerID)

	stopPodSandbox(t, client, ctx, templatePodID)
	removePodSandbox(t, client, ctx, templatePodID)
	// Make sure cleanup function doesn't try to cleanup this deleted pod.
	templatePodID = ""
	templateContainerID = ""

	verifyContainerExec(ctx, t, client, clonedContainerID)
}

// A test to verify that multiple templates can be created and clones
// can be made from each of them simultaneously.
func Test_MultipleTemplateAndClones_WCOW(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	require.Build(t, osversion.V20H2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)
	nTemplates := 2
	var wg sync.WaitGroup
	templatePodIDs := make([]string, nTemplates)
	templateContainerIDs := make([]string, nTemplates)
	clonedPodIDs := make([]string, nTemplates)
	clonedContainerIDs := make([]string, nTemplates)

	for i := 0; i < nTemplates; i++ {
		defer cleanupPod(t, client, ctx, &templatePodIDs[i])
		defer cleanupContainer(t, client, ctx, &templateContainerIDs[i])
		defer cleanupPod(t, client, ctx, &clonedPodIDs[i])
		defer cleanupContainer(t, client, ctx, &clonedContainerIDs[i])
	}

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	wg.Add(nTemplates)
	for i := 0; i < nTemplates; i++ {
		go func(index int) {
			defer wg.Done()
			createTemplateContainer(ctx, t, client, getTemplatePodConfig(fmt.Sprintf("templatepod-%d", index)), getTemplateContainerConfig(fmt.Sprintf("templatecontainer-%d", index)), &templatePodIDs[index], &templateContainerIDs[index])
			requireContainerState(ctx, t, client, templateContainerIDs[index], runtime.ContainerState_CONTAINER_EXITED)
			createClonedContainer(ctx, t, client, templatePodIDs[index], templateContainerIDs[index], index, &clonedPodIDs[index], &clonedContainerIDs[index])
		}(i)
	}

	// Wait before all template & clone creations are done.
	wg.Wait()

	for i := 0; i < nTemplates; i++ {
		verifyContainerExec(ctx, t, client, clonedContainerIDs[i])
	}
}

// Tries to create a clone with a different memory config than the template
// and verifies that the request correctly fails with an error.
func Test_VerifyCloneAndTemplateConfig(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	require.Build(t, osversion.V20H2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)
	var templatePodID, templateContainerID string
	var clonedPodID string

	// send pointers so that immediate evaluation of arguments doesn't evaluate to empty strings
	defer cleanupPod(t, client, ctx, &templatePodID)
	defer cleanupContainer(t, client, ctx, &templateContainerID)
	defer cleanupPod(t, client, ctx, &clonedPodID)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	templatePodConfig := getTemplatePodConfig("templatepod")

	createTemplateContainer(ctx, t, client, templatePodConfig, getTemplateContainerConfig("templatecontainer"), &templatePodID, &templateContainerID)
	requireContainerState(ctx, t, client, templateContainerID, runtime.ContainerState_CONTAINER_EXITED)

	// change pod config to make sure the request fails
	cloneSandboxRequest := getClonedPodConfig(0, templatePodID)
	cloneSandboxRequest.Config.Annotations[annotations.AllowOvercommit] = "false"

	_, err := client.RunPodSandbox(ctx, cloneSandboxRequest)
	if err == nil {
		t.Fatalf("pod cloning should fail with mismatching configurations error")
	} else if !strings.Contains(err.Error(), "doesn't match") {
		t.Fatalf("Expected mismatching configurations error got: %s", err)
	}
}
