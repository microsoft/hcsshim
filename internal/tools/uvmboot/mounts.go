//go:build windows

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/uvm/scsi"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func mountSCSI(ctx context.Context, c *cli.Context, vm *uvm.UtilityVM) error {
	for _, m := range parseMounts(c, scsiMountsArgName) {
		if m.guest != "" {
			return fmt.Errorf("scsi mount %s: guest path must be empty", m.host)
		}
		scsi, err := vm.SCSIManager.AddVirtualDisk(
			ctx,
			m.host,
			!m.writable,
			vm.ID(),
			&scsi.MountConfig{},
		)
		if err != nil {
			return fmt.Errorf("could not mount disk %s: %w", m.host, err)
		} else {
			logrus.WithFields(logrus.Fields{
				"host":     m.host,
				"guest":    scsi.GuestPath(),
				"writable": m.writable,
			}).Info("Mounted SCSI disk")
		}
	}

	return nil
}

func shareFiles(ctx context.Context, c *cli.Context, vm *uvm.UtilityVM) error {
	switch os := vm.OS(); os {
	case "linux":
		return shareFilesLCOW(ctx, c, vm)
	default:
		return fmt.Errorf("file shares are not supported for %s UVMs", os)
	}
}

func shareFilesLCOW(ctx context.Context, c *cli.Context, vm *uvm.UtilityVM) error {
	for _, s := range parseMounts(c, shareFilesArgName) {
		if s.guest == "" {
			return fmt.Errorf("file shares %q has invalid quest destination: %q", s.host, s.guest)
		}

		if err := vm.Share(ctx, s.host, s.guest, !s.writable); err != nil {
			return fmt.Errorf("could not share file or directory %s: %w", s.host, err)
		} else {
			logrus.WithFields(logrus.Fields{
				"host":     s.host,
				"guest":    s.guest,
				"writable": s.writable,
			}).Debug("Shared path")
		}
	}

	return nil
}

func mountVPMem(ctx context.Context, c *cli.Context, vm *uvm.UtilityVM) error {
	if !c.IsSet(vpmemMountsArgName) {
		return nil
	}
	for _, p := range c.StringSlice(vpmemMountsArgName) {
		if _, err := vm.AddVPMem(ctx, p); err != nil {
			return fmt.Errorf("could not mount VPMem device: %w", err)
		}
	}
	return nil
}

type mount struct {
	host     string
	guest    string
	writable bool
}

// parseMounts parses the mounts stored under the cli StringSlice argument, `n`
func parseMounts(c *cli.Context, n string) []mount {
	if c.IsSet(n) {
		ss := c.StringSlice(n)
		ms := make([]mount, 0, len(ss))
		for _, s := range ss {
			logrus.Debugf("parsing %q", s)

			if m, err := mountFromString(s); err == nil {
				ms = append(ms, m)
			}
		}

		return ms
	}

	return nil
}

func mountFromString(s string) (m mount, _ error) {
	ps := strings.Split(s, ",")

	l := len(ps)
	if l == 0 { // shouldn't happen, but just in case
		return m, fmt.Errorf("could not parse string %q", s)
	}

	if l > 3 {
		return m, fmt.Errorf("too many parts in %q", s)
	}

	m.host = ps[0]

	if l >= 2 {
		m.guest = ps[1]
	}

	if l == 3 && strings.ToLower(ps[2]) == "w" {
		m.writable = true
	}

	return m, nil
}
