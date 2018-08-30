package hcnshim

import (
	"encoding/json"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcserror"
	"github.com/Microsoft/hcsshim/internal/interop"
	"github.com/sirupsen/logrus"
)

// HNSGlobals are all global properties of the HNS Service.
type HNSGlobals struct {
	Version HNSVersion `json:"Version"`
}

// HNSVersion is the HNS Service version.
type HNSVersion struct {
	Major int `json:"Major"`
	Minor int `json:"Minor"`
}

var (
	// HNSVersion1803 added ACL functionality.
	HNSVersion1803 = HNSVersion{Major: 7, Minor: 2}
	// HNSV2ApiSupport added support for the V2 Api/Schema
	HNSV2ApiSupport = HNSVersion{Major: 9, Minor: 1}
)

// GetHNSGlobals returns the global properties of the HNS Service.
func GetHNSGlobals() (*HNSGlobals, error) {
	var version HNSVersion
	err := hnsCall("GET", "/globals/version", "", &version)
	if err != nil {
		return nil, err
	}

	globals := &HNSGlobals{
		Version: version,
	}

	return globals, nil
}

type hnsResponse struct {
	Success bool
	Error   string
	Output  json.RawMessage
}

func hnsCall(method, path, request string, returnResponse interface{}) error {
	var responseBuffer *uint16
	logrus.Debugf("[%s]=>[%s] Request : %s", method, path, request)

	err := _hnsCall(method, path, request, &responseBuffer)
	if err != nil {
		return hcserror.New(err, "hnsCall ", "")
	}
	response := interop.ConvertAndFreeCoTaskMemString(responseBuffer)

	hnsresponse := &hnsResponse{}
	if err = json.Unmarshal([]byte(response), &hnsresponse); err != nil {
		return err
	}

	if !hnsresponse.Success {
		return fmt.Errorf("HNS failed with error : %s", hnsresponse.Error)
	}

	if len(hnsresponse.Output) == 0 {
		return nil
	}

	logrus.Debugf("Network Response : %s", hnsresponse.Output)
	err = json.Unmarshal(hnsresponse.Output, returnResponse)
	if err != nil {
		return err
	}

	return nil
}
