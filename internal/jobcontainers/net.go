package jobcontainers

import (
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/pkg/errors"
)

func (c *JobContainer) netSetup(namespaceID string) error {
	// Network setup. We expect that the network namespace has already been setup by a previous container and we simply set the
	// job objects compartment to the one created.
	ns, err := hcn.GetNamespaceByID(namespaceID)
	if err != nil {
		return errors.Wrap(err, "failed to grab network namespace for job container")
	}

	// If the ns is 0, then no other container has created the compartment (or we're asking for host networking). If it isn't that
	// means that we have a compartment that we can attach to (which should have a virtual net adapter in it also).
	if ns.NamespaceId != 0 {
		if err := c.job.SetNetworkCompartment(ns.NamespaceId); err != nil {
			return err
		}
	}
	return nil
}
