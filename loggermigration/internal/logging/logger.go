package logging

import "context"

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

type Attr struct {
	Key   string
	Value any
}

type Logger interface {
	Log(
		ctx context.Context,
		level Level,
		msg string,
		attrs ...Attr,
	)

	With(attrs ...Attr) Logger
}
