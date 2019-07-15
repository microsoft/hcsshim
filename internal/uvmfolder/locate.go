package uvmfolder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

// LocateUVMFolder searches a set of layer folders to determine the "uppermost"
// layer which has a utility VM image. The order of the layers is (for historical) reasons
// Read-only-layers followed by an optional read-write layer. The RO layers are in reverse
// order so that the upper-most RO layer is at the start, and the base OS layer is the
// end.
func LocateUVMFolder(ctx context.Context, layerFolders []string) (_ string, err error) {
	ctx, span := trace.StartSpan(ctx, "uvmfolder::LocateUVMFolder")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var uvmFolder string
	index := 0
	for _, layerFolder := range layerFolders {
		_, err := os.Stat(filepath.Join(layerFolder, `UtilityVM`))
		if err == nil {
			uvmFolder = layerFolder
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		index++
	}
	if uvmFolder == "" {
		return "", fmt.Errorf("utility VM folder could not be found in layers")
	}
	log.G(ctx).WithFields(logrus.Fields{
		"index":  index + 1,
		"count":  len(layerFolders),
		"folder": uvmFolder,
	}).Debug("found uvm folder")
	return uvmFolder, nil
}
