package logging

import "context"

type noopLogger struct{}

func (n noopLogger) Log(
	ctx context.Context,
	level Level,
	msg string,
	attrs ...Attr,
) {
}

func (n noopLogger) With(
	attrs ...Attr,
) Logger {
	return n
}

var Nop Logger = noopLogger{}
