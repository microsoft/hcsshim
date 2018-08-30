package hcsshimtest

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim"
)

func TestSupport(t *testing.T) {
	supportedFeatures := hcsshim.GetHcnSupportedFeatures()
	jsonString, err := json.Marshal(supportedFeatures)
	if err != nil {
		t.Error(err)
	}
	fmt.Printf("Supported Features:\n%s \n", jsonString)
	if supportedFeatures.Api.V2 != true {
		t.Errorf("No V2 Support found")
	}
}
