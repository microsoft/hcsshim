//go:build windows

package helpers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/pkg/cimfs"
	ociwclayercim "github.com/Microsoft/hcsshim/pkg/ociwclayer/cim"
)

// keepBlockCIMEnvVar names a directory where the intermediate Block CIMs are
// written and preserved (instead of a temp dir that is deleted). It exists so
// the generated layer.vhd / merged.vhd block CIM files can be inspected (e.g.
// with the blockcimdump tool) to debug digest mismatches against containerd's
// layer.vhd.
const keepBlockCIMEnvVar = "SECURITYPOLICY_BLOCKCIM_DIR"

// containerd's cimfs snapshotter names every layer's block CIM "layer.cim"
// (each layer in its own directory) and the merged CIM "merged.cim". The CimName
// is embedded in the block CIM's PrimaryBlockTable and therefore contributes to
// the sealed root digest, so the tooling must use the exact same names (and one
// directory per layer) to reproduce the digests the runtime computes.
const (
	containerdLayerCIMName  = "layer.cim"
	containerdMergedCIMName = "merged.cim"
)

// computeWindowsImageDigests imports each image layer into a verified Block CIM,
// merges them, and returns the per-layer root digests (base-to-top, matching
// img.Layers()) plus the merged CIM root digest. This mirrors the digests the
// runtime computes when mounting verified CIMs, so the resulting policy matches
// enforcement. It requires the Windows CIM APIs (cimfs) and typically elevation.
func computeWindowsImageDigests(ctx context.Context, img v1.Image) (_ []string, _ string, err error) {
	imgLayers, err := img.Layers()
	if err != nil {
		return nil, "", err
	}
	if len(imgLayers) == 0 {
		return nil, "", fmt.Errorf("image has no layers")
	}

	// When SECURITYPOLICY_BLOCKCIM_DIR is set, write the Block CIMs there and
	// keep them for inspection; otherwise use a temp dir that is cleaned up.
	tmpDir := os.Getenv(keepBlockCIMEnvVar)
	if tmpDir != "" {
		if err = os.MkdirAll(tmpDir, 0755); err != nil {
			return nil, "", fmt.Errorf("create block CIM output dir %q: %w", tmpDir, err)
		}
		log.G(ctx).WithField("dir", tmpDir).Warn("preserving intermediate Block CIMs")
	} else {
		tmpDir, err = os.MkdirTemp("", "securitypolicy-wcow-")
		if err != nil {
			return nil, "", err
		}
		defer os.RemoveAll(tmpDir)
	}

	blockCIMs := make([]*cimfs.BlockCIM, len(imgLayers))
	layerDigests := make([]string, len(imgLayers))

	// Import base-to-top so each layer's parents are already imported.
	for i, layer := range imgLayers {
		r, err := layer.Uncompressed()
		if err != nil {
			return nil, "", err
		}

		blockCIMs[i] = &cimfs.BlockCIM{
			Type: cimfs.BlockCIMTypeSingleFile,
			// One directory per layer so every layer's block file can share the
			// containerd CimName "layer.cim" without colliding.
			BlockPath: filepath.Join(tmpDir, fmt.Sprintf("layer%d", i), "layer.vhd"),
			CimName:   containerdLayerCIMName,
		}

		// Parent layers are ordered immediate-parent-first.
		var parents []*cimfs.BlockCIM
		for j := i - 1; j >= 0; j-- {
			parents = append(parents, blockCIMs[j])
		}

		// Single-file verified CIM matches containerd's CWCOW extraction. The VHD
		// footer is skipped: it's appended after sealing so it doesn't affect the
		// root digest, and the block CIMs are discarded once digests are read.
		importOpts := []ociwclayercim.BlockCIMLayerImportOpt{ociwclayercim.WithLayerIntegrity()}
		if len(parents) > 0 {
			importOpts = append(importOpts, ociwclayercim.WithParentLayers(parents))
		}

		_, importErr := ociwclayercim.ImportBlockCIMLayerWithOpts(ctx, r, blockCIMs[i], importOpts...)
		if cerr := r.Close(); cerr != nil && importErr == nil {
			importErr = cerr
		}
		if importErr != nil {
			return nil, "", fmt.Errorf("import layer %d into block CIM: %w", i, importErr)
		}

		digest, err := ociwclayercim.GetIntegrityChecksum(ctx, blockCIMs[i].BlockPath, "")
		if err != nil {
			return nil, "", fmt.Errorf("get layer %d digest: %w", i, err)
		}
		layerDigests[i] = digest
		log.G(ctx).WithFields(logrus.Fields{
			"layer":  i,
			"path":   blockCIMs[i].BlockPath,
			"digest": digest,
		}).Debug("imported Block CIM layer")
	}

	// A single-layer image has nothing to merge: the runtime attaches only the
	// lone verified layer CIM (mergedCIM is nil), so the mounted CIM digest the
	// runtime reports is that layer's own digest.
	if len(blockCIMs) < 2 {
		return layerDigests, layerDigests[0], nil
	}

	// MergeBlockCIMLayersWithOpts expects source CIMs ordered topmost-to-base.
	sourceCIMs := make([]*cimfs.BlockCIM, len(blockCIMs))
	for i, c := range blockCIMs {
		sourceCIMs[len(blockCIMs)-1-i] = c
	}

	mergedCIM := &cimfs.BlockCIM{
		Type:      cimfs.BlockCIMTypeSingleFile,
		BlockPath: filepath.Join(tmpDir, "merged", "merged.vhd"),
		CimName:   containerdMergedCIMName,
	}
	if err := ociwclayercim.MergeBlockCIMLayersWithOpts(ctx, sourceCIMs, mergedCIM, ociwclayercim.WithLayerIntegrity()); err != nil {
		return nil, "", fmt.Errorf("merge block CIM layers: %w", err)
	}

	mergedDigest, err := ociwclayercim.GetIntegrityChecksum(ctx, mergedCIM.BlockPath, "")
	if err != nil {
		return nil, "", fmt.Errorf("get merged CIM digest: %w", err)
	}
	log.G(ctx).WithFields(logrus.Fields{
		"path":   mergedCIM.BlockPath,
		"digest": mergedDigest,
	}).Debug("merged Block CIM layers")

	return layerDigests, mergedDigest, nil
}
