//go:build windows

package vmutils

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/log"
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
