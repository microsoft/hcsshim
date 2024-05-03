//go:build windows

package computestorage

import (
	"context"
	"encoding/json"

	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AttachLayerStorageFilter sets up the layer storage filter on a writable
// container layer.
//
// `layerPath` is a path to a directory the writable layer is mounted. If the
// path does not end in a `\` the platform will append it automatically.
//
// `layerData` is the parent read-only layer data.
func AttachLayerStorageFilter(ctx context.Context, layerPath string, layerData LayerData) (err error) {
	title := "hcsshim::AttachLayerStorageFilter"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("layerPath", layerPath))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	bytes, err := json.Marshal(layerData)
	if err != nil {
		return err
	}

	err = hcsAttachLayerStorageFilter(layerPath, string(bytes))
	if err != nil {
		return errors.Wrap(err, "failed to attach layer storage filter")
	}
	return nil
}

// AttachOverlayFilter sets up a filter of the given type on a writable container layer.  Currently the only
// supported filter types are WCIFS & UnionFS (defined in internal/hcs/schema2/layer.go)
//
// `volumePath` is volume path at which writable layer is mounted. If the
// path does not end in a `\` the platform will append it automatically.
//
// `layerData` is the parent read-only layer data.
func AttachOverlayFilter(ctx context.Context, volumePath string, layerData LayerData) (err error) {
	title := "hcsshim::AttachOverlayFilter"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("volumePath", volumePath))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	bytes, err := json.Marshal(layerData)
	if err != nil {
		return err
	}

	err = hcsAttachOverlayFilter(volumePath, string(bytes))
	if err != nil {
		return errors.Wrap(err, "failed to attach overlay filter")
	}
	return nil
}
