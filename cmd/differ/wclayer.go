//go:build windows

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Microsoft/go-winio"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/urfave/cli/v2"

	"github.com/Microsoft/hcsshim/cmd/differ/mediatype"
	"github.com/Microsoft/hcsshim/cmd/differ/payload"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/Microsoft/hcsshim/pkg/ociwclayer"
)

/*
SeChangeNotifyPrivilege,SeBackupPrivilege,SeRestorePrivilege,SeDebugPrivilege,SeImpersonatePrivilege,SeTcbPrivilege,SeIncreaseQuotaPrivilege,SeAssignPrimaryTokenPrivilege
SeChangeNotifyPrivilege,SeAssignPrimaryTokenPrivilege,SeIncreaseQuotaPrivilege,SeTcbPrivilege,SeTakeOwnershipPrivilege,SeManageVolumePrivilege,SeImpersonatePrivilege,SeCreateSymbolicLinkPrivilege,SeLoadDriverPrivilege,SeBackupPrivilege,SeRestorePrivilege
*/

// ociwclayer needs backup and restore, but also needs create global object and manage volume privileges
var privs = []string{
	winapi.SeBackupPrivilege,
	winapi.SeRestorePrivilege,
	winapi.SeCreateGlobalPrivilege,
	winapi.SeManageVolumePrivilege,
}

var wclayerCommand = &cli.Command{
	Name:    "wclayer",
	Aliases: []string{"wc"},
	Usage: fmt.Sprintf("Convert a %q stream and extract it into a Windows layer, %q",
		ocispec.MediaTypeImageLayer, mediatype.MediaTypeMicrosoftImageLayerVHD),
	Before: createCommandBeforeFunc(withPrivileges(privs)),
	Action: importFromTar,
}

func importFromTar(c *cli.Context) error {
	opts := &payload.WCLayerImportOptions{}
	if err := getPayload(c.Context, opts); err != nil {
		return fmt.Errorf("parsing payload: %w", err)
	}
	log.G(c.Context).WithField("payload", opts).Debug("Parsed payload")

	if err := winio.EnableProcessPrivileges(privs); err != nil {
		return fmt.Errorf("enable process privileges: %w", err)
	}
	if _, err := ociwclayer.ImportLayerFromTar(c.Context, os.Stdin, opts.RootPath, opts.Parents); err != nil {
		return fmt.Errorf("wclayer import: %w", err)
	}
	// discard remaining data
	_, _ = io.Copy(io.Discard, os.Stdin)
	return nil
}
