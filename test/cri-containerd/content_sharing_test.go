//go:build windows && functional
// +build windows,functional

package cri_containerd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	ctrdconfig "github.com/containerd/containerd/services/server/config"
	"github.com/pelletier/go-toml"
)

// updateBoltConfig updates the bolt config section inside containerd config to have
// correct values for content sharing policy and snapshot sharing policy based on the
// input boolean parameters.
func updateBoltConfig(cfg *ctrdconfig.Config, contentSharing, snapshotSharing bool) error {
	var boltCfg ctrdconfig.BoltConfig

	boltCfg.ContentSharingPolicy = ctrdconfig.SharingPolicyShared
	if !contentSharing {
		boltCfg.ContentSharingPolicy = ctrdconfig.SharingPolicyIsolated
	}
	boltCfg.SnapshotSharingPolicy = ctrdconfig.SharingPolicyIsolated
	if snapshotSharing {
		boltCfg.SnapshotSharingPolicy = ctrdconfig.SharingPolicyShared
	}

	marshalledCfg, err := toml.Marshal(boltCfg)
	if err != nil {
		fmt.Errorf("failed to marshal bolt config: %s", err)
	}

	boltdata, err := toml.LoadBytes(marshalledCfg)
	if err != nil {
		fmt.Errorf("failed to convert marshalled data into toml tree: %s", err)
	}

	cfg.Plugins["bolt"] = *boltdata
	return nil
}

func createContainerdClientContext(t *testing.T, namespace string) (*containerd.Client, context.Context, error) {
	ctx := namespaces.WithNamespace(context.Background(), namespace)

	conn, err := createGRPCConn(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("grpc connection failed: %s", err)
	}

	client, err := containerd.NewWithConn(conn, containerd.WithDefaultNamespace(namespace))
	if err != nil {
		return nil, nil, fmt.Errorf("create containerd client failed: %s", err)
	}

	return client, ctx, nil
}

// Create 3 namespaces, 1 common & 2 private. Pull an image into common namespace and make
// sure you can access it. Pull another images into private images and make sure you can
// access those only from that namespace. Finally, verify that the snapshots for common
// layers are still shared even if namespaces are different.
func Test_SnapshotSharing(t *testing.T) {
	cfg, err := loadContainerdConfigFile(tomlPath)
	if err != nil {
		t.Fatalf("failed to load containerd config: %s\n", err)
	}

	if err = updateBoltConfig(cfg, false, true); err != nil {
		t.Fatalf("failed to set bolt config: %s", err)
	}

	// use a temporary directory as containerd data directory
	tempDir := t.TempDir()
	cfg.Root = filepath.Join(tempDir, "root")
	cfg.State = filepath.Join(tempDir, "state")

	cm := NewContainerdManager(t, cfg)
	cm.init()
	defer cm.cleanup()

	// All of the following image are created from nanoserver:ltsc2022.  img1
	// has 4 layers, img2 has 6 layers and img3 has 4 layers. All of these images have
	// the common base layer of nanoserver:ltsc2022.  img2 is created by adding two
	// new layers on top of img1, so 4 bottom most layers are common between img1 &
	// img2. img3 shares just one layer with other images (other than the common
	// base layer).
	// When these images are pulled and snapshot sharing is enabled we should see
	// exactly 8 snapshots in the backend windows snapshotter. (1 for nanoserver base
	// layer, 3 unique from img1, 2 unique from img2 & 2 unique from img3).
	imgs := []string{
		"cplatpublic.azurecr.io/multilayer_nanoserver_1:ltsc2022",
		"cplatpublic.azurecr.io/multilayer_nanoserver_2:ltsc2022",
		"cplatpublic.azurecr.io/multilayer_nanoserver_3:ltsc2022",
	}

	testData := []struct {
		client   *containerd.Client
		ctx      context.Context
		ns       string
		nsLabels map[string]string
	}{
		{ns: "common", nsLabels: map[string]string{"containerd.io/namespace.shareable": "true"}},
		{ns: "private1", nsLabels: map[string]string{}},
		{ns: "private2", nsLabels: map[string]string{}},
	}

	for i := range testData {
		td := &testData[i]
		td.client, td.ctx, err = createContainerdClientContext(t, td.ns)
		if err != nil {
			t.Fatalf("failed to created containerd client & context: %s", err)
		}

		// create namespaces
		err = td.client.NamespaceService().Create(td.ctx, td.ns, td.nsLabels)
		if err != nil {
			t.Fatalf("failed to create namespace: %s", err)
		}

		_, err = td.client.Pull(td.ctx, imgs[i], containerd.WithPullUnpack)
		if err != nil {
			t.Fatalf("failed to pull image: %s", err)
		}
	}

	// verify that we have exactly 8 snapshots.
	// windows snapshotter directory
	snDir := filepath.Join(cfg.Root, "io.containerd.snapshotter.v1.windows", "snapshots")
	entries, err := os.ReadDir(snDir)
	if err != nil {
		t.Fatalf("failed to read snapshot directory: %s", err)
	}

	if len(entries) != 8 {
		t.Fatalf("expected exactly 8 snapshot directories")
	}

	// remove images so that cleanup doesn't fail.
	for i, td := range testData {
		if err := td.client.ImageService().Delete(td.ctx, imgs[i], images.SynchronousDelete()); err != nil {
			t.Logf("failed to remove image %s: %s", imgs[i], err)
		}
	}
}
