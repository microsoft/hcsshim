package logging

import "context"

type loggerKey struct{}

func WithLogger(
	ctx context.Context,
	l Logger,
) context.Context {
	return context.WithValue(
		ctx,
		loggerKey{},
		l,
	)
}

func G(ctx context.Context) Logger {
	l, ok := ctx.Value(loggerKey{}).(Logger)

	if !ok || l == nil {
		return Nop
	}

	return l
}
