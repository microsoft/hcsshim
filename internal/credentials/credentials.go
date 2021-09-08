// +build windows

// Package credentials holds the necessary structs and functions for adding
// and removing Container Credential Guard instances (shortened to CCG
// normally) for V2 HCS schema containers.
package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
)

// Container Credential Guard is in HCS's own words "The solution to
// allowing windows containers to have access to domain credentials for the
// applications running in their corresponding guest." It essentially acts as
// a way to temporarily Active Directory join a given container with a Group
// Managed Service Account (GMSA for short) credential specification.
// CCG will launch a process in the host that will act as a middleman for the
// credential passthrough logic. The guest is then configured through registry
// keys to have access to the process in the host.
// A CCG instance needs to be created through various HCS calls and then added to
// the V2 schema container document before being sent to HCS. For V1 HCS schema containers
// setting up instances manually is not needed, the GMSA credential specification
// simply needs to be present in the V1 container document.

// CCGResource stores the id used when creating a ccg instance. Used when
// closing a container to be able to release the instance.
type CCGResource struct {
	// ID of container that instance belongs to.
	id string
}

// Release calls into hcs to remove the ccg instance for the container matching CCGResource.id.
// These do not get cleaned up automatically they MUST be explicitly removed with a call to
// ModifyServiceSettings. The instances will persist unless vmcompute.exe exits or they are removed
// manually as done here.
func (ccgResource *CCGResource) Release(ctx context.Context) error {
	if err := removeCredentialGuard(ctx, ccgResource.id); err != nil {
		return fmt.Errorf("failed to remove container credential guard instance: %s", err)
	}
	return nil
}

// CreateCredentialGuard creates a container credential guard instance and
// returns the state object to be placed in a v2 container doc.
func CreateCredentialGuard(ctx context.Context, id, credSpec string, hypervisorIsolated bool) (*hcsschema.ContainerCredentialGuardInstance, *CCGResource, error) {
	log.G(ctx).WithField("containerID", id).Debug("creating container credential guard instance")
	// V2 schema ccg setup a little different as its expected to be passed
	// through all the way to the gcs. Can no longer be enabled just through
	// a single property. The flow is as follows
	// ------------------------------------------------------------------------
	// 1. Call HcsModifyServiceSettings with a ModificationRequest set with a
	// ContainerCredentialGuardAddInstanceRequest. This is where the cred spec
	// gets passed in. Transport either "LRPC" (Argon) or "HvSocket" (Xenon).
	//
	// 2. Query the instance with a call to HcsGetServiceProperties with the
	// PropertyType "ContainerCredentialGuard". This will return all instances
	//
	// 3. Parse for the id of our container to find which one correlates to the
	// container we're building the doc for, then add to the V2 doc.
	//
	// 4. If xenon container the CCG instance with the Hvsocket service table
	// information is expected to be in the Utility VMs doc before being sent
	// to HCS for creation. For pod scenarios currently we don't have the OCI
	// spec of a container at UVM creation time, therefore the service table entry
	// for the CCG instance will have to be hot added.
	transport := "LRPC"
	if hypervisorIsolated {
		transport = "HvSocket"
	}
	req := hcsschema.ModificationRequest{
		PropertyType: hcsschema.PTContainerCredentialGuard,
		Settings: &hcsschema.ContainerCredentialGuardOperationRequest{
			Operation: hcsschema.AddInstance,
			OperationDetails: &hcsschema.ContainerCredentialGuardAddInstanceRequest{
				Id:             id,
				CredentialSpec: credSpec,
				Transport:      transport,
			},
		},
	}
	if err := hcs.ModifyServiceSettings(ctx, req); err != nil {
		return nil, nil, fmt.Errorf("failed to generate container credential guard instance: %s", err)
	}

	q := hcsschema.PropertyQuery{
		PropertyTypes: []hcsschema.PropertyType{hcsschema.PTContainerCredentialGuard},
	}
	serviceProps, err := hcs.GetServiceProperties(ctx, q)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve container credential guard instances: %s", err)
	}
	if len(serviceProps.Properties) != 1 {
		return nil, nil, errors.New("wrong number of service properties present")
	}

	ccgSysInfo := &hcsschema.ContainerCredentialGuardSystemInfo{}
	if err := json.Unmarshal(serviceProps.Properties[0], ccgSysInfo); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal container credential guard instances: %s", err)
	}
	for _, ccgInstance := range ccgSysInfo.Instances {
		if ccgInstance.Id == id {
			ccgResource := &CCGResource{
				id,
			}
			return &ccgInstance, ccgResource, nil
		}
	}
	return nil, nil, fmt.Errorf("failed to find credential guard instance with container ID %s", id)
}

// Removes a ContainerCredentialGuard instance by container ID.
func removeCredentialGuard(ctx context.Context, id string) error {
	log.G(ctx).WithField("containerID", id).Debug("removing container credential guard")

	req := hcsschema.ModificationRequest{
		PropertyType: hcsschema.PTContainerCredentialGuard,
		Settings: &hcsschema.ContainerCredentialGuardOperationRequest{
			Operation: hcsschema.RemoveInstance,
			OperationDetails: &hcsschema.ContainerCredentialGuardRemoveInstanceRequest{
				Id: id,
			},
		},
	}
	return hcs.ModifyServiceSettings(ctx, req)
}
