//go:build windows

package hns

import (
	"errors"
	"testing"
)

func TestHNSErrorPreservation(t *testing.T) {
	// Mock the raw response
	originalMock := hnsCallRawResponseMock
	defer func() { hnsCallRawResponseMock = originalMock }()

	hnsCallRawResponseMock = func(method, path, request string) (*hnsResponse, error) {
		return &hnsResponse{
			Success:   false,
			Error:     "vSwitch in transient state",
			ErrorCode: 0x803b0013, // HCS_E_VMSWITCH_IN_TRANSIENT_STATE
		}, nil
	}

	// Execute hnsCall
	err := hnsCall("POST", "/endpoints", "{}", nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	// Assert errors.As
	var hnsErr *HNSError
	if !errors.As(err, &hnsErr) {
		t.Fatalf("expected error to be of type *HNSError, got %T: %v", err, err)
	}

	if hnsErr.ErrorCode != 0x803b0013 {
		t.Errorf("expected ErrorCode 0x803b0013, got %#x", hnsErr.ErrorCode)
	}

	if hnsErr.ErrorString != "vSwitch in transient state" {
		t.Errorf("expected ErrorString 'vSwitch in transient state', got %s", hnsErr.ErrorString)
	}
}
