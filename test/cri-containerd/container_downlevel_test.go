// +build functional

package cri_containerd

import (
	"testing"

	"github.com/Microsoft/hcsshim/osversion"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func Test_CreateContainer_DownLevel_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	testutilities.RequiresBuild(t, osversion.V19H1)

	pullRequiredImages(t, []string{imageWindowsNanoserver17763})

	sandboxRequest := &runtime.RunPodSandboxRequest{
		Config: &runtime.PodSandboxConfig{
			Metadata: &runtime.PodSandboxMetadata{
				Name:      t.Name() + "-Sandbox",
				Uid:       "0",
				Namespace: testNamespace,
			},
		},
		RuntimeHandler: wcowHypervisor17763RuntimeHandler,
	}

	request := &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: t.Name() + "-Container",
			},
			Image: &runtime.ImageSpec{
				Image: imageWindowsNanoserver17763,
			},
			// Hold this command open until killed
			Command: []string{
				"cmd",
				"/c", "ping", "-t", "127.0.0.1",
			},
		},
	}
	runCreateContainerTestWithSandbox(t, sandboxRequest, request)
}
