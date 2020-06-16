package windevice

import (
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/pkg/errors"
)

const (
	_CM_GETIDLIST_FILTER_BUSRELATIONS uint32 = 0x00000020

	_CM_LOCATE_DEVNODE_NORMAL uint32 = 0x00000000

	_DEVPROP_TYPE_STRING      uint32 = 0x00000012
	_DEVPROP_TYPEMOD_LIST     uint32 = 0x00002000
	_DEVPROP_TYPE_STRING_LIST uint32 = (_DEVPROP_TYPE_STRING | _DEVPROP_TYPEMOD_LIST)

	_DEVPKEY_LOCATIONPATHS_GUID = "a45c254e-df1c-4efd-8020-67d146a850e0"
)

// getDevPKeyDeviceLocationPaths creates a DEVPROPKEY struct for the
// DEVPKEY_Device_LocationPaths property as defined in devpkey.h
func getDevPKeyDeviceLocationPaths() (*winapi.DevPropKey, error) {
	guid, err := guid.FromString(_DEVPKEY_LOCATIONPATHS_GUID)
	if err != nil {
		return nil, err
	}
	return &winapi.DevPropKey{
		Fmtid: guid,
		// pid value is defined in devpkey.h
		Pid: 37,
	}, nil
}

func GetDeviceLocationPathsFromIDs(ids []string) ([]string, error) {
	result := []string{}
	devPKeyDeviceLocationPaths, err := getDevPKeyDeviceLocationPaths()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		var devNodeInst uint32
		err = winapi.CMLocateDevNode(&devNodeInst, id, _CM_LOCATE_DEVNODE_NORMAL)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to locate device node for %s", id)
		}
		propertyType := uint32(0)
		propertyBufferSize := uint32(0)

		// get the size of the property buffer by querying with a nil buffer and zeroed propertyBufferSize
		err = winapi.CMGetDevNodeProperty(devNodeInst, devPKeyDeviceLocationPaths, &propertyType, nil, &propertyBufferSize, 0)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get property buffer size of devnode query for %s with", id)
		}

		// get the property with the resulting propertyBufferSize
		propertyBuffer := make([]uint16, propertyBufferSize/2)
		err = winapi.CMGetDevNodeProperty(devNodeInst, devPKeyDeviceLocationPaths, &propertyType, &propertyBuffer[0], &propertyBufferSize, 0)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get location path property from device node for %s with", id)
		}
		if propertyType != _DEVPROP_TYPE_STRING_LIST {
			return nil, fmt.Errorf("expected to return property type DEVPROP_TYPE_STRING_LIST %d, instead got %d", _DEVPROP_TYPE_STRING_LIST, propertyType)
		}
		if int(propertyBufferSize/2) > len(propertyBuffer) {
			return nil, fmt.Errorf("location path is too large for the buffer, size in bytes %d", propertyBufferSize)
		}

		formattedResult, err := convertFirstNullTerminatedValueToString(propertyBuffer[:propertyBufferSize/2])
		if err != nil {
			return nil, err
		}
		result = append(result, formattedResult)
	}

	return result, nil
}

// helper function that finds the first \u0000 rune and returns the wide string as a regular go string
func convertFirstNullTerminatedValueToString(buf []uint16) (string, error) {
	r := utf16.Decode(buf)
	converted := string(r)
	zerosIndex := strings.IndexRune(converted, '\u0000')
	if zerosIndex == -1 {
		return "", errors.New("cannot convert value to string, malformed data passed")
	}
	return converted[:zerosIndex], nil
}

func GetChildrenFromInstanceIDs(parentIDs []string) ([]string, error) {
	var result []string
	for _, id := range parentIDs {
		pszFilterParentID := []byte(id)
		children, err := getDeviceIDList(&pszFilterParentID[0], _CM_GETIDLIST_FILTER_BUSRELATIONS)
		if err != nil {
			return nil, err
		}
		result = append(result, children...)
	}
	return result, nil
}

func getDeviceIDList(pszFilter *byte, ulFlags uint32) ([]string, error) {
	listLength := uint32(0)
	if err := winapi.CMGetDeviceIDListSize(&listLength, pszFilter, ulFlags); err != nil {
		return nil, err
	}
	if listLength == 0 {
		return []string{}, nil
	}
	buf := make([]byte, listLength)
	if err := winapi.CMGetDeviceIDList(pszFilter, &buf[0], uint32(listLength), ulFlags); err != nil {
		return nil, err
	}

	return winapi.ConvertStringSetToSlice(buf)
}
