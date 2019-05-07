package runhcs

import (
	"context"
	"encoding/json"

	irunhcs "github.com/microsoft/hcsshim/internal/runhcs"
)

// ContainerState is the representation of the containers state at the moment of
// query.
type ContainerState = irunhcs.ContainerState

// List containers started by runhcs.
//
// Note: This is specific to the Runhcs.Root namespace provided in the global
// settings.
func (r *Runhcs) List(context context.Context) ([]*ContainerState, error) {
	data, err := cmdOutput(r.command(context, "list", "--format=json"), false)
	if err != nil {
		return nil, err
	}
	var out []*ContainerState
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
