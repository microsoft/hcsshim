//go:build windows

package computestorage

import (
	"context"
	"encoding/json"

	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sys/windows"
)

// SetupBaseOSLayer sets up a layer that contains a base OS for a container.
//
// `layerPath` is a path to a directory containing the layer.
//
// `vhdHandle` is an empty file handle of `options.Type == OsLayerTypeContainer`
// or else it is a file handle to the 'SystemTemplateBase.vhdx' if `options.Type
// == OsLayerTypeVm`.
//
// `options` are the options applied while processing the layer.
func SetupBaseOSLayer(ctx context.Context, layerPath string, vhdHandle windows.Handle, options OsLayerOptions) (err error) {
	title := "hcsshim::SetupBaseOSLayer"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("layerPath", layerPath))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	bytes, err := json.Marshal(options)
	if err != nil {
		return err
	}

	err = hcsSetupBaseOSLayer(layerPath, vhdHandle, string(bytes))
	if err != nil {
		return errors.Wrap(err, "failed to setup base OS layer")
	}
	return nil
}

// SetupBaseOSVolume sets up a volume that contains a base OS for a container.
//
// `layerPath` is a path to a directory containing the layer.
//
// `volumePath` is the path to the volume to be used for setup.
//
// `options` are the options applied while processing the layer.
//
// NOTE: This API is only available on builds of Windows greater than 19645. Inside we
// check if the hosts build has the API available by using 'GetVersion' which requires
// the calling application to be manifested. https://docs.microsoft.com/en-us/windows/win32/sbscs/manifests
func SetupBaseOSVolume(ctx context.Context, layerPath, volumePath string, options OsLayerOptions) (err error) {
	if osversion.Build() < 19645 {
		return errors.New("SetupBaseOSVolume is not present on builds older than 19645")
	}
	title := "hcsshim::SetupBaseOSVolume"
	ctx, span := otelutil.StartSpan(ctx, title, trace.WithAttributes(
		attribute.String("layerPath", layerPath),
		attribute.String("volumePath", volumePath))) //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	bytes, err := json.Marshal(options)
	if err != nil {
		return err
	}

	err = hcsSetupBaseOSVolume(layerPath, volumePath, string(bytes))
	if err != nil {
		return errors.Wrap(err, "failed to setup base OS layer")
	}
	return nil
}
