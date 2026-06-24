package commonutils

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	"github.com/sirupsen/logrus"
)

type ErrorRecord struct {
	Result       int32 // HResult
	Message      string
	StackTrace   string `json:",omitempty"`
	ModuleName   string
	FileName     string
	Line         uint32
	FunctionName string `json:",omitempty"`
}

// UnmarshalJSONWithHresult unmarshals the given data into the given interface, and
// wraps any error returned in an HRESULT error.
func UnmarshalJSONWithHresult(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return gcserr.WrapHresult(err, gcserr.HrVmcomputeInvalidJSON)
	}
	return nil
}

// DecodeJSONWithHresult decodes the JSON from the given reader into the given
// interface, and wraps any error returned in an HRESULT error.
func DecodeJSONWithHresult(r io.Reader, v interface{}) error {
	if err := json.NewDecoder(r).Decode(v); err != nil {
		return gcserr.WrapHresult(err, gcserr.HrVmcomputeInvalidJSON)
	}
	return nil
}

func SetErrorForResponseBaseUtil(errForResponse error, moduleName string) (hresult gcserr.Hresult, errorMessage string, newRecord ErrorRecord) {
	errorMessage = errForResponse.Error()
	stackString := ""
	fileName := ""
	// We use -1 as a sentinel if no line number found (or it cannot be parsed),
	// but that will ultimately end up as [math.MaxUint32], so set it to that explicitly.
	// (Still keep using -1 for backwards compatibility ...)
	lineNumber := uint32(math.MaxUint32)
	functionName := ""
	if stack := gcserr.BaseStackTrace(errForResponse); stack != nil {
		bottomFrame := stack[0]
		stackString = fmt.Sprintf("%+v", stack)
		fileName = fmt.Sprintf("%s", bottomFrame)
		lineNumberStr := fmt.Sprintf("%d", bottomFrame)
		if n, err := strconv.ParseUint(lineNumberStr, 10, 32); err == nil {
			lineNumber = uint32(n)
		} else {
			logrus.WithFields(logrus.Fields{
				"line-number":   lineNumberStr,
				logrus.ErrorKey: err,
			}).Error("opengcs::bridge::setErrorForResponseBase - failed to parse line number, using -1 instead")
		}
		functionName = fmt.Sprintf("%n", bottomFrame)
	}
	hresult, err := gcserr.GetHresult(errForResponse)
	if err != nil {
		// Default to using the generic failure HRESULT.
		hresult = gcserr.HrFail
	}

	newRecord = ErrorRecord{
		Result:       int32(hresult),
		Message:      errorMessage,
		StackTrace:   stackString,
		ModuleName:   moduleName,
		FileName:     fileName,
		Line:         lineNumber,
		FunctionName: functionName,
	}

	return hresult, errorMessage, newRecord
}
