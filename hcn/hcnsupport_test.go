// +build integration

package hcn

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSupport(t *testing.T) {
	supportedFeatures := GetSupportedFeatures()
	jsonString, err := json.Marshal(supportedFeatures)
	if err != nil {
		t.Error(err)
	}
	fmt.Printf("Supported Features:\n%s \n", jsonString)
	if supportedFeatures.Api.V2 != true {
		t.Errorf("No V2 Support found")
	}
}
