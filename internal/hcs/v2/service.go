//go:build windows

package hcsv2

import (
	"context"
	"encoding/json"

	"github.com/Microsoft/hcsshim/internal/computecore"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// GetServiceProperties returns properties of the host compute service.
func GetServiceProperties(ctx context.Context, q hcsschema.PropertyQuery) (*hcsschema.ServiceProperties, error) {
	operation := "hcs::GetServiceProperties"

	queryb, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	propertiesJSON, err := computecore.HcsGetServiceProperties(ctx, string(queryb))
	if err != nil {
		err = wrapHcsResult(ctx, err, propertiesJSON)
		return nil, &HcsError{Op: operation, Err: err, Events: eventsFromError(err)}
	}

	if propertiesJSON == "" {
		return nil, ErrUnexpectedValue
	}
	properties := &hcsschema.ServiceProperties{}
	if err := json.Unmarshal([]byte(propertiesJSON), properties); err != nil {
		return nil, err
	}
	return properties, nil
}

// ModifyServiceSettings modifies settings of the host compute service.
func ModifyServiceSettings(ctx context.Context, settings hcsschema.ModificationRequest) error {
	operation := "hcs::ModifyServiceSettings"

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	resultJSON, err := computecore.HcsModifyServiceSettings(ctx, string(settingsJSON))
	if err != nil {
		err = wrapHcsResult(ctx, err, resultJSON)
		return &HcsError{Op: operation, Err: err, Events: eventsFromError(err)}
	}
	return nil
}
