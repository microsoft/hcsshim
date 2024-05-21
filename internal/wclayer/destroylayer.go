//go:build windows

package wclayer

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/hcserror"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// DestroyLayer will remove the on-disk files representing the layer with the given
// path, including that layer's containing folder, if any.
func DestroyLayer(ctx context.Context, path string) (err error) {
	title := "hcsshim::DestroyLayer"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("path", path))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	err = destroyLayer(&stdDriverInfo, path)
	if err != nil {
		return hcserror.New(err, title, "")
	}
	return nil
}
