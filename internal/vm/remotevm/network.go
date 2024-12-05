//go:build windows

package remotevm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/vmservice"
)

func getSwitchID(endpointID, portID string) (string, error) {
	// Get updated endpoint with new fields (need switch ID)
	ep, err := hcn.GetEndpointByID(endpointID)
	if err != nil {
		return "", fmt.Errorf("failed to get endpoint %q: %w", endpointID, err)
	}

	type ExtraInfo struct {
		Allocators []struct {
			SwitchID         string `json:"SwitchId"`
			EndpointPortGUID string `json:"EndpointPortGuid"`
		}
	}

	var exi ExtraInfo
	if err := json.Unmarshal(ep.Health.Extra.Resources, &exi); err != nil {
		return "", fmt.Errorf("failed to unmarshal resource data from endpoint %q: %w", endpointID, err)
	}

	if len(exi.Allocators) == 0 {
		return "", errors.New("no resource data found for endpoint")
	}

	// NIC should only ever belong to one switch but there are cases where there's more than one allocator
	// in the returned data. It seems they only ever contain empty strings so search for the first entry
	// that actually contains a switch ID and that has the matching port GUID we made earlier.
	var switchID string
	for _, allocator := range exi.Allocators {
		if allocator.SwitchID != "" && strings.ToLower(allocator.EndpointPortGUID) == portID {
			switchID = allocator.SwitchID
			break
		}
	}
	return switchID, nil
}

func (uvm *utilityVM) AddNIC(ctx context.Context, nicID, endpointID, macAddr string) error {
	portID, err := guid.NewV4()
	if err != nil {
		return fmt.Errorf("failed to generate guid for port: %w", err)
	}

	vmEndpointRequest := hcn.VmEndpointRequest{
		PortId:           portID,
		VirtualNicName:   fmt.Sprintf("%s--%s", nicID, portID),
		VirtualMachineId: guid.GUID{},
	}

	m, err := json.Marshal(vmEndpointRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal endpoint request json: %w", err)
	}

	if err := hcn.ModifyEndpointSettings(endpointID, &hcn.ModifyEndpointSettingRequest{
		ResourceType: hcn.EndpointResourceTypePort,
		RequestType:  hcn.RequestTypeAdd,
		Settings:     json.RawMessage(m),
	}); err != nil {
		return fmt.Errorf("failed to configure switch port: %w", err)
	}

	switchID, err := getSwitchID(endpointID, portID.String())
	if err != nil {
		return err
	}

	nic := &vmservice.NICConfig{
		NicID:      nicID,
		MacAddress: macAddr,
		PortID:     portID.String(),
		SwitchID:   switchID,
	}

	if _, err := uvm.client.ModifyResource(ctx,
		&vmservice.ModifyResourceRequest{
			Type: vmservice.ModifyType_ADD,
			Resource: &vmservice.ModifyResourceRequest_NicConfig{
				NicConfig: nic,
			},
		},
	); err != nil {
		return fmt.Errorf("failed to add network adapter: %w", err)
	}

	return nil
}

func (uvm *utilityVM) RemoveNIC(ctx context.Context, nicID, endpointID, macAddr string) error {
	nic := &vmservice.NICConfig{
		NicID:      nicID,
		MacAddress: macAddr,
	}

	if _, err := uvm.client.ModifyResource(ctx,
		&vmservice.ModifyResourceRequest{
			Type: vmservice.ModifyType_REMOVE,
			Resource: &vmservice.ModifyResourceRequest_NicConfig{
				NicConfig: nic,
			},
		},
	); err != nil {
		return fmt.Errorf("failed to remove network adapter: %w", err)
	}

	return nil
}
