package main

import (
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"

	"github.com/Microsoft/opengcs/service/gcs/bridge"
	"github.com/Microsoft/opengcs/service/gcs/core/gcs"
	"github.com/Microsoft/opengcs/service/gcs/oslayer/realos"
	"github.com/Microsoft/opengcs/service/gcs/runtime/runc"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	"github.com/Microsoft/opengcs/service/libs/commonutils"
)

func main() {
	// parse command line parameters and init logger
	if err := utils.ProcessCommandlineOptions(); err != nil {
		logrus.Fatalf("%+v", err)
	}

	// Set logrus output file.
	// TODO: Consolidate all logs into one file, probably with logrus.
	const logFilePath = "/tmp/logrus.log"
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		logrus.Fatalf("%+v", errors.Errorf("failed to create log file %s", logFilePath))
	}
	logrus.SetOutput(logFile)

	utils.LogMsg("GCS started")
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
