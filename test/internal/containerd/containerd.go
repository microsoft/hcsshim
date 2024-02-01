//go:build windows

package containerd

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/containerd/containerd"
	kubeutil "github.com/containerd/containerd/integration/remote/util"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/containerd/remotes/docker/config"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/image-spec/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	imagesutil "github.com/Microsoft/hcsshim/test/pkg/images"
	"github.com/Microsoft/hcsshim/test/pkg/timeout"
)

// images maps image refs -> chain ID
var images sync.Map

// default containerd.New(address) does not connect to tcp endpoints on windows
func createGRPCConn(ctx context.Context, address string) (*grpc.ClientConn, error) {
	addr, dialer, err := kubeutil.GetAddressAndDialer(address)
	if err != nil {
		return nil, err
	}
	return grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer))
}

type ContainerdClientOptions struct {
	Address   string
	Namespace string
}

// NewClient returns a containerd client connected using the specified address and namespace.
func (cco ContainerdClientOptions) NewClient(
	ctx context.Context,
	tb testing.TB,
	opts ...containerd.ClientOpt,
) *containerd.Client {
	tb.Helper()

	ctx, cancel := context.WithTimeout(ctx, timeout.ConnectTimeout)
	defer cancel()

	// regular `New` does not work on windows, need to use `WithConn`
	conn, err := createGRPCConn(ctx, cco.Address)
	if err != nil {
		tb.Fatalf("failed to dial runtime client: %v", err)
	}

	defOpts := []containerd.ClientOpt{
		containerd.WithDefaultNamespace(cco.Namespace),
	}
	opts = append(defOpts, opts...)
	c, err := containerd.NewWithConn(conn, opts...)
	if err != nil {
		tb.Fatalf("could not create new containerd client: %v", err)
	}
	tb.Cleanup(func() {
		c.Close()
	})

	return c
}

func GetPlatformComparer(tb testing.TB, platform string) platforms.MatchComparer {
	tb.Helper()
	var p platforms.MatchComparer
	if platform == "" {
		p = platforms.All
	} else {
		pp, err := platforms.Parse(platform)
		if err != nil {
			tb.Fatalf("could not parse platform %q: %v", platform, err)
		}
		p = platforms.Only(pp)
	}

	return p
}

// GetImageChainID gets the chain id of an image. platform can be "".
func GetImageChainID(ctx context.Context, tb testing.TB, client *containerd.Client, image, platform string) string {
	tb.Helper()
	is := client.ImageService()

	i, err := is.Get(ctx, image)
	if err != nil {
		tb.Fatalf("could not retrieve image %q: %v", image, err)
	}

	p := GetPlatformComparer(tb, platform)

	diffIDs, err := i.RootFS(ctx, client.ContentStore(), p)
	if err != nil {
		tb.Fatalf("could not retrieve unpacked diff ids: %v", err)
	}
	chainID := identity.ChainID(diffIDs).String()

	return chainID
}

func CreateActiveSnapshot(ctx context.Context, tb testing.TB, client *containerd.Client, snapshotter, parent, key string, opts ...snapshots.Opt) []mount.Mount {
	tb.Helper()

	ss := client.SnapshotService(snapshotter)

	ms, err := ss.Prepare(ctx, key, parent, opts...)
	if err != nil {
		tb.Fatalf("could not make active snapshot %q from %q: %v", key, parent, err)
	}

	tb.Cleanup(func() {
		if err := ss.Remove(ctx, key); err != nil && !errors.Is(err, errdefs.ErrNotFound) {
			// remove is not idempotent, so do not Fail test
			tb.Logf("failed to remove active snapshot %q: %v", key, err)
		}
	})

	return ms
}

// a view will not not create a new scratch layer/vhd, but instead return only the directory of the
// committed snapshot `parent`
func CreateViewSnapshot(ctx context.Context, tb testing.TB, client *containerd.Client, snapshotter, parent, key string, opts ...snapshots.Opt) []mount.Mount {
	tb.Helper()

	ss := client.SnapshotService(snapshotter)

	ms, err := ss.View(ctx, key, parent, opts...)
	if err != nil {
		tb.Fatalf("could not make active snapshot %q from %q: %v", key, parent, err)
	}

	tb.Cleanup(func() {
		if err := ss.Remove(ctx, key); err != nil && !errors.Is(err, errdefs.ErrNotFound) {
			// remove is not idempotent, so do not Fail test
			tb.Logf("failed to remove view snapshot %q: %v %T", key, err, err)
		}
	})

	return ms
}

// copied from https://github.com/containerd/containerd/blob/main/cmd/ctr/commands/images/pull.go

// PullImage pulls the image for the specified platform and returns the chain ID
func PullImage(ctx context.Context, tb testing.TB, client *containerd.Client, ref, plat string) string {
	tb.Helper()

	if chainID, ok := images.Load(ref); ok {
		return chainID.(string)
	}

	opts := []containerd.RemoteOpt{
		containerd.WithPlatform(plat),
		containerd.WithPullUnpack,
	}

	if s, err := imagesutil.SnapshotterFromPlatform(plat); err == nil {
		opts = append(opts, containerd.WithPullSnapshotter(s))
	}

	img, err := client.Pull(ctx, ref, opts...)
	if err != nil {
		tb.Fatalf("could not pull image %q: %v", ref, err)
	}

	name := img.Name()
	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		tb.Fatalf("could not fetch RootFS diff IDs for %q: %v", name, err)
	}

	chainID := identity.ChainID(diffIDs).String()
	images.Store(ref, chainID)
	tb.Logf("unpacked image %q with ID %q", name, chainID)

	return chainID
}

func GetResolver(ctx context.Context, tb testing.TB) remotes.Resolver {
	tb.Helper()
	options := docker.ResolverOptions{
		Tracker: docker.NewInMemoryTracker(),
	}
	hostOptions := config.HostOptions{}
	options.Hosts = config.ConfigureHosts(ctx, hostOptions)

	return docker.NewResolver(options)
}
