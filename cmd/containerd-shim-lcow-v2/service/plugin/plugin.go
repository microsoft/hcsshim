//go:build windows

package plugin

import (
	"context"
	"os"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-lcow-v2/service"
	"github.com/Microsoft/hcsshim/internal/shim"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	hcsversion "github.com/Microsoft/hcsshim/internal/version"

	"github.com/Microsoft/go-winio/pkg/etw"
	"github.com/Microsoft/go-winio/pkg/etwlogrus"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/containerd/containerd/v2/pkg/shutdown"
	"github.com/containerd/containerd/v2/plugins"
	"github.com/containerd/plugin"
	"github.com/containerd/plugin/registry"
	"github.com/sirupsen/logrus"
)

const (
	// etwProviderName is the ETW provider name for lcow shim.
	etwProviderName = "Microsoft.Virtualization.RunHCSLCOW"
)

// svc holds the single Service instance created during plugin initialization.
var svc *service.Service

func init() {
	// Provider ID: 64F6FC7F-8326-5EE8-B890-3734AE584136
	// Provider and hook aren't closed explicitly, as they will exist until process exit.
	provider, err := etw.NewProvider(etwProviderName, etwCallback)
	if err != nil {
		logrus.Error(err)
	} else {
		if hook, err := etwlogrus.NewHookFromProvider(provider); err == nil {
			logrus.AddHook(hook)
		} else {
			logrus.Error(err)
		}
	}

	// Write the "ShimLaunched" event with the shim's command-line arguments.
	_ = provider.WriteEvent(
		"ShimLaunched",
		nil,
		etw.WithFields(
			etw.StringArray("Args", os.Args),
			etw.StringField("Version", hcsversion.Version),
			etw.StringField("GitCommit", hcsversion.Commit),
		),
	)

	// Register the shim's TTRPC plugin with the containerd plugin registry.
	// The plugin depends on the event publisher (for publishing task/sandbox
	// events to containerd) and the internal shutdown service (for co-ordinated
	// graceful teardown).
	registry.Register(&plugin.Registration{
		Type: plugins.TTRPCPlugin,
		ID:   "shim-services",
		Requires: []plugin.Type{
			plugins.EventPlugin,
			plugins.InternalPlugin,
		},
		InitFn: func(ic *plugin.InitContext) (interface{}, error) {
			pp, err := ic.GetByID(plugins.EventPlugin, "publisher")
			if err != nil {
				return nil, err
			}
			ss, err := ic.GetByID(plugins.InternalPlugin, "shutdown")
			if err != nil {
				return nil, err
			}
			// We will register all the services namely-
			// 1. Sandbox service
			// 2. Task service
			// 3. Shimdiag service
			svc = service.NewService(
				ic.Context,
				pp.(shim.Publisher),
				ss.(shutdown.Service),
			)

			return svc, nil
		},
	})
}

// etwCallback is the ETW callback method for this shim.
//
// On a CaptureState notification (triggered by tools such as wpr or xperf) it
// dumps all goroutine stacks – both host-side Go stacks and, when available,
// the guest Linux stacks – to the logrus logger tagged with the sandbox ID.
// This provides an out-of-band diagnostic snapshot without requiring the shim
// to be paused or restarted.
func etwCallback(sourceID guid.GUID, state etw.ProviderState, level etw.Level, matchAnyKeyword uint64, matchAllKeyword uint64, filterData uintptr) {
	if state == etw.ProviderStateCaptureState {
		if svc == nil {
			logrus.Warn("service not initialized")
			return
		}
		resp, err := svc.DiagStacks(context.Background(), &shimdiag.StacksRequest{})
		if err != nil {
			return
		}
		log := logrus.WithField("sandboxID", svc.SandboxID())
		log.WithField("stack", resp.Stacks).Info("goroutine stack dump")
		if resp.GuestStacks != "" {
			log.WithField("stack", resp.GuestStacks).Info("guest stack dump")
		}
	}
}
