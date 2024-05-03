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

// InitializeWritableLayer initializes a writable layer for a container.
//
// `layerPath` is a path to a directory the layer is mounted. If the
// path does not end in a `\` the platform will append it automatically.
//
// `layerData` is the parent read-only layer data.
func InitializeWritableLayer(ctx context.Context, layerPath string, layerData LayerData) (err error) {
	title := "hcsshim::InitializeWritableLayer"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("layerPath", layerPath))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	bytes, err := json.Marshal(layerData)
	if err != nil {
		return err
	}

	// Options are not used in the platform as of RS5
	err = hcsInitializeWritableLayer(layerPath, string(bytes), "")
	if err != nil {
		return errors.Wrap(err, "failed to intitialize container layer")
	}
	return nil
}
