//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/test/pkg/require"
)

var _containerBenchmarkTests = []struct {
	Name        string
	Feature     string
	Runtime     string
	SandboxOpts []SandboxConfigOpt
	Image       string
	Command     []string
}{
	{
		Name:    "WCOW_Hypervisor",
		Feature: featureWCOWHypervisor,
		Runtime: wcowHypervisorRuntimeHandler,
		Image:   imageWindowsNanoserver,
		Command: []string{"cmd", "/c", "ping -t 127.0.0.1"},
	},
	{
		Name:    "WCOW_Process",
		Feature: featureWCOWProcess,
		Runtime: wcowProcessRuntimeHandler,
		Image:   imageWindowsNanoserver,
		Command: []string{"cmd", "/c", "ping -t 127.0.0.1"},
	},
}

func BenchmarkPodCreate(b *testing.B) {
	require.Build(b, osversion.RS5)
	client := newTestRuntimeClient(b)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tt := range _containerBenchmarkTests {
		b.StopTimer()
		b.ResetTimer()
		b.Run(tt.Name, func(b *testing.B) {
			requireFeatures(b, tt.Feature)

			switch tt.Feature {
			case featureWCOWHypervisor, featureWCOWProcess:
				pullRequiredImages(b, []string{tt.Image})
			}

			sandboxRequest := getRunPodSandboxRequest(b, tt.Runtime, tt.SandboxOpts...)

			for i := 0; i < b.N; i++ {
				b.StartTimer()
				podID := runPodSandbox(b, client, ctx, sandboxRequest)
				b.StopTimer()

				stopPodSandbox(b, client, ctx, podID)
				removePodSandbox(b, client, ctx, podID)
			}
		})
	}
}

func BenchmarkContainerCreate(b *testing.B) {
	require.Build(b, osversion.RS5)
	client := newTestRuntimeClient(b)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//
	// new pod per container
	//
	b.Run("NewPod", func(b *testing.B) {
		for _, tt := range _containerBenchmarkTests {
			b.StopTimer()
			b.ResetTimer()
			b.Run(tt.Name, func(b *testing.B) {
				requireFeatures(b, tt.Feature)

				switch tt.Feature {
				case featureWCOWHypervisor, featureWCOWProcess:
					pullRequiredImages(b, []string{tt.Image})
				}

				sandboxRequest := getRunPodSandboxRequest(b, tt.Runtime, tt.SandboxOpts...)
				for i := 0; i < b.N; i++ {
					podID := runPodSandbox(b, client, ctx, sandboxRequest)

					request := getCreateContainerRequest(podID, b.Name()+"-Container", tt.Image, tt.Command, sandboxRequest.Config)

					b.StartTimer()
					containerID := createContainer(b, client, ctx, request)
					b.StopTimer()

					removeContainer(b, client, ctx, containerID)
					stopPodSandbox(b, client, ctx, podID)
					removePodSandbox(b, client, ctx, podID)
				}
			})
		}
	})

	//
	// same pod for containers
	//
	b.Run("SamePod", func(b *testing.B) {
		for _, tt := range _containerBenchmarkTests {
			b.StopTimer()
			b.ResetTimer()
			b.Run(tt.Name, func(b *testing.B) {
				requireFeatures(b, tt.Feature)

				switch tt.Feature {
				case featureWCOWHypervisor, featureWCOWProcess:
					pullRequiredImages(b, []string{tt.Image})
				}
				sandboxRequest := getRunPodSandboxRequest(b, tt.Runtime, tt.SandboxOpts...)
				podID := runPodSandbox(b, client, ctx, sandboxRequest)

				for i := 0; i < b.N; i++ {
					request := getCreateContainerRequest(podID, b.Name()+"-Container", tt.Image, tt.Command, sandboxRequest.Config)

					b.StartTimer()
					containerID := createContainer(b, client, ctx, request)
					b.StopTimer()

					removeContainer(b, client, ctx, containerID)
				}

				stopPodSandbox(b, client, ctx, podID)
				removePodSandbox(b, client, ctx, podID)
			})
		}
	})
}

func BenchmarkContainerStart(b *testing.B) {
	require.Build(b, osversion.RS5)
	client := newTestRuntimeClient(b)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//
	// new pod per container
	//
	b.Run("NewPod", func(b *testing.B) {
		for _, tt := range _containerBenchmarkTests {
			b.StopTimer()
			b.ResetTimer()
			b.Run(tt.Name, func(b *testing.B) {
				requireFeatures(b, tt.Feature)

				switch tt.Feature {
				case featureWCOWHypervisor, featureWCOWProcess:
					pullRequiredImages(b, []string{tt.Image})
				}

				sandboxRequest := getRunPodSandboxRequest(b, tt.Runtime, tt.SandboxOpts...)
				for i := 0; i < b.N; i++ {
					podID := runPodSandbox(b, client, ctx, sandboxRequest)

					request := getCreateContainerRequest(podID, b.Name()+"-Container", tt.Image, tt.Command, sandboxRequest.Config)
					containerID := createContainer(b, client, ctx, request)

					b.StartTimer()
					startContainer(b, client, ctx, containerID)
					b.StopTimer()

					stopContainer(b, client, ctx, containerID)
					removeContainer(b, client, ctx, containerID)
					stopPodSandbox(b, client, ctx, podID)
					removePodSandbox(b, client, ctx, podID)
				}
			})
		}
	})

	//
	// same pod for containers
	//
	b.Run("SamePod", func(b *testing.B) {
		for _, tt := range _containerBenchmarkTests {
			b.StopTimer()
			b.ResetTimer()
			b.Run(tt.Name, func(b *testing.B) {
				requireFeatures(b, tt.Feature)

				switch tt.Feature {
				case featureWCOWHypervisor, featureWCOWProcess:
					pullRequiredImages(b, []string{tt.Image})
				}
				sandboxRequest := getRunPodSandboxRequest(b, tt.Runtime, tt.SandboxOpts...)
				podID := runPodSandbox(b, client, ctx, sandboxRequest)

				for i := 0; i < b.N; i++ {
					request := getCreateContainerRequest(podID, b.Name()+"-Container", tt.Image, tt.Command, sandboxRequest.Config)
					containerID := createContainer(b, client, ctx, request)

					b.StartTimer()
					startContainer(b, client, ctx, containerID)
					b.StopTimer()

					stopContainer(b, client, ctx, containerID)
					removeContainer(b, client, ctx, containerID)
				}

				stopPodSandbox(b, client, ctx, podID)
				removePodSandbox(b, client, ctx, podID)
			})
		}
	})
}
