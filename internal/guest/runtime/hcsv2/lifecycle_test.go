//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"io"
	"strings"
	"testing"

	oci "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

type fakeConfidentialPolicyEnforcer struct {
	*securitypolicy.OpenDoorSecurityPolicyEnforcer
}

func (f *fakeConfidentialPolicyEnforcer) EncodedSecurityPolicy() string {
	return "test-policy"
}

func newConfidentialHostForTest() *Host {
	h := NewHost(nil, nil, &securitypolicy.OpenDoorSecurityPolicyEnforcer{}, io.Discard)
	h.securityOptions = securitypolicy.NewSecurityOptions(
		&fakeConfidentialPolicyEnforcer{OpenDoorSecurityPolicyEnforcer: &securitypolicy.OpenDoorSecurityPolicyEnforcer{}},
		false,
		"",
		"",
		io.Discard,
	)
	return h
}

func newTestContainer(id, rootfs string, st containerStatus) *Container {
	c := &Container{
		id:   id,
		spec: &oci.Spec{Root: &oci.Root{Path: rootfs}},
	}
	c.setStatus(st)
	return c
}

func TestGetInitializedContainer_StateGating(t *testing.T) {
	h := &Host{containers: map[string]*Container{}}

	for _, tc := range []struct {
		name      string
		status    containerStatus
		wantError bool
	}{
		{name: "creating denied", status: containerCreating, wantError: true},
		{name: "created allowed", status: containerCreated, wantError: false},
		{name: "running allowed", status: containerRunning, wantError: false},
		{name: "terminated allowed", status: containerTerminated, wantError: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := "cid-" + tc.name
			h.containers = map[string]*Container{id: newTestContainer(id, "/rootfs", tc.status)}

			got, err := h.GetInitializedContainer(id)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error for status %v", tc.status)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for status %v: %v", tc.status, err)
			}
			if got == nil {
				t.Fatalf("expected non-nil container for status %v", tc.status)
			}
		})
	}
}

func TestIsOverlayInUse_OnlyRunningContainersCount(t *testing.T) {
	overlay := "/run/gcs/c/abc/rootfs"

	h := &Host{
		containers: map[string]*Container{
			"created":    newTestContainer("created", overlay, containerCreated),
			"terminated": newTestContainer("terminated", overlay, containerTerminated),
			"other":      newTestContainer("other", "/run/gcs/c/xyz/rootfs", containerRunning),
		},
	}

	if h.IsOverlayInUse(overlay) {
		t.Fatalf("expected overlay %q not in use when no running container uses it", overlay)
	}

	h.containers["running"] = newTestContainer("running", overlay, containerRunning)
	if !h.IsOverlayInUse(overlay) {
		t.Fatalf("expected overlay %q in use when running container uses it", overlay)
	}
}

func TestDeleteContainerState_DeniesRunningContainerInConfidentialMode(t *testing.T) {
	h := newConfidentialHostForTest()
	id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	h.containers[id] = newTestContainer(id, "/run/gcs/c/abc/rootfs", containerRunning)

	err := h.DeleteContainerState(context.Background(), id)
	if err == nil {
		t.Fatal("expected error deleting running container state")
	}
	if !strings.Contains(err.Error(), "Denied deleting state of a running container") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := h.containers[id]; !ok {
		t.Fatalf("container %q should remain registered on early deny", id)
	}
}

func TestDeleteContainerState_DeniesIfOverlayStillMountedInConfidentialMode(t *testing.T) {
	h := newConfidentialHostForTest()
	h.hostMounts = newHostMounts()

	id := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	overlay := "/run/gcs/c/def/rootfs"
	h.containers[id] = newTestContainer(id, overlay, containerCreated)

	h.hostMounts.Lock()
	if err := h.hostMounts.AddOverlay(overlay, nil, ""); err != nil {
		h.hostMounts.Unlock()
		t.Fatalf("failed to seed overlay mount: %v", err)
	}
	h.hostMounts.Unlock()

	err := h.DeleteContainerState(context.Background(), id)
	if err == nil {
		t.Fatal("expected error deleting state with active overlay")
	}
	if !strings.Contains(err.Error(), "overlay mount still active") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := h.containers[id]; !ok {
		t.Fatalf("container %q should remain registered on early deny", id)
	}
}
