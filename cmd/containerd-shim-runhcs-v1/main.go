//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Microsoft/go-winio/pkg/etw"
	"github.com/Microsoft/go-winio/pkg/etwlogrus"
	"github.com/Microsoft/go-winio/pkg/guid"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	hcsversion "github.com/Microsoft/hcsshim/internal/version"

	// register common types spec with typeurl
	_ "github.com/containerd/containerd/runtime"
)

const usage = ``
const ttrpcAddressEnv = "TTRPC_ADDRESS"

// Add a manifest to get proper Windows version detection.
//go:generate go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo -platform-specific

// `-ldflags '-X ...'` only works if the variable is uninitialized or set to a constant value.
// keep empty and override with data from [internal/version] only if empty to allow
// workflows currently setting these values to work.
var (
	// version will be the repo version that the binary was built from
	version = ""
	// gitCommit will be the hash that the binary was built from
	gitCommit = ""
)

var (
	namespaceFlag        string
	addressFlag          string
	containerdBinaryFlag string

	idFlag string

	// gracefulShutdownTimeout is how long to wait for clean-up before just exiting
	gracefulShutdownTimeout = 3 * time.Second
)

func etwCallback(sourceID guid.GUID, state etw.ProviderState, level etw.Level, matchAnyKeyword uint64, matchAllKeyword uint64, filterData uintptr) {
	if state == etw.ProviderStateCaptureState {
		resp, err := svc.DiagStacks(context.Background(), &shimdiag.StacksRequest{})
		if err != nil {
			return
		}
		log := logrus.WithField("tid", svc.tid)
		log.WithField("stack", resp.Stacks).Info("goroutine stack dump")
		if resp.GuestStacks != "" {
			log.WithField("stack", resp.GuestStacks).Info("guest stack dump")
		}
	}
}

func main() {
	logrus.AddHook(log.NewHook())

	// Provider ID: 0b52781f-b24d-5685-ddf6-69830ed40ec3
	// Provider and hook aren't closed explicitly, as they will exist until process exit.
	provider, err := etw.NewProvider("Microsoft.Virtualization.RunHCS", etwCallback)
	if err != nil {
		logrus.Error(err)
	} else {
		if hook, err := etwlogrus.NewHookFromProvider(provider); err == nil {
			logrus.AddHook(hook)
		} else {
			logrus.Error(err)
		}
	}

	// fall back on embedded version info (if any), if variables above were not set
	if version == "" {
		version = hcsversion.Version
	}
	if gitCommit == "" {
		gitCommit = hcsversion.Commit
	}

	_ = provider.WriteEvent(
		"ShimLaunched",
		nil,
		etw.WithFields(
			etw.StringArray("Args", os.Args),
			etw.StringField("version", version),
			etw.StringField("commit", gitCommit),
		),
	)

	// Register our OpenCensus logrus exporter
	trace.ApplyConfig(trace.Config{DefaultSampler: oc.DefaultSampler})
	trace.RegisterExporter(&oc.LogrusExporter{})

	app := cli.NewApp()
	app.Name = "containerd-shim-runhcs-v1"
	app.Usage = usage

	var v []string
	if version != "" {
		v = append(v, version)
	}
	if gitCommit != "" {
		v = append(v, fmt.Sprintf("commit: %s", gitCommit))
	}
	v = append(v, fmt.Sprintf("spec: %s", specs.Version))
	app.Version = strings.Join(v, "\n")

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "namespace",
			Usage: "the namespace of the container",
		},
		cli.StringFlag{
			Name:  "address",
			Usage: "the address of the containerd's main socket",
		},
		cli.StringFlag{
			Name:  "publish-binary",
			Usage: "the binary path to publish events back to containerd",
		},
		cli.StringFlag{
			Name:  "id",
			Usage: "the id of the container",
		},
		cli.StringFlag{
			Name:  "bundle",
			Usage: "the bundle path to delete (delete command only).",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "run the shim in debug mode",
		},
	}
	app.Commands = []cli.Command{
		startCommand,
		deleteCommand,
		serveCommand,
	}
	app.Before = func(context *cli.Context) error {
		if namespaceFlag = context.GlobalString("namespace"); namespaceFlag == "" {
			return errors.New("namespace is required")
		}
		if addressFlag = context.GlobalString("address"); addressFlag == "" {
			return errors.New("address is required")
		}
		if containerdBinaryFlag = context.GlobalString("publish-binary"); containerdBinaryFlag == "" {
			return errors.New("publish-binary is required")
		}
		if idFlag = context.GlobalString("id"); idFlag == "" {
			return errors.New("id is required")
		}
		return nil
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(cli.ErrWriter, err)
		os.Exit(1)
	}
}
