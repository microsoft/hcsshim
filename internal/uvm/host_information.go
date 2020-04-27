package uvm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
)

// hostProcessorInfo queries HCS for the UVM hosts processor information, including topology
// and NUMA configuration. This is used to reliably get the hosts number of logical
// processors in multi socket or > 1 NUMA node machines. Returns the hosts NUMA
// configuration and logical processor information.
func (uvm *UtilityVM) hostProcessorInfo(ctx context.Context) (*hcsschema.ProcessorInformationForHost, *hcsschema.ProcessorTopology, error) {
	q := hcsschema.PropertyQuery{
		PropertyTypes: []hcsschema.PropertyType{hcsschema.PTProcessor, hcsschema.PTProcessorTopology},
	}
	serviceProps, err := hcs.GetServiceProperties(ctx, q)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve processor and processor topology information: %s", err)
	}
	if len(serviceProps.Properties) != 2 {
		return nil, nil, errors.New("wrong number of service properties present")
	}

	processorInfo := &hcsschema.ProcessorInformationForHost{}
	processorTopology := &hcsschema.ProcessorTopology{}
	if err := json.Unmarshal(serviceProps.Properties[0], processorInfo); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal host processor information: %s", err)
	}
	if err := json.Unmarshal(serviceProps.Properties[1], processorTopology); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal host processor topology: %s", err)
	}
	return processorInfo, processorTopology, nil
}
