package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"

	"github.com/Microsoft/opengcs/service/gcs/bridge"
	"github.com/Microsoft/opengcs/service/gcs/core/gcs"
	"github.com/Microsoft/opengcs/service/gcs/oslayer/realos"
	"github.com/Microsoft/opengcs/service/gcs/runtime/runc"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	"github.com/Microsoft/opengcs/service/libs/commonutils"
)

func main() {
	logLevel := flag.String("loglevel", "warning", "Logging Level: debug, info, warning, error.")
	logFile := flag.String("logFile", "", "Logging Target: An optional file name/path. Omit for console output.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\nUsage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "    %s -loglevel=debug -logfile=gcs.log (default)\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    %s -loglevel=info -logfile=stdout\n", os.Args[0])
	}

	flag.Parse()

	// Use a file instead of stdout
	if *logFile != "" {
		logFileHandle, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			logrus.Fatalf("failed to create log file %s", *logFile)
		}
		logrus.SetOutput(logFileHandle)
	}

	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		logrus.Fatal(err)
	}

	if level == logrus.DebugLevel {
		logrus.AddHook(commonutils.NewStackHook(logrus.AllLevels))
	}

	logrus.SetLevel(level)

	logrus.Info("GCS started")
	tport := &transport.VsockTransport{}
	rtime, err := runc.NewRuntime()
	if err != nil {
		logrus.Fatalf("%+v", err)
	}
	os := realos.NewOS()
	coreint := gcs.NewGCSCore(rtime, os)
	b := bridge.NewBridge(tport, coreint, true)
	b.CommandLoop()
}
