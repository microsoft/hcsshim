package logrusadapter

import (
	"context"

	"github.com/sirupsen/logrus"

	"your/module/internal/logging"
)

type Adapter struct {
	entry *logrus.Entry
}

func New(
	entry *logrus.Entry,
) *Adapter {
	return &Adapter{
		entry: entry,
	}
}

func (a *Adapter) Log(
	ctx context.Context,
	level logging.Level,
	msg string,
	attrs ...logging.Attr,
) {
	fields := logrus.Fields{}

	for _, attr := range attrs {
		fields[attr.Key] = attr.Value
	}

	e := a.entry.WithFields(fields)

	switch level {
	case logging.Debug:
		e.Debug(msg)

	case logging.Info:
		e.Info(msg)

	case logging.Warn:
		e.Warn(msg)

	case logging.Error:
		e.Error(msg)
	}
}

func (a *Adapter) With(
	attrs ...logging.Attr,
) logging.Logger {
	fields := logrus.Fields{}

	for _, attr := range attrs {
		fields[attr.Key] = attr.Value
	}

	return &Adapter{
		entry: a.entry.WithFields(fields),
	}
}
