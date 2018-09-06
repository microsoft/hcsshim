package wclayer

import (
	"path/filepath"

	"github.com/google/uuid"
)

// LayerID returns the layer ID of a layer on disk.
func LayerID(path string) (uuid.UUID, error) {
	_, file := filepath.Split(path)
	return NameToGuid(file)
}
