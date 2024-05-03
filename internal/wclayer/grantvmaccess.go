//go:build windows

package wclayer

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/hcserror"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// GrantVmAccess adds access to a file for a given VM
func GrantVmAccess(ctx context.Context, vmid string, filepath string) (err error) {
	title := "hcsshim::GrantVmAccess"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("vm-id", vmid),
		attribute.String("path", filepath))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	err = grantVmAccess(vmid, filepath)
	if err != nil {
		return hcserror.New(err, title, "")
	}
	return nil
}
