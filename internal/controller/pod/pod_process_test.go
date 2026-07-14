//go:build windows && wcowprocess

package pod

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

const (
	testPodID          = "test-pod-1234"
	testIORetryTimeout = 5 * time.Second
)

// TestNewPodStartsEmpty verifies a freshly created pod reports its ID and holds no containers.
func TestNewPodStartsEmpty(t *testing.T) {
	c := New(testPodID, testIORetryTimeout)

	if got := c.PodID(); got != testPodID {
		t.Errorf("pod ID = %q, want %q", got, testPodID)
	}
	if got := c.ListContainers(); len(got) != 0 {
		t.Errorf("new pod has %d containers, want 0", len(got))
	}
}

// TestContainerRegistrationAndRetrieval verifies created containers are handed back,
// become retrievable, and that duplicate and multiple IDs behave as a caller expects.
func TestContainerRegistrationAndRetrieval(t *testing.T) {
	t.Run("creating a container makes it retrievable", func(t *testing.T) {
		c := New(testPodID, testIORetryTimeout)

		created, err := c.NewContainer(t.Context(), "container-1")
		if err != nil {
			t.Fatalf("create container: %v", err)
		}
		if created == nil {
			t.Fatal("create container returned a nil controller")
		}

		got, err := c.GetContainer("container-1")
		if err != nil {
			t.Fatalf("get container: %v", err)
		}
		if got != created {
			t.Error("retrieved a different controller than the one created")
		}
	})

	t.Run("creating a duplicate ID fails", func(t *testing.T) {
		c := New(testPodID, testIORetryTimeout)

		if _, err := c.NewContainer(t.Context(), "dup"); err != nil {
			t.Fatalf("first create: %v", err)
		}
		if _, err := c.NewContainer(t.Context(), "dup"); err == nil {
			t.Fatal("expected an error creating a container with a duplicate ID")
		}
	})

	t.Run("distinct IDs are tracked independently", func(t *testing.T) {
		c := New(testPodID, testIORetryTimeout)
		ids := []string{"a", "b", "c"}
		for _, id := range ids {
			if _, err := c.NewContainer(t.Context(), id); err != nil {
				t.Fatalf("create %q: %v", id, err)
			}
		}

		if got := c.ListContainers(); len(got) != len(ids) {
			t.Fatalf("listed %d containers, want %d", len(got), len(ids))
		}
		for _, id := range ids {
			if _, err := c.GetContainer(id); err != nil {
				t.Errorf("get %q: %v", id, err)
			}
		}
	})
}

// TestRetrievingUnknownContainerFails verifies an unknown ID surfaces an error to the caller.
func TestRetrievingUnknownContainerFails(t *testing.T) {
	c := New(testPodID, testIORetryTimeout)

	if _, err := c.GetContainer("missing"); err == nil {
		t.Fatal("expected an error retrieving an unknown container ID")
	}
}

// TestListingContainers verifies the empty case, the populated case, and that the
// returned map is a snapshot the caller can mutate without affecting the pod.
func TestListingContainers(t *testing.T) {
	t.Run("empty pod lists nothing", func(t *testing.T) {
		c := New(testPodID, testIORetryTimeout)
		if got := c.ListContainers(); len(got) != 0 {
			t.Errorf("listed %d containers, want 0", len(got))
		}
	})

	t.Run("returned map is an independent snapshot", func(t *testing.T) {
		c := New(testPodID, testIORetryTimeout)
		if _, err := c.NewContainer(t.Context(), "x"); err != nil {
			t.Fatalf("create container: %v", err)
		}

		list := c.ListContainers()
		delete(list, "x")

		// Mutating the snapshot must not remove the container from the pod.
		if _, err := c.GetContainer("x"); err != nil {
			t.Error("mutating the listed map affected the pod's state")
		}
	})
}

// TestDeletingContainers verifies removal makes a container unreachable and that
// deleting an unknown or already-deleted ID fails.
func TestDeletingContainers(t *testing.T) {
	t.Run("deleting removes the container", func(t *testing.T) {
		c := New(testPodID, testIORetryTimeout)
		if _, err := c.NewContainer(t.Context(), "gone"); err != nil {
			t.Fatalf("create container: %v", err)
		}

		if err := c.DeleteContainer(t.Context(), "gone"); err != nil {
			t.Fatalf("delete container: %v", err)
		}
		if _, err := c.GetContainer("gone"); err == nil {
			t.Error("expected retrieval to fail after deletion")
		}
		if got := c.ListContainers(); len(got) != 0 {
			t.Errorf("listed %d containers after deletion, want 0", len(got))
		}
	})

	t.Run("deleting an unknown ID fails", func(t *testing.T) {
		c := New(testPodID, testIORetryTimeout)
		if err := c.DeleteContainer(t.Context(), "missing"); err == nil {
			t.Fatal("expected an error deleting an unknown container ID")
		}
	})

	t.Run("deleting twice fails", func(t *testing.T) {
		c := New(testPodID, testIORetryTimeout)
		if _, err := c.NewContainer(t.Context(), "twice"); err != nil {
			t.Fatalf("create container: %v", err)
		}
		if err := c.DeleteContainer(t.Context(), "twice"); err != nil {
			t.Fatalf("first delete: %v", err)
		}
		if err := c.DeleteContainer(t.Context(), "twice"); err == nil {
			t.Fatal("expected an error deleting the same container twice")
		}
	})
}

// TestRecreatingDeletedContainer verifies a deleted ID can be reused and yields a fresh controller.
func TestRecreatingDeletedContainer(t *testing.T) {
	c := New(testPodID, testIORetryTimeout)
	ctx := t.Context()

	first, err := c.NewContainer(ctx, "recreate")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := c.DeleteContainer(ctx, "recreate"); err != nil {
		t.Fatalf("delete container: %v", err)
	}

	second, err := c.NewContainer(ctx, "recreate")
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if first == second {
		t.Error("re-creating a deleted ID should yield a fresh controller")
	}

	got, err := c.GetContainer("recreate")
	if err != nil {
		t.Fatalf("get after re-create: %v", err)
	}
	if got != second {
		t.Error("retrieval returned the stale controller after re-creation")
	}
}

// TestConcurrentContainerAccess verifies the pod safely serves concurrent create,
// retrieve, list, and delete calls, leaving no containers behind. Run with -race.
func TestConcurrentContainerAccess(t *testing.T) {
	c := New(testPodID, testIORetryTimeout)
	ctx := t.Context()
	const workers = 50

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("container-%d", i)
			if _, err := c.NewContainer(ctx, id); err != nil {
				t.Errorf("create %q: %v", id, err)
				return
			}
			_, _ = c.GetContainer(id)
			_ = c.ListContainers()
			if err := c.DeleteContainer(ctx, id); err != nil {
				t.Errorf("delete %q: %v", id, err)
			}
		}(i)
	}
	wg.Wait()

	if got := c.ListContainers(); len(got) != 0 {
		t.Errorf("listed %d containers after concurrent add/delete, want 0", len(got))
	}
}
