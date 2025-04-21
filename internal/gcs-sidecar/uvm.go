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
	"github.com/pkg/errors"
)

func unmarshalContainerModifySettings(req *request) (_ *prot.ContainerModifySettings, err error) {
	ctx, span := oc.StartSpan(req.ctx, "sidecar::unmarshalContainerModifySettings")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.ContainerModifySettings
	var requestRawSettings json.RawMessage
	r.Request = &requestRawSettings
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal rpcModifySettings")
	}

	var modifyGuestSettingsRequest guestrequest.ModificationRequest
	var rawGuestRequest json.RawMessage
	modifyGuestSettingsRequest.Settings = &rawGuestRequest
	if err := commonutils.UnmarshalJSONWithHresult(requestRawSettings, &modifyGuestSettingsRequest); err != nil {
		return nil, errors.Wrap(err, "invalid rpcModifySettings ModificationRequest")
	}

	if modifyGuestSettingsRequest.RequestType == "" {
		modifyGuestSettingsRequest.RequestType = guestrequest.RequestTypeAdd
	}

	if modifyGuestSettingsRequest.ResourceType != "" {
		switch modifyGuestSettingsRequest.ResourceType {
		case guestresource.ResourceTypeCWCOWCombinedLayers:
			settings := &guestresource.CWCOWCombinedLayers{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, settings); err != nil {
				return nil, errors.Wrap(err, "invalid ResourceTypeCWCOWCombinedLayers request")
			}
			modifyGuestSettingsRequest.Settings = settings

		case guestresource.ResourceTypeCombinedLayers:
			settings := &guestresource.WCOWCombinedLayers{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, settings); err != nil {
				return nil, errors.Wrap(err, "invalid ResourceTypeCombinedLayers request")
			}
			modifyGuestSettingsRequest.Settings = settings

		case guestresource.ResourceTypeNetworkNamespace:
			settings := &hcn.HostComputeNamespace{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, settings); err != nil {
				return nil, errors.Wrap(err, "invalid ResourceTypeNetworkNamespace request")
			}
			modifyGuestSettingsRequest.Settings = settings

		case guestresource.ResourceTypeNetwork:
			settings := &guestrequest.NetworkModifyRequest{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, settings); err != nil {
				return nil, errors.Wrap(err, "invalid ResourceTypeNetwork request")
			}
			modifyGuestSettingsRequest.Settings = settings

		case guestresource.ResourceTypeMappedVirtualDisk:
			wcowMappedVirtualDisk := &guestresource.WCOWMappedVirtualDisk{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, wcowMappedVirtualDisk); err != nil {
				return nil, errors.Wrap(err, "invalid ResourceTypeMappedVirtualDisk request")
			}
			modifyGuestSettingsRequest.Settings = wcowMappedVirtualDisk

		case guestresource.ResourceTypeHvSocket:
			hvSocketAddress := &hcsschema.HvSocketAddress{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, hvSocketAddress); err != nil {
				return nil, errors.Wrap(err, "invalid ResourceTypeHvSocket request")
			}
			modifyGuestSettingsRequest.Settings = hvSocketAddress

		case guestresource.ResourceTypeMappedDirectory:
			settings := &hcsschema.MappedDirectory{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, settings); err != nil {
				return nil, errors.Wrap(err, "invalid ResourceTypeMappedDirectory request")
			}
			modifyGuestSettingsRequest.Settings = settings

		case guestresource.ResourceTypeSecurityPolicy:
			securityPolicyRequest := &guestresource.WCOWConfidentialOptions{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, securityPolicyRequest); err != nil {
				return nil, errors.Wrap(err, "invalid ResourceTypeSecurityPolicy request")
			}
			modifyGuestSettingsRequest.Settings = securityPolicyRequest

		case guestresource.ResourceTypeMappedVirtualDiskForContainerScratch:
			wcowMappedVirtualDisk := &guestresource.WCOWMappedVirtualDisk{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, wcowMappedVirtualDisk); err != nil {
				return nil, errors.Wrap(err, "invalid ResourceTypeMappedVirtualDiskForContainerScratch request")
			}
			modifyGuestSettingsRequest.Settings = wcowMappedVirtualDisk

		case guestresource.ResourceTypeWCOWBlockCims:
			wcowBlockCimMounts := &guestresource.WCOWBlockCIMMounts{}
			if err := commonutils.UnmarshalJSONWithHresult(rawGuestRequest, wcowBlockCimMounts); err != nil {
				return nil, errors.Wrap(err, "invalid ResourceTypeWCOWBlockCims request")
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
