package commoncli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// SetFlagsForLogging sets the command line flags for logging.
func SetFlagsForLogging() []*string {
	basename := filepath.Base(os.Args[0]) + ".log"
	logFile := flag.String("logfile", filepath.Join("/tmp", basename), "logging file location")
	logLevel := flag.String("loglevel", "debug", "Logging Level: debug, info, warning, error, fatal, panic.")
	return []*string{logFile, logLevel}
}

// SetupLogging creates the logger from the command line parameters.
func SetupLogging(args ...*string) error {
	if len(args) < 1 {
		return fmt.Errorf("Invalid log params")
	}
	level, err := logrus.ParseLevel(*args[1])
	if err != nil {
		logrus.Fatal(err)
		return err
	}
	logrus.SetLevel(level)

	filename := *args[0]
	outputTarget, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	logrus.SetOutput(outputTarget)
	return nil
}
