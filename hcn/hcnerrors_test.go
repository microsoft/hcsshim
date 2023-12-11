//go:build windows && integration
// +build windows,integration

package hcn

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcserror"
)

func TestMissingNetworkByName(t *testing.T) {
	_, err := GetNetworkByName("Not found name")
	if err == nil {
		t.Fatal("Error was not thrown.")
	}
	if !IsNotFoundError(err) {
		t.Fatal("Unrelated error was thrown.", err)
	}
	if _, ok := err.(NetworkNotFoundError); !ok { //nolint:errorlint
		t.Fatal("Wrong error type was thrown.", err)
	}
	if err.Error() != `Network name "Not found name" not found` {
		t.Fatal("Wrong error message was returned", err.Error())
	}
}

func TestMissingNetworkById(t *testing.T) {
	// Random guid
	_, err := GetNetworkByID("5f0b1190-63be-4e0c-b974-bd0f55675a42")
	if err == nil {
		t.Fatal("Error was not thrown.")
	}
	if !IsNotFoundError(err) {
		t.Fatal("Unrelated error was thrown.", err)
	}
	if _, ok := err.(NetworkNotFoundError); !ok { //nolint:errorlint
		t.Fatal("Wrong error type was thrown.", err)
	}
	if err.Error() != `Network ID "5f0b1190-63be-4e0c-b974-bd0f55675a42" not found` {
		t.Fatal("Wrong error message was returned", err.Error())
	}
}

func TestMissingNamespaceById(t *testing.T) {
	// Random guid
	_, err := GetNamespaceByID("5f0b1190-63be-4e0c-b974-bd0f55675a42")
	if err == nil {
		t.Fatal("Error was not thrown.")
	}

	if !IsNotFoundError(err) {
		t.Fatal("Unrelated error was thrown.", err)
	}
	if _, ok := err.(NamespaceNotFoundError); !ok { //nolint:errorlint
		t.Fatal("Wrong error type was thrown.", err)
	}
	if err.Error() != `Namespace ID "5f0b1190-63be-4e0c-b974-bd0f55675a42" not found` {
		t.Fatal("Wrong error message was returned.", err.Error())
	}
}

func TestEndpointAlreadyExistsError(t *testing.T) {
	testNetwork, err := CreateTestOverlayNetwork()
	if err != nil {
		t.Fatal("Failed to create overlay network for setup.", err)
	}
	defer testNetwork.Delete() //nolint:errcheck
	portMappingSetting := PortMappingPolicySetting{
		Protocol:     17,
		InternalPort: 45678,
		ExternalPort: 56789,
	}
	settingString, _ := json.Marshal(portMappingSetting)
	portMappingPolicy := EndpointPolicy{
		Type:     PortMapping,
		Settings: settingString,
	}

	endpoint, err := HcnCreateTestEndpointWithPolicies(testNetwork, []EndpointPolicy{portMappingPolicy})
	if err != nil {
		t.Fatal("Failed to create endpoint for setup.", err)
	}
	defer endpoint.Delete() //nolint:errcheck

	endpoint2, err := HcnCreateTestEndpointWithPolicies(testNetwork, []EndpointPolicy{portMappingPolicy})
	if err == nil {
		_ = endpoint2.Delete()
		t.Fatal("Endpoint should have failed with duplicate port mapping.", err)
	}

	if !IsPortAlreadyExistsError(err) {
		t.Fatal("Error is not a PortAlreadyExists Error", err)
	}
}

func TestNotFoundError(t *testing.T) {
	namespaceGuid, _ := guid.FromString("5f0b1190-63be-4e0c-b974-bd0f55675a42")
	_, err := getNamespace(namespaceGuid, "")
	if !IsElementNotFoundError(err) {
		t.Fatal("Error is not a ElementNotFound Error.", err)
	}
}

func TestIsNotFoundError(t *testing.T) {
	for _, tc := range []struct {
		err        error
		isNotFound bool
	}{
		// not-found errors
		{
			err:        NetworkNotFoundError{},
			isNotFound: true,
		},
		{
			err:        EndpointNotFoundError{},
			isNotFound: true,
		},
		{
			err:        NamespaceNotFoundError{},
			isNotFound: true,
		},
		{
			err:        LoadBalancerNotFoundError{},
			isNotFound: true,
		},
		{
			err:        RouteNotFoundError{},
			isNotFound: true,
		},
		{
			err: &hcserror.HcsError{
				Err: hcs.ErrElementNotFound,
			},
			isNotFound: true,
		},
		{
			err: &hcserror.HcsError{
				Err: windows.ERROR_NOT_FOUND,
			},
			isNotFound: true,
		},

		// not-not-found errors
		{err: &hcserror.HcsError{Err: hcs.ErrInvalidData}},
		{err: &hcserror.HcsError{}},
		{err: fmt.Errorf("random error")},
		{err: windows.ERROR_SUCCESS},
	} {
		// go doesn't have an xor operator :'(
		if b := IsNotFoundError(tc.err); (!b && tc.isNotFound) || (b && !tc.isNotFound) {
			t.Errorf("IsNotFoundError did not return %t for %T: %v", tc.isNotFound, tc.err, tc.err)
		}
		err := fmt.Errorf("wrapping error with another error: %w", tc.err)
		if b := IsNotFoundError(err); (!b && tc.isNotFound) || (b && !tc.isNotFound) {
			t.Errorf("IsNotFoundError did not return %t for wrapped %T: %v", tc.isNotFound, tc.err, tc.err)
		}
	}
}
