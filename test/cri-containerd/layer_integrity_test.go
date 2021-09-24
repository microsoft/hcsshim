// +build functional

package cri_containerd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/oci"
)

func Test_LCOW_Layer_Integrity(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity, featureLCOW)

	client := newTestRuntimeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pullRequiredLCOWImages(t, []string{imageLcowK8sPause})

	// Delete container image in case it already exists.
	removeImages(t, []string{imageLcowAlpine})

	// Pull image with dm-verity enabled.
	pullRequiredLCOWImages(
		t,
		[]string{imageLcowAlpine},
		WithSandboxAnnotations(map[string]string{
			"containerd.io/diff/io.microsoft.storage.lcow.append-dm-verity": "true",
		}),
	)

	type config struct {
		layerType  string
		vPMemCount int
		rootFSType string
	}

	for _, scenario := range []config{
		{
			layerType:  "scsi",
			vPMemCount: 0,
			rootFSType: "initrd",
		},
		{
			layerType:  "pmem",
			vPMemCount: 16,
			rootFSType: "initrd",
		},
		{
			layerType:  "pmem",
			vPMemCount: 16,
			rootFSType: "vhd",
		},
	} {
		t.Run(fmt.Sprintf("Integrity-For-%s", scenario.layerType), func(t *testing.T) {
			podReq := getRunPodSandboxRequest(
				t,
				lcowRuntimeHandler,
				WithSandboxAnnotations(map[string]string{
					oci.AnnotationVPMemCount:          strconv.Itoa(scenario.vPMemCount),
					oci.AnnotationPreferredRootFSType: scenario.rootFSType,
				}),
			)
			podID := runPodSandbox(t, client, ctx, podReq)
			defer removePodSandbox(t, client, ctx, podID)

			// Launch container
			cmd := []string{"ash", "-c", "while true; do sleep 1; done"}
			contReq := getCreateContainerRequest(
				podID,
				fmt.Sprintf("alpine-%s", scenario.layerType),
				imageLcowAlpine,
				cmd,
				podReq.Config,
			)
			contID := createContainer(t, client, ctx, contReq)
			defer removeContainer(t, client, ctx, contID)
			startContainer(t, client, ctx, contID)
			defer stopContainer(t, client, ctx, contID)

			// Validate that verity target(s) present
			output := shimDiagExecOutput(ctx, t, podID, []string{"ls", "-l", "/dev/mapper"})
			filtered := filterStrings(strings.Split(output, "\n"), fmt.Sprintf("dm-verity-%s", scenario.layerType))
			if len(filtered) == 0 {
				t.Fatalf("expected verity targets for %s devices, none found.\n%s\n", scenario.layerType, output)
			}
		})
	}
}
