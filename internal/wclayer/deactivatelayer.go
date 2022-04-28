//go:build windows

package wclayer

import (
	"context"

	hcserror "github.com/Microsoft/hcsshim/internal/hcs/errors/legacy"
	"github.com/Microsoft/hcsshim/internal/oc"
	"go.opencensus.io/trace"
)

// DeactivateLayer will dismount a layer that was mounted via ActivateLayer.
func DeactivateLayer(ctx context.Context, path string) (err error) {
	title := "hcsshim::DeactivateLayer"
	ctx, span := oc.StartSpan(ctx, title) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("path", path))

	err = deactivateLayer(&stdDriverInfo, path)
	if err != nil {
		return hcserror.New(err, title+"- failed", "")
	}
	return nil
}
