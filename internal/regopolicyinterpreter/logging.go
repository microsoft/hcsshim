package regopolicyinterpreter

import (
	"context"
	"encoding/json"
	"os"
	"sync/atomic"

	"github.com/sirupsen/logrus"

	hcsshimLog "github.com/Microsoft/hcsshim/internal/log"
)

type logLevel struct {
	level LogLevel
}

func (ll *logLevel) SetLevel(level LogLevel) {
	atomic.StoreInt32((*int32)(&ll.level), int32(level))
}

func (ll *logLevel) Level() LogLevel {
	return LogLevel(atomic.LoadInt32((*int32)(&ll.level)))
}

type InterpreterLogger interface {
	LogInfo(ctx context.Context, msg string, args ...interface{})
	LogResult(ctx context.Context, rule string, resultSet interface{})
	LogMetadata(ctx context.Context, data map[string]interface{})
	SetLevel(level LogLevel)
	Level() LogLevel
	Close(ctx context.Context) error
}

type fileLogger struct {
	*logLevel
	logFile *os.File
	logger  *logrus.Logger
}

var _ InterpreterLogger = (*fileLogger)(nil)
var _ InterpreterLogger = (*logrusLogger)(nil)

func NewFileLogger(path string, level LogLevel) (InterpreterLogger, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	lr := &logrus.Logger{
		Out: file,
		ExitFunc: func(exitCode int) {
			file.Close()
			os.Exit(exitCode)
		},
		Formatter: &logrus.TextFormatter{
			FullTimestamp: true,
		},
	}
	return &fileLogger{
		logger:  lr,
		logFile: file,
		logLevel: &logLevel{
			level: level,
		},
	}, nil
}

func (l *fileLogger) LogInfo(_ context.Context, msg string, args ...interface{}) {
	if l.Level() < LogInfo || len(msg) == 0 {
		return
	}
	l.logger.Printf("INFO: "+msg, args...)
}

func (l *fileLogger) LogResult(_ context.Context, rule string, resultSet interface{}) {
	if l.Level() < LogResults {
		return
	}

	prefix := "RESULT: "
	contents, err := json.Marshal(resultSet)
	if err != nil {
		l.logger.Printf(prefix+"error marshaling result set: %s\n", err.Error())
	} else {
		l.logger.Printf(prefix+"%s -> %s", rule, string(contents))
	}
}

func (l *fileLogger) LogMetadata(_ context.Context, data map[string]interface{}) {
	if l.Level() < LogMetadata {
		return
	}

	prefix := "METADATA: "
	contents, err := json.Marshal(data["metadata"])
	if err != nil {
		l.logger.Printf(prefix+"error marshaling metadata: %s\n", err.Error())
	} else {
		l.logger.Println(prefix + string(contents))
	}
}

func (l *fileLogger) Close(_ context.Context) error {
	l.SetLevel(LogNone)
	return l.logFile.Close()
}

type logrusLogger struct {
	*logLevel
}

func NewLogrusLogger(level LogLevel) (InterpreterLogger, error) {
	return &logrusLogger{
		logLevel: &logLevel{
			level: level,
		},
	}, nil
}

func (l *logrusLogger) LogInfo(ctx context.Context, msg string, args ...interface{}) {
	if l.Level() < LogInfo {
		return
	}
	hcsshimLog.G(ctx).Infof(msg, args...)
}

func (l *logrusLogger) LogResult(ctx context.Context, rule string, resultSet interface{}) {
	if l.Level() < LogResults {
		return
	}

	contents, err := json.Marshal(resultSet)
	if err != nil {
		hcsshimLog.G(ctx).WithError(err).WithField("rule", rule).Warning("error marshaling metadata")
	} else {
		hcsshimLog.G(ctx).WithFields(logrus.Fields{
			"rule":      rule,
			"resultSet": string(contents),
		}).Info("result set")
	}
}

func (l *logrusLogger) LogMetadata(ctx context.Context, data map[string]interface{}) {
	if l.Level() < LogMetadata {
		return
	}
	contents, err := json.Marshal(data["metadata"])
	if err != nil {
		hcsshimLog.G(ctx).WithError(err).Error("error marshaling metadata")
	} else {
		hcsshimLog.G(ctx).WithFields(logrus.Fields{
			"metadata": string(contents),
		}).Info("metadata")
	}
}

func (l *logrusLogger) Close(_ context.Context) error {
	return nil
}
