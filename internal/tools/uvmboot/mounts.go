//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func mountSCSI(ctx context.Context, c *cli.Context, vm *uvm.UtilityVM) error {
	for _, m := range parseMounts(c, scsiMountsArgName) {
		if _, err := vm.AddSCSI(
			ctx,
			m.host,
			m.guest,
			!m.writable,
			false, // encrypted
			[]string{},
			uvm.VMAccessTypeIndividual,
		); err != nil {
			return fmt.Errorf("could not mount disk %s: %w", m.host, err)
		} else {
			logrus.WithFields(logrus.Fields{
				"host":     m.host,
				"guest":    m.guest,
				"writable": m.writable,
			}).Debug("Mounted SCSI disk")
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

	if len(ps) >= 3 {
		return m, errors.New("too many parts")
	}

	m.host = ps[0]

	if len(ps) == 2 {
		m.guest = ps[1]
	}

	if len(ps) == 3 && strings.ToLower(ps[2]) == "w" {
		m.writable = true
	}

	return m, nil
}
