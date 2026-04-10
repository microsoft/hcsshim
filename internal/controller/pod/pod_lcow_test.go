//go:build windows && lcow

package pod

import (
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/internal/controller/linuxcontainer"
	"github.com/Microsoft/hcsshim/internal/controller/network"
	"github.com/Microsoft/hcsshim/internal/controller/pod/mocks"
)

const testPodID = "test-pod-1234"

// errTest is a sentinel used in table-driven tests to verify error propagation.
var errTest = errors.New("test error")

// newSetup creates a gomock controller, vm mock, network mock, and a pod
// Controller wired together. Mock expectations are verified automatically via
// t.Cleanup.
func newSetup(t *testing.T) (*mocks.MockvmController, *mocks.MocknetworkController, *Controller) {
	t.Helper()
	mc := gomock.NewController(t)
	vm := mocks.NewMockvmController(mc)
	net := mocks.NewMocknetworkController(mc)
	return vm, net, &Controller{
		podID:      testPodID,
		gcsPodID:   testPodID,
		vm:         vm,
		network:    net,
		containers: make(map[string]*linuxcontainer.Controller),
	}
}

// expectVMCallsForNewContainer sets up the mock expectations on vmController
// that NewContainer triggers when constructing a linuxcontainer.Controller.
func expectVMCallsForNewContainer(vm *mocks.MockvmController) {
	vm.EXPECT().RuntimeID().Return("vm-runtime-1")
	vm.EXPECT().Guest().Return(nil)
	vm.EXPECT().SCSIController().Return(nil)
	vm.EXPECT().Plan9Controller().Return(nil)
	vm.EXPECT().VPCIController().Return(nil)
}

// TestNew_InitializesFields verifies that New sets up all fields correctly and
// that the containers map is non-nil and empty.
func TestNew_InitializesFields(t *testing.T) {
	mc := gomock.NewController(t)
	vm := mocks.NewMockvmController(mc)
	vm.EXPECT().NetworkController().Return(nil)

	c := New(testPodID, vm)

	if c.podID != testPodID {
		t.Errorf("expected podID=%q, got %q", testPodID, c.podID)
	}
	if c.gcsPodID != testPodID {
		t.Errorf("expected gcsPodID=%q, got %q", testPodID, c.gcsPodID)
	}
	if c.containers == nil {
		t.Fatal("expected non-nil containers map")
	}
	if len(c.containers) != 0 {
		t.Errorf("expected empty containers map, got %d entries", len(c.containers))
	}
}

// TestSetupNetwork verifies that SetupNetwork delegates to the network controller.
func TestSetupNetwork(t *testing.T) {
	opts := &network.SetupOptions{NetworkNamespace: "ns-1234"}

	tests := []struct {
		name   string
		retErr error
	}{
		{"happy path", nil},
		{"fails", errTest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, net, c := newSetup(t)
			net.EXPECT().Setup(gomock.Any(), opts).Return(tt.retErr)

			err := c.SetupNetwork(t.Context(), opts)
			if !errors.Is(err, tt.retErr) {
				t.Errorf("SetupNetwork() error = %v, wantErr %v", err, tt.retErr)
			}
		})
	}
}

// TestTeardownNetwork verifies that TeardownNetwork delegates to the network controller.
func TestTeardownNetwork(t *testing.T) {
	tests := []struct {
		name   string
		retErr error
	}{
		{"happy path", nil},
		{"fails", errTest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, net, c := newSetup(t)
			net.EXPECT().Teardown(gomock.Any()).Return(tt.retErr)

			err := c.TeardownNetwork(t.Context())
			if !errors.Is(err, tt.retErr) {
				t.Errorf("TeardownNetwork() error = %v, wantErr %v", err, tt.retErr)
			}
		})
	}
}

// TestGetContainer verifies retrieval of registered and unknown containers.
func TestGetContainer(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		vm, _, c := newSetup(t)
		expectVMCallsForNewContainer(vm)

		created, err := c.NewContainer(t.Context(), "container-1")
		if err != nil {
			t.Fatalf("NewContainer: %v", err)
		}

		got, err := c.GetContainer("container-1")
		if err != nil {
			t.Fatalf("GetContainer: %v", err)
		}
		if got != created {
			t.Error("GetContainer returned a different controller than NewContainer")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, _, c := newSetup(t)
		if _, err := c.GetContainer("nonexistent"); err == nil {
			t.Fatal("expected error for unknown container ID")
		}
	})
}

// TestNewContainer verifies creating containers with new, duplicate, and multiple IDs.
func TestNewContainer(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		vm, _, c := newSetup(t)
		expectVMCallsForNewContainer(vm)

		containerCtrl, err := c.NewContainer(t.Context(), "container-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if containerCtrl == nil {
			t.Fatal("expected non-nil container controller")
		}
		if _, ok := c.containers["container-1"]; !ok {
			t.Error("container not in map after NewContainer")
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		vm, _, c := newSetup(t)
		expectVMCallsForNewContainer(vm)

		if _, err := c.NewContainer(t.Context(), "container-dup"); err != nil {
			t.Fatalf("first NewContainer: %v", err)
		}
		if _, err := c.NewContainer(t.Context(), "container-dup"); err == nil {
			t.Fatal("expected error for duplicate container ID")
		}
	})

	t.Run("multiple different", func(t *testing.T) {
		vm, _, c := newSetup(t)
		ids := []string{"container-a", "container-b", "container-c"}
		for _, id := range ids {
			expectVMCallsForNewContainer(vm)
			if _, err := c.NewContainer(t.Context(), id); err != nil {
				t.Fatalf("NewContainer(%q): %v", id, err)
			}
		}

		if len(c.containers) != len(ids) {
			t.Errorf("expected %d containers, got %d", len(ids), len(c.containers))
		}
		for _, id := range ids {
			if _, ok := c.containers[id]; !ok {
				t.Errorf("container %q missing from map", id)
			}
		}
	})
}

// TestListContainers verifies snapshots of the live container map.
func TestListContainers(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		_, _, c := newSetup(t)
		if result := c.ListContainers(); len(result) != 0 {
			t.Errorf("expected empty map, got %d entries", len(result))
		}
	})

	t.Run("multiple containers", func(t *testing.T) {
		vm, _, c := newSetup(t)
		ids := []string{"container-x", "container-y"}
		for _, id := range ids {
			expectVMCallsForNewContainer(vm)
			if _, err := c.NewContainer(t.Context(), id); err != nil {
				t.Fatalf("NewContainer(%q): %v", id, err)
			}
		}

		result := c.ListContainers()
		if len(result) != len(ids) {
			t.Errorf("expected %d containers, got %d", len(ids), len(result))
		}
		for _, id := range ids {
			if _, ok := result[id]; !ok {
				t.Errorf("container %q missing from ListContainers result", id)
			}
		}

		// Verify that the returned map is a snapshot: mutating it must not
		// affect the internal state.
		delete(result, ids[0])
		if _, ok := c.containers[ids[0]]; !ok {
			t.Error("deleting from ListContainers result should not affect internal map")
		}
	})
}

// TestDeleteContainer verifies the full create → delete lifecycle and error cases.
func TestDeleteContainer(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		vm, _, c := newSetup(t)
		expectVMCallsForNewContainer(vm)

		if _, err := c.NewContainer(t.Context(), "container-del"); err != nil {
			t.Fatalf("NewContainer: %v", err)
		}
		if err := c.DeleteContainer(t.Context(), "container-del"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := c.containers["container-del"]; ok {
			t.Error("container still in map after DeleteContainer")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, _, c := newSetup(t)
		if err := c.DeleteContainer(t.Context(), "nonexistent"); err == nil {
			t.Fatal("expected error for unknown container ID")
		}
	})

	t.Run("then get fails", func(t *testing.T) {
		vm, _, c := newSetup(t)
		expectVMCallsForNewContainer(vm)

		if _, err := c.NewContainer(t.Context(), "container-gone"); err != nil {
			t.Fatalf("NewContainer: %v", err)
		}
		if err := c.DeleteContainer(t.Context(), "container-gone"); err != nil {
			t.Fatalf("DeleteContainer: %v", err)
		}
		if _, err := c.GetContainer("container-gone"); err == nil {
			t.Fatal("expected error from GetContainer after deletion")
		}
	})

	t.Run("double fails", func(t *testing.T) {
		vm, _, c := newSetup(t)
		expectVMCallsForNewContainer(vm)

		if _, err := c.NewContainer(t.Context(), "container-double-del"); err != nil {
			t.Fatalf("NewContainer: %v", err)
		}
		if err := c.DeleteContainer(t.Context(), "container-double-del"); err != nil {
			t.Fatalf("first DeleteContainer: %v", err)
		}
		if err := c.DeleteContainer(t.Context(), "container-double-del"); err == nil {
			t.Fatal("expected error on second DeleteContainer")
		}
	})
}

// TestNewContainer_AfterDelete verifies that a container can be re-created with
// the same ID after the original has been deleted.
func TestNewContainer_AfterDelete(t *testing.T) {
	vm, _, c := newSetup(t)
	ctx := t.Context()

	// First lifecycle.
	expectVMCallsForNewContainer(vm)
	first, err := c.NewContainer(ctx, "container-recreate")
	if err != nil {
		t.Fatalf("first NewContainer: %v", err)
	}
	if err := c.DeleteContainer(ctx, "container-recreate"); err != nil {
		t.Fatalf("DeleteContainer: %v", err)
	}

	// Re-create with the same ID.
	expectVMCallsForNewContainer(vm)
	second, err := c.NewContainer(ctx, "container-recreate")
	if err != nil {
		t.Fatalf("second NewContainer: %v", err)
	}
	if first == second {
		t.Error("expected a new controller instance after re-creation")
	}

	got, err := c.GetContainer("container-recreate")
	if err != nil {
		t.Fatalf("GetContainer after re-creation: %v", err)
	}
	if got != second {
		t.Error("GetContainer returned the old controller after re-creation")
	}
}
