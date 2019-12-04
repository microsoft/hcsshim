// +build integration

package hcn

import (
	"testing"

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
	if _, ok := err.(NetworkNotFoundError); !ok {
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
	if _, ok := err.(NetworkNotFoundError); !ok {
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
	if _, ok := err.(*hcserror.HcsError); !ok {
		t.Fatal("Wrong error type was thrown.", err)
	}
}
