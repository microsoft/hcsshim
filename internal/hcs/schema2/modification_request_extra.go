package hcsschema

import "fmt"

// NewModificationRequest creates a new [ModificationRequest] with the provided settings to
// use with the [HcsModifyServiceSettings] HCS API call.
//
// [HcsModifyServiceSettings]: https://learn.microsoft.com/en-us/virtualization/api/hcs/reference/hcsmodifyservicesettings
func NewModificationRequest(pt ModifyPropertyType, settings any) (ModificationRequest, error) {
	switch pt {
	case ModifyPropertyType_CPU_GROUP:
		if _, ok := settings.(HostProcessorModificationRequest); !ok {
			return ModificationRequest{},
				fmt.Errorf("ModifyPropertyType %s requires settings type 'HostProcessorModificationRequest': received %T", pt, settings)
		}
	case ModifyPropertyType_CONTAINER_CREDENTIAL_GUARD:
		if _, ok := settings.(ContainerCredentialGuardOperationRequest); !ok {
			return ModificationRequest{},
				fmt.Errorf("ModifyPropertyType %s requires settings type 'ContainerCredentialGuardOperationRequest': received %T", pt, settings)
		}
	default:
		return ModificationRequest{}, fmt.Errorf("unsupported ModifyPropertyType: %s", pt)
	}

	s, err := ToRawMessage(settings)
	if err != nil {
		return ModificationRequest{},
			fmt.Errorf("encode %s modification settings (%+v) to json: %w", string(pt), settings, err)
	}

	return ModificationRequest{
		PropertyType: &pt,
		Settings:     s,
	}, nil
}
