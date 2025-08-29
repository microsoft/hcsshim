//go:build windows
// +build windows

package bridge

import (
	"encoding/json"
	"fmt"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/bridgeutils/commonutils"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

func unmarshalContainerModifySettings(req *request) (_ *prot.ContainerModifySettings, err error) {
	ctx, span := oc.StartSpan(req.ctx, "sidecar::unmarshalContainerModifySettings")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.ContainerModifySettings
	var requestRawSettings json.RawMessage
	r.Request = &requestRawSettings
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return nil, fmt.Errorf("failed to unmarshal rpcModifySettings: %w", err)
	}

	var modifyGuestSettingsRequest guestrequest.ModificationRequest
	var rawGuestRequest json.RawMessage
	modifyGuestSettingsRequest.Settings = &rawGuestRequest
	if err := commonutils.UnmarshalJSONWithHresult(requestRawSettings, &modifyGuestSettingsRequest); err != nil {
		return nil, fmt.Errorf("invalid rpcModifySettings ModificationRequest: %w", err)
	}

	if modifyGuestSettingsRequest.RequestType == "" {
		modifyGuestSettingsRequest.RequestType = guestrequest.RequestTypeAdd
	}

	if modifyGuestSettingsRequest.ResourceType != "" {
		switch modifyGuestSettingsRequest.ResourceType {
		case guestresource.ResourceTypeCWCOWCombinedLayers:
			settings := &guestresource.CWCOWCombinedLayers{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, settings); err != nil {
				return nil, fmt.Errorf("invalid ResourceTypeCWCOWCombinedLayers request: %w", err)
			}
			modifyGuestSettingsRequest.Settings = settings

		case guestresource.ResourceTypeCombinedLayers:
			settings := &guestresource.WCOWCombinedLayers{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, settings); err != nil {
				return nil, fmt.Errorf("invalid ResourceTypeCombinedLayers request: %w", err)
			}
			modifyGuestSettingsRequest.Settings = settings

		case guestresource.ResourceTypeNetworkNamespace:
			settings := &hcn.HostComputeNamespace{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, settings); err != nil {
				return nil, fmt.Errorf("invalid ResourceTypeNetworkNamespace request: %w", err)
			}
			modifyGuestSettingsRequest.Settings = settings

		case guestresource.ResourceTypeNetwork:
			settings := &guestrequest.NetworkModifyRequest{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, settings); err != nil {
				return nil, fmt.Errorf("invalid ResourceTypeNetwork request: %w", err)
			}
			modifyGuestSettingsRequest.Settings = settings

		case guestresource.ResourceTypeMappedVirtualDisk:
			wcowMappedVirtualDisk := &guestresource.WCOWMappedVirtualDisk{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, wcowMappedVirtualDisk); err != nil {
				return nil, fmt.Errorf("invalid ResourceTypeMappedVirtualDisk request: %w", err)
			}
			modifyGuestSettingsRequest.Settings = wcowMappedVirtualDisk

		case guestresource.ResourceTypeHvSocket:
			hvSocketAddress := &hcsschema.HvSocketAddress{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, hvSocketAddress); err != nil {
				return nil, fmt.Errorf("invalid ResourceTypeHvSocket request: %w", err)
			}
			modifyGuestSettingsRequest.Settings = hvSocketAddress

		case guestresource.ResourceTypeMappedDirectory:
			settings := &hcsschema.MappedDirectory{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, settings); err != nil {
				return nil, fmt.Errorf("invalid ResourceTypeMappedDirectory request: %w", err)
			}
			modifyGuestSettingsRequest.Settings = settings

		case guestresource.ResourceTypeSecurityPolicy:
			securityPolicyRequest := &guestresource.WCOWConfidentialOptions{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, securityPolicyRequest); err != nil {
				return nil, fmt.Errorf("invalid ResourceTypeSecurityPolicy request: %w", err)
			}
			modifyGuestSettingsRequest.Settings = securityPolicyRequest

		case guestresource.ResourceTypeMappedVirtualDiskForContainerScratch:
			wcowMappedVirtualDisk := &guestresource.WCOWMappedVirtualDisk{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, wcowMappedVirtualDisk); err != nil {
				return nil, fmt.Errorf("invalid ResourceTypeMappedVirtualDiskForContainerScratch request: %w", err)
			}
			modifyGuestSettingsRequest.Settings = wcowMappedVirtualDisk

		case guestresource.ResourceTypeWCOWBlockCims:
			wcowBlockCimMounts := &guestresource.CWCOWBlockCIMMounts{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, wcowBlockCimMounts); err != nil {
				return nil, fmt.Errorf("invalid ResourceTypeWCOWBlockCims request: %w", err)
			}
			modifyGuestSettingsRequest.Settings = wcowBlockCimMounts

		default:
			// Invalid request
			log.G(ctx).Errorf("Invald modifySettingsRequest: %v", modifyGuestSettingsRequest.ResourceType)
			return nil, fmt.Errorf("invald modifySettingsRequest")
		}
	}
	r.Request = &modifyGuestSettingsRequest
	return &r, nil
}
