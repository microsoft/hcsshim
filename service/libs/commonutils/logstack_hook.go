package commonutils

import (
	"fmt"
	"runtime"

	"github.com/Sirupsen/logrus"
)

type stacklogger struct {
	levels []logrus.Level
}

// NewStackHook creates a new hook to append the stack to log messages.
func NewStackHook(levels []logrus.Level) logrus.Hook {
	return &stacklogger{levels}
}

func (h *stacklogger) Levels() []logrus.Level {
	return h.levels
}

func (h *stacklogger) Fire(e *logrus.Entry) error {
	pc := make([]uintptr, 10)
	runtime.Callers(2, pc)
	f := runtime.FuncForPC(pc[1])
	file, line := f.FileLine(pc[1])
	e.Message = fmt.Sprintf("%s:%d %s() %s", file, line, f.Name(), e.Message)
	return nil
}
