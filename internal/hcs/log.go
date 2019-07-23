package hcs

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/sirupsen/logrus"
)

func logOperationBegin(ctx context.Context, f logrus.Fields, msg string) {
	log.G(ctx).WithFields(f).Debug(msg)
}

func logOperationEnd(ctx context.Context, f logrus.Fields, msg string, err error) {
	// Copy the log and fields first.
	log := log.G(ctx).WithFields(f)
	if err == nil {
		log.Debug(msg)
	} else {
		// Edit only the copied field data to avoid race conditions on the
		// write.
		log.Data[logrus.ErrorKey] = err
		log.Error(msg)
	}
}
