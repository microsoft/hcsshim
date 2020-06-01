// +build functional

package cri_containerd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// Possible tests for cloning
// 1. Create a template pod, create template container inside it. Kill it without cloning.
// 2. Create a template pod, don't create template container. Just create a normal container
// and see if we can connect to that container.
// 3. Create a template pod and template container and clone it. Verify if the process
// started in template container is still running in cloned container.
// 4. Create a template pod & container and clone it then stop and delete the template pod, verify if the cloned container is still running well and storage for template pod is removed.
// 5. Create a cloned pod & container, create another container inside the cloned pod, remove
// the cloned container from that pod and verify if it still works.
// 6. Verify if networking works correctly in the cloned container.
// 7. Verify if files created inside the template container are visible in the cloned container.
// 8. Create a template pod, create a normal container inside it then create another
// template container inside it, then save the pod as a template and see what happens.
// 9. Create a template pod and verify that execInHost works in that pod before it is
// saved as a template
// 10. verify that the information is correctly removed from the registry after the template
// is removed.

// returns a request config for creating a template sandbox
// Name field is empty so it must be filled before actually sending this request.
func getTemplatePodConfig() *runtime.RunPodSandboxRequest {
	return &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      "",
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.saveastemplate": "true",
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
}

// returns a request config for creating a template container
// Name field is empty so it must be filled before actually sending this request.
func getTemplateContainerConfig() *runtime.CreateContainerRequest {
	return &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: "",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Do not keep the ping running on template containers.
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"127.0.0.1",
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.saveastemplate": "true",
			},
		},
	}
}

// returns a request config for creating a standard container
// Name field is empty so it must be filled before actually sending this request.
func getStandardContainerConfig() *runtime.CreateContainerRequest {
	return &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: "",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
		},
	}
}

// returns a create cloned sandbox request config. This config can not be used
// as it is because `Name` and `Annotation: templateid` fields are empty
func getClonedPodConfig() *runtime.RunPodSandboxRequest {
	return &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      "",
				Uid:       "0",
				Namespace: testNamespace,
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.templateid": "",
			},
		},
		RuntimeHandler: wcowHypervisorRuntimeHandler,
	}
}

// returns a create cloned container request config. This config can not be used
// as it is because `Name` and `Annotation: templateid` fields are empty
func getClonedContainerConfig() *runtime.CreateContainerRequest {
	return &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: "",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver,
			},
			// Command for cloned containers
			Command: []string{
				"cmd",
				"/c",
				"ping",
				"-t",
				"127.0.0.1",
			},
			Annotations: map[string]string{
				"io.microsoft.virtualmachine.templateid": "",
			},
		},
	}
}

func waitForTemplateSave(ctx context.Context, t *testing.T, templatePodID string) {
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

func createPodAndContainer(ctx context.Context, t *testing.T, client runtime.RuntimeServiceClient, sandboxRequest *runtime.RunPodSandboxRequest, containerRequest *runtime.CreateContainerRequest) (podID, containerID string) {
	podID = runPodSandbox(t, client, ctx, sandboxRequest)
	containerRequest.PodSandboxId = podID
	containerRequest.SandboxConfig = sandboxRequest.Config
	containerID = createContainer(t, client, ctx, containerRequest)
	startContainer(t, client, ctx, containerID)
	return podID, containerID
}

// Creates a template sandbox and then a template container inside it.
// Since, template container can take time to finish the init process and then exit (at which
// point it will actually be saved as a template) this function wait until the template is
// actually saved.
// It is the callers responsibility to clean the stop and remove the cloned
// containers and pods.
func createTemplateContainer(ctx context.Context, t *testing.T, client runtime.RuntimeServiceClient, templateSandboxRequest *runtime.RunPodSandboxRequest, templateContainerRequest *runtime.CreateContainerRequest) (templatePodID, templateContainerID string) {
	templatePodID, templateContainerID = createPodAndContainer(ctx, t, client, templateSandboxRequest, templateContainerRequest)

	// Send a context with deadline for waitForTemplateSave function
	d := time.Now().Add(10 * time.Second)
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()
	waitForTemplateSave(ctx, t, templatePodID)
	return
}

// Creates a clone from the given template pod and container.
// It is the callers responsibility to clean the stop and remove the cloned
// containers and pods.
func createClonedContainer(ctx context.Context, t *testing.T, client runtime.RuntimeServiceClient, templatePodID, templateContainerID string, cloneNumber int) (clonedPodID, clonedContainerID string) {

	cloneSandboxRequest := getClonedPodConfig()
	cloneSandboxRequest.Config.Metadata.Name = fmt.Sprintf("clonedPod-%d", cloneNumber)
	cloneSandboxRequest.Config.Annotations["io.microsoft.virtualmachine.templateid"] = templatePodID + "@vm"

	cloneContainerRequest := getClonedContainerConfig()
	cloneContainerRequest.Config.Metadata.Name = fmt.Sprintf("clonedContainer-%d", cloneNumber)
	cloneContainerRequest.Config.Annotations["io.microsoft.virtualmachine.templateid"] = templateContainerID
	clonedPodID, clonedContainerID = createPodAndContainer(ctx, t, client, cloneSandboxRequest, cloneContainerRequest)
	return
}

// Runs a command inside given container and verifies if the command executes successfully.
func verifyContainerExec(ctx context.Context, t *testing.T, client runtime.RuntimeServiceClient, containerID string) {
	execCommand := []string{
		"ping",
		"www.microsoft.com",
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
		t.Logf("Exec stdout: %s, stderr: %s, exitCode: %d\n", output, errorMsg, exitCode)
	}
}

// A simple test to just create a template container and then create one
// cloned container from that template.
func Test_CloneContainer_WCOW(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	templatePodID, templateContainerID := createTemplateContainer(ctx, t, client, getTemplatePodConfig(), getTemplateContainerConfig())
	defer removePodSandbox(t, client, ctx, templatePodID)
	defer stopPodSandbox(t, client, ctx, templatePodID)
	defer removeContainer(t, client, ctx, templateContainerID)
	defer stopContainer(t, client, ctx, templateContainerID)

	clonedPodID, clonedContainerID := createClonedContainer(ctx, t, client, templatePodID, templateContainerID, 1)
	defer removePodSandbox(t, client, ctx, clonedPodID)
	defer stopPodSandbox(t, client, ctx, clonedPodID)
	defer removeContainer(t, client, ctx, clonedContainerID)
	defer stopContainer(t, client, ctx, clonedContainerID)

	verifyContainerExec(ctx, t, client, clonedContainerID)
}

// A test for creating multiple clones(5 clones) from one template container.
func Test_MultiplClonedContainers_WCOW(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)
	nClones := 3

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	// create template pod & container
	templatePodID, templateContainerID := createTemplateContainer(ctx, t, client, getTemplatePodConfig(), getTemplateContainerConfig())
	defer removePodSandbox(t, client, ctx, templatePodID)
	defer stopPodSandbox(t, client, ctx, templatePodID)
	defer removeContainer(t, client, ctx, templateContainerID)
	defer stopContainer(t, client, ctx, templateContainerID)

	// create multiple clones
	clonedContainers := []string{}
	for i := 0; i < nClones; i++ {
		clonedPodID, clonedContainerID := createClonedContainer(ctx, t, client, templatePodID, templateContainerID, i)
		// cleanup
		defer removePodSandbox(t, client, ctx, clonedPodID)
		defer stopPodSandbox(t, client, ctx, clonedPodID)
		defer removeContainer(t, client, ctx, clonedContainerID)
		defer stopContainer(t, client, ctx, clonedContainerID)
		clonedContainers = append(clonedContainers, clonedContainerID)
	}

	for i := 0; i < nClones; i++ {
		verifyContainerExec(ctx, t, client, clonedContainers[i])
	}
}

// Test if a normal container can be created inside a clond pod alongside the cloned
// container.
// TODO(ambarve): This doesn't work as of now. Enable this test when the bug is fixed.
func DisabledTest_NormalContainerInClonedPod_WCOW(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)

	// create template pod & container
	templatePodID, templateContainerID := createTemplateContainer(ctx, t, client, getTemplatePodConfig(), getTemplateContainerConfig())
	defer removePodSandbox(t, client, ctx, templatePodID)
	defer stopPodSandbox(t, client, ctx, templatePodID)
	defer removeContainer(t, client, ctx, templateContainerID)
	defer stopContainer(t, client, ctx, templateContainerID)

	// create a cloned pod and a cloned container
	cloneSandboxRequest := getClonedPodConfig()
	cloneSandboxRequest.Config.Metadata.Name = fmt.Sprintf("clonedPod-%d", 1)
	cloneSandboxRequest.Config.Annotations["io.microsoft.virtualmachine.templateid"] = templatePodID + "@vm"
	cloneContainerRequest := getClonedContainerConfig()
	cloneContainerRequest.Config.Metadata.Name = fmt.Sprintf("clonedContainer-%d", 1)
	cloneContainerRequest.Config.Annotations["io.microsoft.virtualmachine.templateid"] = templateContainerID
	clonedPodID, clonedContainerID := createPodAndContainer(ctx, t, client, cloneSandboxRequest, cloneContainerRequest)
	defer removePodSandbox(t, client, ctx, clonedPodID)
	defer stopPodSandbox(t, client, ctx, clonedPodID)
	defer removeContainer(t, client, ctx, clonedContainerID)
	defer stopContainer(t, client, ctx, clonedContainerID)

	// create a normal container in cloned pod
	stdContainerRequest := getStandardContainerConfig()
	stdContainerRequest.PodSandboxId = clonedPodID
	stdContainerRequest.SandboxConfig = cloneSandboxRequest.Config
	stdContainerID := createContainer(t, client, ctx, stdContainerRequest)
	startContainer(t, client, ctx, stdContainerID)
	defer removeContainer(t, client, ctx, stdContainerID)
	defer stopContainer(t, client, ctx, stdContainerID)

	verifyContainerExec(ctx, t, client, clonedContainerID)
	verifyContainerExec(ctx, t, client, stdContainerID)
}

// A test for cloning multiple pods first and then cloning one container in each
// of those pods.
func Test_CloneContainersWithClonedPodPool_WCOW(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)
	nClones := 3

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	// create template pod & container
	templatePodID, templateContainerID := createTemplateContainer(ctx, t, client, getTemplatePodConfig(), getTemplateContainerConfig())
	defer removePodSandbox(t, client, ctx, templatePodID)
	defer stopPodSandbox(t, client, ctx, templatePodID)
	defer removeContainer(t, client, ctx, templateContainerID)
	defer stopContainer(t, client, ctx, templateContainerID)

	// create multiple pods
	clonedPodIDs := []string{}
	clonedSandboxRequests := []*runtime.RunPodSandboxRequest{}
	for i := 0; i < nClones; i++ {
		cloneSandboxRequest := getClonedPodConfig()
		cloneSandboxRequest.Config.Metadata.Name = fmt.Sprintf("clonedPod-%d", i)
		cloneSandboxRequest.Config.Annotations["io.microsoft.virtualmachine.templateid"] = templatePodID + "@vm"
		clonedPodID := runPodSandbox(t, client, ctx, cloneSandboxRequest)
		clonedPodIDs = append(clonedPodIDs, clonedPodID)
		clonedSandboxRequests = append(clonedSandboxRequests, cloneSandboxRequest)
		defer removePodSandbox(t, client, ctx, clonedPodID)
		defer stopPodSandbox(t, client, ctx, clonedPodID)
	}

	// create multiple clones
	clonedContainers := []string{}
	for i := 0; i < nClones; i++ {
		cloneContainerRequest := getClonedContainerConfig()
		cloneContainerRequest.Config.Metadata.Name = fmt.Sprintf("clonedContainer-%d", i)
		cloneContainerRequest.Config.Annotations["io.microsoft.virtualmachine.templateid"] = templateContainerID

		cloneContainerRequest.PodSandboxId = clonedPodIDs[i]
		cloneContainerRequest.SandboxConfig = clonedSandboxRequests[i].Config
		clonedContainerID := createContainer(t, client, ctx, cloneContainerRequest)
		startContainer(t, client, ctx, clonedContainerID)

		// cleanup
		defer removeContainer(t, client, ctx, clonedContainerID)
		defer stopContainer(t, client, ctx, clonedContainerID)

		clonedContainers = append(clonedContainers, clonedContainerID)
	}

	for i := 0; i < nClones; i++ {
		verifyContainerExec(ctx, t, client, clonedContainers[i])
	}
}

func Test_ClonedContainerRunningAfterDeletingTemplate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestRuntimeClient(t)

	pullRequiredImages(t, []string{imageWindowsNanoserver})

	templatePodID, templateContainerID := createTemplateContainer(ctx, t, client, getTemplatePodConfig(), getTemplateContainerConfig())

	clonedPodID, clonedContainerID := createClonedContainer(ctx, t, client, templatePodID, templateContainerID, 1)
	defer removePodSandbox(t, client, ctx, clonedPodID)
	defer stopPodSandbox(t, client, ctx, clonedPodID)
	defer removeContainer(t, client, ctx, clonedContainerID)
	defer stopContainer(t, client, ctx, clonedContainerID)

	stopPodSandbox(t, client, ctx, templatePodID)
	removePodSandbox(t, client, ctx, templatePodID)
	// TODO verify if template information is removed from the registry.

	verifyContainerExec(ctx, t, client, clonedContainerID)

}
