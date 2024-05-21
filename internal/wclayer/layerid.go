//go:build windows

package wclayer

import (
	"context"
	"path/filepath"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// LayerID returns the layer ID of a layer on disk.
func LayerID(ctx context.Context, path string) (_ guid.GUID, err error) {
	title := "hcsshim::LayerID"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("path", path)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	_, file := filepath.Split(path)
	return NameToGuid(ctx, file)
}
