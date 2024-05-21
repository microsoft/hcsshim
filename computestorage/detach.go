//go:build windows

package computestorage

import (
	"context"
	"encoding/json"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// DetachLayerStorageFilter detaches the layer storage filter on a writable container layer.
//
// `layerPath` is a path to a directory containing the layer to export.
func DetachLayerStorageFilter(ctx context.Context, layerPath string) (err error) {
	title := "hcsshim::DetachLayerStorageFilter"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("layerPath", layerPath))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	err = hcsDetachLayerStorageFilter(layerPath)
	if err != nil {
		return errors.Wrap(err, "failed to detach layer storage filter")
	}
	return nil
}

// DetachOverlayFilter detaches the filter on a writable container layer.
//
// `volumePath` is a path to writable container volume.
func DetachOverlayFilter(ctx context.Context, volumePath string, filterType hcsschema.FileSystemFilterType) (err error) {
	title := "hcsshim::DetachOverlayFilter"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("volumePath", volumePath))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	layerData := LayerData{}
	layerData.FilterType = filterType
	bytes, err := json.Marshal(layerData)
	if err != nil {
		return err
	}

	err = hcsDetachOverlayFilter(volumePath, string(bytes))
	if err != nil {
		return errors.Wrap(err, "failed to detach overlay filter")
	}
	return nil
}
