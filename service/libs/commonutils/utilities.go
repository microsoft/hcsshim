package utils

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"

	"github.com/pkg/errors"
)

// variable declaration for logging
var loglevel string
var logger *log.Logger

var gcsUsage = func() {
	fmt.Fprintf(os.Stderr, "\nUsage of %s:\n", os.Args[0])
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "Examples:\n")
	fmt.Fprintf(os.Stderr, "    %s -loglevel=verbose -logfile=gcslog.txt (default)\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "    %s -loglevel=verbose -logfile=stdout\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "    %s -loglevel=none \n", os.Args[0])
}

func ProcessCommandlineOptions() error {
	var logLevelPtr = flag.String("loglevel", "verbose", "logging level: either none or verbose")
	var logFilePtr = flag.String("logfile", "stdout", "logging target: a file name or stdout")

	// parse commandline
	flag.Usage = gcsUsage
	flag.Parse()

	// set logging options
	if err := SetLoggingOptions(*logLevelPtr, *logFilePtr); err != nil {
		return err
	}
	return nil
}

func SetLoggingOptions(level string, file string) error {
	loglevel = level
	logfile := file

	if loglevel != "none" && loglevel != "verbose" {
		return errors.New("SetLoggingOptions failed with invalid loglevel parameter")
	}

	var outputTarget io.Writer
	if loglevel == "verbose" {
		if logfile == "stdout" {
			outputTarget = os.Stdout
		} else {
			var err error
			outputTarget, err = os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return errors.Wrapf(err, "failed opening log output file %s", logfile)
			}
		}
		logger = log.New(outputTarget, "gcs:", log.Ltime)
	}
	return nil
}

func LogMsg(message string) {
	switch loglevel {
	case "verbose":
		pc := make([]uintptr, 10)
		runtime.Callers(2, pc)
		f := runtime.FuncForPC(pc[1])
		file, line := f.FileLine(pc[1])
		logger.Printf("%s:%d %s() %s\n", file, line, f.Name(), message)

	default:
		// skip output
	}
}

func LogMsgf(format string, a ...interface{}) {
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, format, a...)
	LogMsg(buffer.String())
}
