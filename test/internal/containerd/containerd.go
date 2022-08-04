//go:build windows

package containerd

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	kubeutil "github.com/containerd/containerd/integration/remote/util"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/containerd/remotes/docker/config"
	"github.com/containerd/containerd/snapshots"
	"github.com/opencontainers/image-spec/identity"
	"google.golang.org/grpc"

	"github.com/Microsoft/hcsshim/test/internal/constants"
	"github.com/Microsoft/hcsshim/test/internal/timeout"
)

// images maps image refs -> chain ID
var images sync.Map

// default containerd.New(address) does not connect to tcp endpoints on windows
func createGRPCConn(ctx context.Context, address string) (*grpc.ClientConn, error) {
	addr, dialer, err := kubeutil.GetAddressAndDialer(address)
	if err != nil {
		return nil, err
	}

	return grpc.DialContext(ctx, addr, grpc.WithInsecure(), grpc.WithContextDialer(dialer))
}

type ContainerdClientOptions struct {
	Address   string
	Namespace string
}

// NewClient returns a containerd client, a context with the namespace set, and the
// context's cancel function. The context should be used for containerd operations, and
// cancel function will terminate those operations early.
func (cco ContainerdClientOptions) NewClient(
	ctx context.Context,
	t testing.TB,
	opts ...containerd.ClientOpt,
) (context.Context, context.CancelFunc, *containerd.Client) {
	t.Helper()

	// regular `New` does not work on windows, need to use `WithConn`
	cctx, ccancel := context.WithTimeout(ctx, timeout.ConnectTimeout)
	defer ccancel()

	conn, err := createGRPCConn(cctx, cco.Address)
	if err != nil {
		t.Fatalf("failed to dial runtime client: %v", err)
	}

	defOpts := []containerd.ClientOpt{
		containerd.WithDefaultNamespace(cco.Namespace),
	}
	opts = append(defOpts, opts...)
	c, err := containerd.NewWithConn(conn, opts...)
	if err != nil {
		t.Fatalf("could not create new containerd client: %v", err)
	}
	t.Cleanup(func() {
		c.Close()
	})

	ctx = namespaces.WithNamespace(ctx, cco.Namespace)
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	return ctx, cancel, c
}

func GetPlatformComparer(t testing.TB, platform string) platforms.MatchComparer {
	var p platforms.MatchComparer
	if platform == "" {
		p = platforms.All
	} else {
		pp, err := platforms.Parse(platform)
		if err != nil {
			t.Helper()
			t.Fatalf("could not parse platform %q: %v", platform, err)
		}
		p = platforms.Only(pp)
	}

	return p
}

// GetImageChainID gets the chain id of an image. platform can be "".
func GetImageChainID(ctx context.Context, t testing.TB, client *containerd.Client, image, platform string) string {
	t.Helper()
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

func CreateActiveSnapshot(ctx context.Context, t testing.TB, client *containerd.Client, snapshotter, parent, key string, opts ...snapshots.Opt) []mount.Mount {
	t.Helper()

	ss := client.SnapshotService(snapshotter)

	ms, err := ss.Prepare(ctx, key, parent, opts...)
	if err != nil {
		t.Fatalf("could not make active snapshot %q from %q: %v", key, parent, err)
	}

	t.Cleanup(func() {
		if err := ss.Remove(ctx, key); err != nil && !errors.Is(err, errdefs.ErrNotFound) {
			// remove is not idempotent, so do not Fail test
			t.Logf("failed to remove active snapshot %q: %v", key, err)
		}
	})

	return ms
}

// a view will not not create a new scratch layer/vhd, but instead return only the directory of the
// committed snapshot `parent`
func CreateViewSnapshot(ctx context.Context, t testing.TB, client *containerd.Client, snapshotter, parent, key string, opts ...snapshots.Opt) []mount.Mount {
	t.Helper()

	ss := client.SnapshotService(snapshotter)

	ms, err := ss.View(ctx, key, parent, opts...)
	if err != nil {
		t.Fatalf("could not make active snapshot %q from %q: %v", key, parent, err)
	}

	t.Cleanup(func() {
		if err := ss.Remove(ctx, key); err != nil && !errors.Is(err, errdefs.ErrNotFound) {
			// remove is not idempotent, so do not Fail test
			t.Logf("failed to remove view snapshot %q: %v %T", key, err, err)
		}
	})

	return ms
}

// copied from https://github.com/containerd/containerd/blob/main/cmd/ctr/commands/images/pull.go

// PullImage pulls the image for the specified platform and returns the chain ID
func PullImage(ctx context.Context, t testing.TB, client *containerd.Client, ref, plat string) string {
	if chainID, ok := images.Load(ref); ok {
		return chainID.(string)
	}

	opts := []containerd.RemoteOpt{
		containerd.WithSchema1Conversion,
		containerd.WithPlatform(plat),
		containerd.WithPullUnpack,
	}

	if s, err := constants.SnapshotterFromPlatform(plat); err == nil {
		opts = append(opts, containerd.WithPullSnapshotter(s))
	}

	img, err := client.Pull(ctx, ref, opts...)
	if err != nil {
		t.Fatalf("could not pull image %q: %v", ref, err)
	}

	name := img.Name()
	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		t.Fatalf("could not fetch RootFS diff IDs for %q: %v", name, err)
	}

	chainID := identity.ChainID(diffIDs).String()
	images.Store(ref, chainID)
	t.Logf("unpacked image %q with ID %q", name, chainID)

	return chainID
}

func GetResolver(ctx context.Context, _ testing.TB) remotes.Resolver {
	options := docker.ResolverOptions{
		Tracker: docker.NewInMemoryTracker(),
	}
	hostOptions := config.HostOptions{}
	options.Hosts = config.ConfigureHosts(ctx, hostOptions)

	return docker.NewResolver(options)
}
