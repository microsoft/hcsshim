//go:build windows

package bridge

import (
	"encoding/json"
	"testing"

	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils/etw"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

func TestSidecarLogProviderPolicy(t *testing.T) {
	for _, allow := range []bool{false, true} {
		var enforcer securitypolicy.SecurityPolicyEnforcer = &securitypolicy.ClosedDoorSecurityPolicyEnforcer{}
		if allow {
			enforcer = &securitypolicy.OpenDoorSecurityPolicyEnforcer{}
		}
		b := newTestBridge(enforcer)
		req := newModifyServiceSettingsRequest(buildLogForwardServiceRequest(t, etw.SidecarProviderName))
		err := b.modifyServiceSettings(req)
		if !allow {
			if err == nil {
				t.Fatal("denied sidecar subscription succeeded")
			}
			select {
			case <-b.sendToGCSCh:
				t.Fatal("denied subscription forwarded")
			default:
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		select {
		case forwarded := <-b.sendToGCSCh:
			settings := &guestrequest.LogForwardServiceRPCRequest{}
			message := prot.ServiceModificationRequest{Settings: settings}
			if err := json.Unmarshal(forwarded.message, &message); err != nil {
				t.Fatal(err)
			}
			sources, err := etw.DecodeAndUnmarshalLogSources(settings.Settings)
			if err != nil {
				t.Fatal(err)
			}
			providers := sources.LogConfig.Sources[0].Providers
			if len(providers) != 1 || providers[0].ProviderGUID != etw.GetProviderGUIDFromName(etw.SidecarProviderName) {
				t.Fatalf("unexpected forwarded providers: %+v", providers)
			}
		default:
			t.Fatal("approved subscription not forwarded")
		}
	}
}
