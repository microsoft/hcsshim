//go:build windows

package computestorage

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/oc"

	"go.opencensus.io/trace"
)

// DestroyLayer deletes a container layer.
//
// `layerPath` is a path to a directory containing the layer to export.
func DestroyLayer(ctx context.Context, layerPath string) (err error) {
	title := "hcsshim::DestroyLayer"
	ctx, span := oc.StartSpan(ctx, title) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("layerPath", layerPath))

	err = hcsDestroyLayer(layerPath)
	if err != nil {
		return fmt.Errorf("failed to destroy layer: %w", err)
	}
	return nil
}
