//go:build linux

package gcs

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cgroups "github.com/containerd/cgroups/v3/cgroup1"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	"github.com/Microsoft/hcsshim/internal/guest/runtime/hcsv2"
	"github.com/Microsoft/hcsshim/internal/guest/runtime/runc"
	"github.com/Microsoft/hcsshim/internal/guest/transport"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"

	"github.com/Microsoft/hcsshim/test/internal/util"
	testflag "github.com/Microsoft/hcsshim/test/pkg/flag"
	"github.com/Microsoft/hcsshim/test/pkg/require"
)

const (
	featureCRI        = "CRI"
	featureStandalone = "StandAlone"
)

var allFeatures = []string{
	featureCRI,
	featureStandalone,
}

var (
	flagLogLevel      = testflag.NewLogrusLevel("log-level", "warning", "logrus logging `level`")
	flagFeatures      = testflag.NewFeatureFlag(allFeatures)
	flagJoinGCSCgroup = flag.Bool(
		"join-gcs-cgroup",
		false,
		"If true, join the same cgroup as the gcs daemon, `/gcs`",
	)
	flagRootfsPath = flag.String(
		"rootfs-path",
		"/run/rootfs",
		"The path on the uVM of the unpacked rootfs to use for the containers",
	)
	flagSandboxPause = flag.Bool(
		"pause-sandbox",
		false,
		"Use `/pause` as the sandbox container command",
	)
)

func TestMain(m *testing.M) {
	flag.Parse()

	if err := setup(); err != nil {
		logrus.WithError(err).Fatal("could not set up testing")
	}

	// print additional configuration options when running benchmarks, so we can better track performance.
	if util.RunningBenchmarks() {
		util.PrintAdditionalBenchmarkConfig()

		// print uname info
		uname := &unix.Utsname{}
		if err := unix.Uname(uname); err == nil {
			for n, bs := range map[string][]byte{
				"kernel-name":    uname.Sysname[:],
				"node-name":      uname.Nodename[:],
				"kernel-release": uname.Release[:],
				"kernel-version": uname.Version[:],
				"machine-name":   uname.Machine[:],
				"domain-name":    uname.Domainname[:],
			} {
				if s := unix.ByteSliceToString(bs); s != "" && s != "(none)" {
					fmt.Println(n + ": " + s)
				}
			}
		}

		// print additional (rootfs) build info written by Makefile to /info/
		for _, f := range []string{
			"tar.date",
			"image.name",
			"build.date",
		} {
			if b, err := os.ReadFile(filepath.Join("/info", f)); err == nil {
				if s := strings.TrimSpace(string(b)); s != "" {
					// convention for benchmark config is kebab-case
					fmt.Println("rootfs-" + strings.ReplaceAll(f, ".", "-") + ": " + s)
				}
			}

		}
	}

	os.Exit(m.Run())
}

func setup() (err error) {
	_ = os.MkdirAll(guestpath.LCOWRootPrefixInUVM, 0755)

	otel.SetTracerProvider(sdktrace.NewTracerProvider(
		sdktrace.WithSampler(otelutil.DefaultSampler),
		sdktrace.WithBatcher(&otelutil.LogrusExporter{}),
	))

	logrus.SetLevel(flagLogLevel.Level)
	// test2json does not consume stderr
	logrus.SetOutput(os.Stdout)
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	logrus.Debugf("using features: %s", flagFeatures.Strings())

	// should already start in gcs cgroup
	if !*flagJoinGCSCgroup {
		gcsControl, err := cgroups.Load(cgroups.StaticPath("/"))
		if err != nil {
			return fmt.Errorf("failed to load root cgroup: %w", err)
		}
		if err := gcsControl.Add(cgroups.Process{Pid: os.Getpid()}); err != nil {
			return fmt.Errorf("failed join root cgroup: %w", err)
		}
		logrus.Debug("joined root cgroup")
	}

	// initialize runtime
	rt, err := getRuntimeErr()
	if err != nil {
		return err
	}

	// check policy will be parsed properly
	if _, err = getHostErr(rt, getTransport()); err != nil {
		return err
	}

	return nil
}

//
// host and runtime management
//

func getTestState(ctx context.Context, tb testing.TB) (*hcsv2.Host, runtime.Runtime) {
	tb.Helper()
	rt := getRuntime(ctx, tb)
	return getHost(ctx, tb, rt), rt
}

func getHost(_ context.Context, tb testing.TB, rt runtime.Runtime) *hcsv2.Host {
	tb.Helper()
	h, err := getHostErr(rt, getTransport())
	if err != nil {
		tb.Fatalf("could not get host: %v", err)
	}

	return h
}

func getHostErr(rt runtime.Runtime, tp transport.Transport) (*hcsv2.Host, error) {
	h := hcsv2.NewHost(rt, tp, &securitypolicy.OpenDoorSecurityPolicyEnforcer{}, os.Stdout)
	if err := h.SetConfidentialUVMOptions(
		context.Background(),
		&guestresource.LCOWConfidentialOptions{},
	); err != nil {
		return nil, fmt.Errorf("could not set host security policy: %w", err)
	}

	return h, nil
}

func getRuntime(_ context.Context, tb testing.TB) runtime.Runtime {
	tb.Helper()
	rt, err := getRuntimeErr()
	if err != nil {
		tb.Fatalf("could not get runtime: %v", err)
	}

	return rt
}

func getRuntimeErr() (runtime.Runtime, error) {
	rt, err := runc.NewRuntime(guestpath.LCOWRootPrefixInUVM)
	if err != nil {
		return rt, fmt.Errorf("failed to initialize runc runtime: %w", err)
	}

	return rt, nil
}

func getTransport() transport.Transport {
	return &PipeTransport{}
}

func requireFeatures(tb testing.TB, features ...string) {
	tb.Helper()
	require.Features(tb, flagFeatures, features...)
}
