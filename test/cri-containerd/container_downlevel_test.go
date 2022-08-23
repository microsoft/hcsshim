//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"testing"

	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/test/internal/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func Test_CreateContainer_DownLevel_WCOW_Hypervisor(t *testing.T) {
	requireFeatures(t, featureWCOWHypervisor)
	require.Build(t, osversion.V19H1)

	pullRequiredImages(t, []string{imageWindowsNanoserver17763})

	sandboxRequest := getRunPodSandboxRequest(t, wcowHypervisor17763RuntimeHandler)

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
