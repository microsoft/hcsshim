//go:build windows

package scsi

import (
	"context"
	"reflect"
	"testing"
)

func removeIndex[T any](a []T, i int) []T {
	return append(a[:i], a[i+1:]...)
}

func TestRemoveIndex(t *testing.T) {
	a := []int{1, 2, 3, 4, 5}
	a = removeIndex(a, 2)
	if !reflect.DeepEqual(a, []int{1, 2, 4, 5}) {
		t.Errorf("wrong values: %v", a)
	}
	a = removeIndex(a, 1)
	if !reflect.DeepEqual(a, []int{1, 4, 5}) {
		t.Errorf("wrong values: %v", a)
	}
	a = removeIndex(a, 0)
	if !reflect.DeepEqual(a, []int{4, 5}) {
		t.Errorf("wrong values: %v", a)
	}
	a = removeIndex(a, 1)
	if !reflect.DeepEqual(a, []int{4}) {
		t.Errorf("wrong values: %v", a)
	}
	a = removeIndex(a, 0)
	if !reflect.DeepEqual(a, []int{}) {
		t.Errorf("wrong values: %v", a)
	}
}

type testAttachment struct {
	controller uint
	lun        uint
	config     *attachConfig
}

type testMount struct {
	controller uint
	lun        uint
	path       string
	config     *mountConfig
}

type hostBackend struct {
	attachments []*testAttachment
}

func (hb *hostBackend) attach(ctx context.Context, controller uint, lun uint, config *attachConfig) error {
	hb.attachments = append(hb.attachments, &testAttachment{
		controller: controller,
		lun:        lun,
		config:     config,
	})
	return nil
}

func (hb *hostBackend) detach(ctx context.Context, controller uint, lun uint) error {
	for i, a := range hb.attachments {
		if a.controller == controller && a.lun == lun {
			hb.attachments = removeIndex(hb.attachments, i)
			break
		}
	}
	return nil
}

func (hb *hostBackend) attachmentPaths() []string {
	ret := []string{}
	for _, a := range hb.attachments {
		ret = append(ret, a.config.path)
	}
	return ret
}

type guestBackend struct {
	mounts []*testMount
}

func (gb *guestBackend) mount(ctx context.Context, controller uint, lun uint, path string, config *mountConfig) error {
	gb.mounts = append(gb.mounts, &testMount{
		controller: controller,
		lun:        lun,
		path:       path,
		config:     config,
	})
	return nil
}

func (gb *guestBackend) unmount(ctx context.Context, controller uint, lun uint, path string, config *mountConfig) error {
	for i, m := range gb.mounts {
		if m.path == path {
			gb.mounts = removeIndex(gb.mounts, i)
			break
		}
	}
	return nil
}

func (gb *guestBackend) unplug(ctx context.Context, controller uint, lun uint) error {
	return nil
}

func (gb *guestBackend) mountPaths() []string {
	ret := []string{}
	for _, m := range gb.mounts {
		ret = append(ret, m.path)
	}
	return ret
}

func TestAddAddRemoveRemove(t *testing.T) {
	ctx := context.Background()

	hb := &hostBackend{}
	gb := &guestBackend{}
	mgr, err := NewManager(hb, gb, 4, 64, "/var/run/scsi/%d", nil)
	if err != nil {
		t.Fatal(err)
	}
	m1, err := mgr.AddVirtualDisk(ctx, "path", true, "", "", &MountConfig{})
	if err != nil {
		t.Fatal(err)
	}
	m2, err := mgr.AddVirtualDisk(ctx, "path", true, "", "", &MountConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if m1.GuestPath() == "" {
		t.Error("guest path for m1 should not be empty")
	}
	if m1.GuestPath() != m2.GuestPath() {
		t.Errorf("expected same guest paths for both mounts, but got %q and %q", m1.GuestPath(), m2.GuestPath())
	}
	if !reflect.DeepEqual(hb.attachmentPaths(), []string{"path"}) {
		t.Errorf("wrong attachment paths after add: %v", hb.attachmentPaths())
	}
	if err := m1.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m2.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hb.attachmentPaths(), []string{}) {
		t.Errorf("wrong attachment paths after remove: %v", hb.attachmentPaths())
	}
}

func TestGuestPath(t *testing.T) {
	ctx := context.Background()

	hb := &hostBackend{}
	gb := &guestBackend{}
	mgr, err := NewManager(hb, gb, 4, 64, "/var/run/scsi/%d", nil)
	if err != nil {
		t.Fatal(err)
	}

	m1, err := mgr.AddVirtualDisk(ctx, "path", true, "", "/mnt1", &MountConfig{})
	if err != nil {
		t.Fatal(err)
	}
	// m1 should get the guest path it asked for.
	if m1.GuestPath() != "/mnt1" {
		t.Errorf("wrong guest path for m1: %s", m1.GuestPath())
	}
	if !reflect.DeepEqual(gb.mountPaths(), []string{"/mnt1"}) {
		t.Errorf("wrong mount paths after adding m1: %v", gb.mountPaths())
	}

	m2, err := mgr.AddVirtualDisk(ctx, "path", true, "", "", &MountConfig{})
	if err != nil {
		t.Fatal(err)
	}
	// m2 didn't ask for a guest path, so it should re-use m1.
	if m2.GuestPath() != "/mnt1" {
		t.Errorf("wrong guest path for m2: %s", m2.GuestPath())
	}
	if !reflect.DeepEqual(gb.mountPaths(), []string{"/mnt1"}) {
		t.Errorf("wrong mount paths after adding m2: %v", gb.mountPaths())
	}

	m3, err := mgr.AddVirtualDisk(ctx, "path", true, "", "/mnt2", &MountConfig{})
	if err != nil {
		t.Fatal(err)
	}
	// m3 should get the guest path it asked for.
	if m3.GuestPath() != "/mnt2" {
		t.Errorf("wrong guest path for m3: %s", m2.GuestPath())
	}
	if !reflect.DeepEqual(gb.mountPaths(), []string{"/mnt1", "/mnt2"}) {
		t.Errorf("wrong mount paths after adding m3: %v", gb.mountPaths())
	}

	if err := m3.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gb.mountPaths(), []string{"/mnt1"}) {
		t.Errorf("wrong mount paths after removing m3: %v", gb.mountPaths())
	}
	if err := m2.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gb.mountPaths(), []string{"/mnt1"}) {
		t.Errorf("wrong mount paths after removing m2: %v", gb.mountPaths())
	}
	if err := m1.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gb.mountPaths(), []string{}) {
		t.Errorf("wrong mount paths after removing m1: %v", gb.mountPaths())
	}
}

func TestConflictingGuestPath(t *testing.T) {
	ctx := context.Background()

	hb := &hostBackend{}
	gb := &guestBackend{}
	mgr, err := NewManager(hb, gb, 4, 64, "/var/run/scsi/%d", nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := mgr.AddVirtualDisk(ctx, "path", true, "", "/mnt1", &MountConfig{}); err != nil {
		t.Fatal(err)
	}
	// Different host path but same guest path as m1, should conflict.
	if _, err := mgr.AddVirtualDisk(ctx, "path2", true, "", "/mnt1", &MountConfig{}); err == nil {
		t.Fatalf("expected error but got none")
	}
}
