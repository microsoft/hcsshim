package cfgmgr

import (
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/pkg/errors"
)

//go:generate go run ../../mksyscall_windows.go -output zsyscall_windows.go cfgmgr.go

//sys cmGetDeviceIDListSize(pulLen *uint32, pszFilter *byte, uFlags uint64) (hr error) = cfgmgr32.CM_Get_Device_ID_List_SizeA
//sys cmGetDeviceIDList(pszFilter *byte, buffer *byte, bufferLen uint64, uFlags uint64) (hr error)= cfgmgr32.CM_Get_Device_ID_ListA
//sys cmLocateDevNode(pdnDevInst *uint32, pDeviceID string, uFlags uint64) (hr error) = cfgmgr32.CM_Locate_DevNodeW
//sys cmGetDevNodeProperty(dnDevInst uint32, propertyKey *devPropKey, propertyType *uint64, propertyBuffer *uint16, propertyBufferSize *uint64, uFlags uint64) (hr error) = cfgmgr32.CM_Get_DevNode_PropertyW

type devPropKey struct {
	fmtid guid.GUID
	pid   uint64
}

const (
	CM_GETIDLIST_FILTER_NONE               = uint64(0x00000000)
	CM_GETIDLIST_FILTER_ENUMERATOR         = uint64(0x00000001)
	CM_GETIDLIST_FILTER_SERVICE            = uint64(0x00000002)
	CM_GETIDLIST_FILTER_EJECTRELATIONS     = uint64(0x00000004)
	CM_GETIDLIST_FILTER_REMOVALRELATIONS   = uint64(0x00000008)
	CM_GETIDLIST_FILTER_POWERRELATIONS     = uint64(0x00000010)
	CM_GETIDLIST_FILTER_BUSRELATIONS       = uint64(0x00000020)
	CM_GETIDLIST_DONOTGENERATE             = uint64(0x10000040)
	CM_GETIDLIST_FILTER_TRANSPORTRELATIONS = uint64(0x00000080)
	CM_GETIDLIST_FILTER_PRESENT            = uint64(0x00000100)
	CM_GETIDLIST_FILTER_CLASS              = uint64(0x00000200)
	CM_GETIDLIST_FILTER_BITS               = uint64(0x100003FF)

	CM_LOCATE_DEVNODE_NORMAL       = uint64(0x00000000)
	CM_LOCATE_DEVNODE_PHANTOM      = uint64(0x00000001)
	CM_LOCATE_DEVNODE_CANCELREMOVE = uint64(0x00000002)
	CM_LOCATE_DEVNODE_NOVALIDATION = uint64(0x00000004)

	DEVPROP_TYPE_STRING        = uint64(0x00000012)
	DEVPROP_TYPEMOD_LIST       = uint64(0x00002000)
	DEVPROP_TYPE_STRING_LIST   = uint64(DEVPROP_TYPE_STRING | DEVPROP_TYPEMOD_LIST)
	DEVPKEY_LOCATIONPATHS_GUID = "a45c254e-df1c-4efd-8020-67d146a850e0"

	STRING_BUFFER_SIZE = uint64(1024)
)

func getDevPKeyDeviceLocationPaths() (*devPropKey, error) {
	guid, err := guid.FromString(DEVPKEY_LOCATIONPATHS_GUID)
	if err != nil {
		return nil, err
	}
	return &devPropKey{
		fmtid: guid,
		pid:   37,
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
		err = cmLocateDevNode(&devNodeInst, id, CM_LOCATE_DEVNODE_NORMAL)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to locate device node for %s", id)
		}
		propertyType := uint64(0)
		propertyBufferSize := STRING_BUFFER_SIZE
		var propertyBuffer [STRING_BUFFER_SIZE]uint16
		err = cmGetDevNodeProperty(devNodeInst, devPKeyDeviceLocationPaths, &propertyType, &propertyBuffer[0], &propertyBufferSize, 0)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get location path property from device node for %s with", id)
		}
		if propertyType != DEVPROP_TYPE_STRING_LIST {
			return nil, fmt.Errorf("expected to return property type DEVPROP_TYPE_STRING_LIST %d, instead got %d", DEVPROP_TYPE_STRING_LIST, propertyType)
		}
		if propertyBufferSize > STRING_BUFFER_SIZE {
			return nil, fmt.Errorf("location path is too long, size %d", propertyBufferSize)
		}
		formattedResult := convertFirstNullTerminatedValueToString(propertyBuffer[:propertyBufferSize])
		result = append(result, formattedResult)
	}

	return result, nil
}

// helper function that finds the first \u0000 rune and returns the wide string as a regular go string
func convertFirstNullTerminatedValueToString(buf []uint16) string {
	r := utf16.Decode(buf)
	converted := string(r)
	zerosIndex := strings.IndexRune(converted, '\u0000')
	if zerosIndex != -1 {
		converted = converted[:zerosIndex]
	}
	return converted
}

func GetChildrenFromInstanceIDs(parentIDs []string) ([]string, error) {
	var result []string
	for _, id := range parentIDs {
		var devNodeInst uint32
		if err := cmLocateDevNode(&devNodeInst, id, CM_LOCATE_DEVNODE_NORMAL); err != nil {
			return nil, err
		}
		pszFilterParentID := []byte(id)
		children, err := getDeviceIDList(&pszFilterParentID[0], CM_GETIDLIST_FILTER_BUSRELATIONS)
		if err != nil {
			return nil, err
		}
		result = append(result, children...)
	}
	return result, nil
}

func getDeviceIDList(pszFilter *byte, ulFlags uint64) ([]string, error) {
	listLength := uint32(0)
	if err := cmGetDeviceIDListSize(&listLength, pszFilter, ulFlags); err != nil {
		return nil, err
	}
	if listLength == 0 {
		return []string{}, nil
	}
	buf := make([]byte, uint64(listLength))
	if err := cmGetDeviceIDList(pszFilter, &buf[0], uint64(listLength), ulFlags); err != nil {
		return nil, err
	}

	var result []string
	prev := 0
	for i, c := range buf {
		if c == 0 {
			// this is a null character, we've seen a string
			if buf[prev] != 0 {
				result = append(result, string(buf[prev:i]))
			}
			// don't include the null character
			prev = i + 1
		}
	}

	return result, nil
}
