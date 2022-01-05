package testutil

import (
	"context"
	"testing"

	"github.com/containerd/containerd"
	kubeutil "github.com/containerd/containerd/integration/remote/util"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/snapshots"
	"github.com/opencontainers/image-spec/identity"
	"google.golang.org/grpc"
)

// default containerd.New(address) does not connect to tcp endpoints on windows
func createGRPCConn(ctx context.Context, address string) (*grpc.ClientConn, error) {
	addr, dialer, err := kubeutil.GetAddressAndDialer(address)
	if err != nil {
		return nil, err
	}
	return grpc.DialContext(ctx, addr, grpc.WithInsecure(), grpc.WithContextDialer(dialer))
}

type CtrdClientOptions struct {
	Address   string
	Namespace string
}

func (cco CtrdClientOptions) defaultOpts() []containerd.ClientOpt {
	return []containerd.ClientOpt{containerd.WithDefaultNamespace(cco.Namespace)}
}

// returned context is how namespaces are passed to various containerd client calls
func (cco CtrdClientOptions) NewClient(ctx context.Context, t *testing.T, opts ...containerd.ClientOpt) (*containerd.Client, context.Context) {
	// regular `New` does not work on windows, need to use `WithConn`
	cctx, ccancel := context.WithTimeout(ctx, connectTimeout)
	defer ccancel()

	conn, err := createGRPCConn(cctx, cco.Address)
	if err != nil {
		t.Fatalf("failed to dial runtime client: %v", err)
	}

	opts = append(opts, cco.defaultOpts()...)
	c, err := containerd.NewWithConn(conn, opts...)
	if err != nil {
		t.Fatalf("containerd.New() client failed: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	ctx = namespaces.WithNamespace(ctx, cco.Namespace)
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	return c, ctx
}

func GetPlatformComparer(t *testing.T, platform string) platforms.MatchComparer {
	var p platforms.MatchComparer
	if platform == "" {
		p = platforms.All
	} else {
		pp, err := platforms.Parse(platform)
		if err != nil {
			t.Fatalf("could not parse platform %q: %v", platform, err)
		}
		p = platforms.Only(pp)
	}

	return p
}

// GetImageChainID gets the chain id of an image. platform can be "".
func GetImageChainID(ctx context.Context, t *testing.T, client *containerd.Client, image, platform string) string {
	is := client.ImageService()

	i, err := is.Get(ctx, image)
	if err != nil {
		t.Fatalf("could not retrieve image %q: %v", image, err)
	}

	p := GetPlatformComparer(t, platform)

	diffIDs, err := i.RootFS(ctx, client.ContentStore(), p)
	if err != nil {
		t.Fatalf("could not retrieve unpacked diff ids: %v", err)
	}
	chainID := identity.ChainID(diffIDs).String()
	return chainID
}

func CreateActiveSnapshot(ctx context.Context, t *testing.T, client *containerd.Client, snapshotter, parent, key string, opts ...snapshots.Opt) []mount.Mount {
	ss := client.SnapshotService(snapshotter)

	ms, err := ss.Prepare(ctx, key, parent, opts...)
	if err != nil {
		t.Fatalf("could not make active snapshot %q from %q: %v", key, parent, err)
	}

	t.Cleanup(func() {
		err = ss.Remove(ctx, key)
		if err != nil {
			// remove is not idempotent, so do not Fail test
			t.Logf("failed to remove active snapshot %q: %v", key, err)
		}
	})

	return ms
}

// a view will not not create a new scratch layer/vhd, but instead return only the directory of the
// committed snapshot `parent`
func CreateViewSnapshot(ctx context.Context, t *testing.T, client *containerd.Client, snapshotter, parent, key string, opts ...snapshots.Opt) []mount.Mount {
	ss := client.SnapshotService(snapshotter)

	ms, err := ss.View(ctx, key, parent, opts...)
	if err != nil {
		t.Fatalf("could not make active snapshot %q from %q: %v", key, parent, err)
	}

	t.Cleanup(func() {
		err = ss.Remove(ctx, key)
		if err != nil {
			// remove is not idempotent, so do not Fail test
			t.Logf("failed to remove view snapshot %q: %v", key, err)
		}
	})

	return ms
}
