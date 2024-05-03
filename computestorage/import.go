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

// ImportLayer imports a container layer.
//
// `layerPath` is a path to a directory to import the layer to. If the directory
// does not exist it will be automatically created.
//
// `sourceFolderpath` is a pre-existing folder that contains the layer to
// import.
//
// `layerData` is the parent layer data.
func ImportLayer(ctx context.Context, layerPath, sourceFolderPath string, layerData LayerData) (err error) {
	title := "hcsshim::ImportLayer"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("layerPath", layerPath),
		attribute.String("sourceFolderPath", sourceFolderPath))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	bytes, err := json.Marshal(layerData)
	if err != nil {
		return err
	}

	err = hcsImportLayer(layerPath, sourceFolderPath, string(bytes))
	if err != nil {
		return errors.Wrap(err, "failed to import layer")
	}
	return nil
}
