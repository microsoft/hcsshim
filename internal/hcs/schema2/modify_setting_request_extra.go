package hcsschema

import "fmt"

// NewModifySettingRequest creates a [ModifySettingRequest] with the provided settings to use
// with the [HcsModifyComputeSystem] HCS API call.
//
// [HcsModifyComputeSystem]: https://learn.microsoft.com/en-us/virtualization/api/hcs/reference/hcsmodifycomputesystem
func NewModifySettingRequest(
	path string,
	reqType ModifyRequestType,
	settings any,
	guestRequest any,
) (ModifySettingRequest, error) {
	switch reqType {
	case ModifyRequestType_ADD,
		ModifyRequestType_REMOVE,
		ModifyRequestType_UPDATE:
	default:
		return ModifySettingRequest{}, fmt.Errorf("unsupported ModifyRequestType: %s", reqType)
	}

	s, err := ToRawMessage(settings)
	if err != nil {
		return ModifySettingRequest{},
			fmt.Errorf("encode %s ModifySettingRequest settings (%+v) at path %q to json: %w", string(reqType), path, settings, err)
	}

	gr, err := ToRawMessage(guestRequest)
	if err != nil {
		return ModifySettingRequest{},
			fmt.Errorf("encode ModifySettingRequest guest request (%+v) to json: %w", guestRequest, err)
	}

	return ModifySettingRequest{
		ResourcePath: path,
		RequestType:  &reqType,
		Settings:     s,
		GuestRequest: gr,
	}, nil
}

// NewModifySettingGuestRequest creates a [ModifySettingRequest] with the provided guestRequest.
func NewModifySettingGuestRequest(guestRequest any) (ModifySettingRequest, error) {
	gr, err := ToRawMessage(guestRequest)
	if err != nil {
		return ModifySettingRequest{},
			fmt.Errorf("encode ModifySettingRequest guest request (%+v) to json: %w", guestRequest, err)
	}

	return ModifySettingRequest{GuestRequest: gr}, nil
}

func (r *ModifySettingRequest) ValidGuestRequest() bool {
	return r != nil && r.GuestRequest != nil || len(*r.GuestRequest) > 0
}
