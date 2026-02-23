//go:build windows

package vmmanager

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/sirupsen/logrus"
)

// UtilityVM is an abstraction around a lightweight virtual machine.
// It houses core lifecycle methods such as Create, Start, and Stop and
// also several optional methods that can be used to determine what the virtual machine
// supports and to configure these resources.
type UtilityVM struct {
	id   string
	vmID guid.GUID
	cs   *hcs.System
}

// Create creates a new utility VM with the given ID and compute system configuration.
//
// This method returns the concrete UtilityVM. Callers
// can use the manager interfaces (for example, LifetimeManager, NetworkManager)
// as needed.
func Create(ctx context.Context, id string, config *hcsschema.ComputeSystem) (*UtilityVM, error) {
	cs, err := hcs.CreateComputeSystem(ctx, id, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute system: %w", err)
	}

	defer func() {
		if cs != nil {
			_ = cs.Terminate(ctx)
			_ = cs.WaitCtx(ctx)
		}
	}()

	uvm := &UtilityVM{
		id: id,
	}

	// Cache the VM ID of the utility VM.
	properties, err := cs.Properties(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get compute system properties: %w", err)
	}
	uvm.vmID = properties.RuntimeID
	uvm.cs = cs
	cs = nil

	log.G(ctx).WithFields(logrus.Fields{
		logfields.UVMID: uvm.id,
		"runtime-id":    uvm.vmID.String(),
	}).Debug("created utility VM")

	return uvm, nil
}
