package hcsoci

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/sirupsen/logrus"
)

// NormalizeProcessorCount returns the `Min(requested, logical CPU count)`.
func NormalizeProcessorCount(ctx context.Context, cid string, requestedCount, hostCount int32) int32 {
	if requestedCount > hostCount {
		log.G(ctx).WithFields(logrus.Fields{
			"id":              cid,
			"requested count": requestedCount,
			"assigned count":  hostCount,
		}).Warn("Changing user requested cpu count to current number of processors on the host")
		return hostCount
	} else {
		return requestedCount
	}
}

// NormalizeMemorySize returns the requested memory size in MB aligned up to an even number
func NormalizeMemorySize(ctx context.Context, cid string, requestedSizeMB uint64) uint64 {
	actualMB := (requestedSizeMB + 1) &^ 1 // align up to an even number
	if requestedSizeMB != actualMB {
		log.G(ctx).WithFields(logrus.Fields{
			"id":          cid,
			"requestedMB": requestedSizeMB,
			"actualMB":    actualMB,
		}).Warn("Changing user requested MemorySizeInMB to align to 2MB")
	}
	return actualMB
}
