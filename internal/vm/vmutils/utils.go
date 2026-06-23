//go:build windows

package vmutils

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	hcs "github.com/Microsoft/hcsshim/internal/hcs/v2"
	"github.com/Microsoft/hcsshim/internal/log"

	"github.com/containerd/typeurl/v2"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/anypb"
)

// ParseUVMReferenceInfo reads the UVM reference info file, and base64 encodes the content if it exists.
func ParseUVMReferenceInfo(ctx context.Context, referenceRoot, referenceName string) (string, error) {
	if referenceName == "" {
		return "", nil
	}

	fullFilePath := filepath.Join(referenceRoot, referenceName)
	content, err := os.ReadFile(fullFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.G(ctx).WithField("filePath", fullFilePath).Debug("UVM reference info file not found")
			return "", nil
		}
		return "", fmt.Errorf("failed to read UVM reference info file: %w", err)
	}

	return base64.StdEncoding.EncodeToString(content), nil
}

// UnmarshalRuntimeOptions decodes the runtime options into runhcsoptions.Options.
// When no options are provided (options == nil) it returns a non-nil,
// zero-value Options struct.
func UnmarshalRuntimeOptions(ctx context.Context, options *anypb.Any) (*runhcsoptions.Options, error) {
	opts := &runhcsoptions.Options{}
	if options == nil {
		return opts, nil
	}

	v, err := typeurl.UnmarshalAny(options)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal options: %w", err)
	}

	shimOpts, ok := v.(*runhcsoptions.Options)
	if !ok {
		return nil, fmt.Errorf("failed to unmarshal runtime options: expected *runhcsoptions.Options, got %T", v)
	}

	if entry := log.G(ctx); entry.Logger.IsLevelEnabled(logrus.DebugLevel) {
		entry.WithField("options", log.Format(ctx, shimOpts)).Debug("parsed runtime options")
	}

	return shimOpts, nil
}

// IsVMNotAvailableError reports whether err indicates the underlying compute system is
// no longer available for modification — either it has stopped, no longer
// exists, or is in an invalid state for further modifications..
func IsVMNotAvailableError(err error) bool {
	return hcs.IsNotExist(err) ||
		hcs.IsAlreadyStopped(err) ||
		hcs.IsAlreadyClosed(err) ||
		hcs.IsOperationInvalidState(err)
}
