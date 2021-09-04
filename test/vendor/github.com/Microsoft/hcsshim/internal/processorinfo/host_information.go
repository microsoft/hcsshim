package processorinfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
)

// HostProcessorInfo queries HCS for the host's processor information, including topology
// and NUMA configuration. This is also used to reliably get the hosts number of logical
// processors in multi processor group settings.
func HostProcessorInfo(ctx context.Context) (*hcsschema.ProcessorTopology, error) {
	q := hcsschema.PropertyQuery{
		PropertyTypes: []hcsschema.PropertyType{hcsschema.PTProcessorTopology},
	}
	serviceProps, err := hcs.GetServiceProperties(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve processor and processor topology information: %s", err)
	}
	if len(serviceProps.Properties) != 1 {
		return nil, errors.New("wrong number of service properties present")
	}
	processorTopology := &hcsschema.ProcessorTopology{}
	if err := json.Unmarshal(serviceProps.Properties[0], processorTopology); err != nil {
		return nil, fmt.Errorf("failed to unmarshal host processor topology: %s", err)
	}
	return processorTopology, nil
}
