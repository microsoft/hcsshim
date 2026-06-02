//go:build windows
// +build windows

package hcsoci

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/log"
)

// applyArgonCPUAffinity honors spec.Windows.Resources.CPU.Affinity for a
// process-isolated (Argon) container by pinning the container's server silo.
//
// HCS ignores CPU affinity on the container Processor schema (Count/Maximum/Weight),
// so instead we set the affinity on the silo's job object directly. This must run
// after the compute system is created but before it is started, so the affinity is
// already recorded on the job when HCS assigns the init process to the silo. See
// (*hcs.System).SetSiloCPUGroupAffinities for the race-free timeline.
//
// If the spec requests no affinity this is a no-op.
func applyArgonCPUAffinity(ctx context.Context, system *hcs.System, coi *createOptionsInternal) error {
	affinities, err := ValidateCPUAffinity(coi.Spec)
	if err != nil {
		return err
	}
	if len(affinities) == 0 {
		return nil
	}

	if err := system.SetSiloCPUGroupAffinities(ctx, ToJobObjectAffinities(affinities)); err != nil {
		return fmt.Errorf("apply CPU affinity to container silo: %w", err)
	}

	log.G(ctx).WithField("affinities", affinities).Debug("applied CPU affinity to Argon container silo")
	return nil
}
