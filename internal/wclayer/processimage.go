//go:build windows

package wclayer

import (
	"context"
	"os"

	"github.com/Microsoft/hcsshim/internal/otelutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ProcessBaseLayer post-processes a base layer that has had its files extracted.
// The files should have been extracted to <path>\Files.
func ProcessBaseLayer(ctx context.Context, path string) (err error) {
	title := "hcsshim::ProcessBaseLayer"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("path", path))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	err = processBaseImage(path)
	if err != nil {
		return &os.PathError{Op: title, Path: path, Err: err}
	}
	return nil
}

// ProcessUtilityVMImage post-processes a utility VM image that has had its files extracted.
// The files should have been extracted to <path>\Files.
func ProcessUtilityVMImage(ctx context.Context, path string) (err error) {
	title := "hcsshim::ProcessUtilityVMImage"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("path", path))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	err = processUtilityImage(path)
	if err != nil {
		return &os.PathError{Op: title, Path: path, Err: err}
	}
	return nil
}
