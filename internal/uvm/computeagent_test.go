package uvm

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/computeagent"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/hns"
)

type testUtilityVM struct{}

func (t *testUtilityVM) AddEndpointToNSWithID(ctx context.Context, nsID, nicID string, endpoint *hns.HNSEndpoint) error {
	return nil
}

func (t *testUtilityVM) RemoveEndpointFromNS(ctx context.Context, id string, endpoint *hns.HNSEndpoint) error {
	return nil
}

func (t *testUtilityVM) UpdateNIC(ctx context.Context, id string, settings *hcsschema.NetworkAdapter) error {
	return nil
}

func TestAddNIC(t *testing.T) {
	ctx := context.Background()

	agent := &computeAgent{
		uvm: &testUtilityVM{},
	}

	hnsGetHNSEndpointByName = func(endpointName string) (*hns.HNSEndpoint, error) {
		return &hns.HNSEndpoint{
			Namespace: &hns.Namespace{ID: t.Name() + "-namespaceID"},
		}, nil
	}

	var (
		testNICID        = t.Name() + "-nicID"
		testEndpointName = t.Name() + "-endpoint"
	)

	type config struct {
		name          string
		nicID         string
		endpointName  string
		errorExpected bool
	}
	tests := []config{
		{
			name:          "AddNIC returns no error",
			nicID:         testNICID,
			endpointName:  testEndpointName,
			errorExpected: false,
		},
		{
			name:          "AddNIC returns error with blank nic ID",
			nicID:         "",
			endpointName:  testEndpointName,
			errorExpected: true,
		},
		{
			name:          "AddNIC returns error with blank endpoint name",
			nicID:         testNICID,
			endpointName:  "",
			errorExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {

			req := &computeagent.AddNICInternalRequest{
				NicID:        test.nicID,
				EndpointName: test.endpointName,
			}

			_, err := agent.AddNIC(ctx, req)
			if test.errorExpected && err == nil {
				t.Fatalf("expected AddNIC to return an error")
			}
			if !test.errorExpected && err != nil {
				t.Fatalf("expected AddNIC to return no error, instead got %v", err)
			}
		})
	}
}

func TestModifyNIC(t *testing.T) {
	ctx := context.Background()

	agent := &computeAgent{
		uvm: &testUtilityVM{},
	}

	hnsGetHNSEndpointByName = func(endpointName string) (*hns.HNSEndpoint, error) {
		return &hns.HNSEndpoint{
			Id:         t.Name() + "-endpoint-ID",
			MacAddress: "00-00-00-00-00-00",
		}, nil
	}

	var (
		testNICID        = t.Name() + "-nicID"
		testEndpointName = t.Name() + "-endpoint"
	)

	iovSettingsOn := &computeagent.IovSettings{
		IovOffloadWeight: 100,
	}

	type config struct {
		name          string
		nicID         string
		endpointName  string
		iovSettings   *computeagent.IovSettings
		errorExpected bool
	}
	tests := []config{
		{
			name:          "ModifyNIC returns no error",
			nicID:         testNICID,
			endpointName:  testEndpointName,
			iovSettings:   iovSettingsOn,
			errorExpected: false,
		},
		{
			name:          "ModifyNIC returns error with blank nic ID",
			nicID:         "",
			endpointName:  testEndpointName,
			iovSettings:   iovSettingsOn,
			errorExpected: true,
		},
		{
			name:          "ModifyNIC returns error with blank endpoint name",
			nicID:         testNICID,
			endpointName:  "",
			iovSettings:   iovSettingsOn,
			errorExpected: true,
		},
		{
			name:          "ModifyNIC returns error with nil IOV settings",
			nicID:         testNICID,
			endpointName:  testEndpointName,
			iovSettings:   nil,
			errorExpected: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			req := &computeagent.ModifyNICInternalRequest{
				NicID:             test.nicID,
				EndpointName:      test.endpointName,
				IovPolicySettings: test.iovSettings,
			}

			_, err := agent.ModifyNIC(ctx, req)
			if test.errorExpected && err == nil {
				t.Fatalf("expected ModifyNIC to return an error")
			}
			if !test.errorExpected && err != nil {
				t.Fatalf("expected ModifyNIC to return no error, instead got %v", err)
			}
		})
	}
}

func TestDeleteNIC(t *testing.T) {
	ctx := context.Background()

	agent := &computeAgent{
		uvm: &testUtilityVM{},
	}

	hnsGetHNSEndpointByName = func(endpointName string) (*hns.HNSEndpoint, error) {
		return &hns.HNSEndpoint{
			Namespace: &hns.Namespace{ID: "test-namespace-ID"},
		}, nil
	}

	var (
		testNICID        = t.Name() + "-nicID"
		testEndpointName = t.Name() + "-endpoint"
	)

	type config struct {
		name          string
		nicID         string
		endpointName  string
		errorExpected bool
	}
	tests := []config{
		{
			name:          "DeleteNIC returns no error",
			nicID:         testNICID,
			endpointName:  testEndpointName,
			errorExpected: false,
		},
		{
			name:          "DeleteNIC returns error with blank nic ID",
			nicID:         "",
			endpointName:  testEndpointName,
			errorExpected: true,
		},
		{
			name:          "DeleteNIC returns error with blank endpoint name",
			nicID:         testNICID,
			endpointName:  "",
			errorExpected: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			req := &computeagent.DeleteNICInternalRequest{
				NicID:        test.nicID,
				EndpointName: test.endpointName,
			}

			_, err := agent.DeleteNIC(ctx, req)
			if test.errorExpected && err == nil {
				t.Fatalf("expected DeleteNIC to return an error")
			}
			if !test.errorExpected && err != nil {
				t.Fatalf("expected DeleteNIC to return no error, instead got %v", err)
			}
		})
	}
}
