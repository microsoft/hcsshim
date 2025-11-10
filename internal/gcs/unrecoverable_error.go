//go:build linux
// +build linux

package gcs

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/pkg/amdsevsnp"
	"github.com/sirupsen/logrus"
)

// UnrecoverableError logs the error and then puts the current thread into an
// infinite sleep loop.  This is to be used instead of panicking, as the
// behaviour of GCS panics is unpredictable.  This function can be extended to,
// for example, try to shutdown the VM cleanly.
func UnrecoverableError(err error) {
	buf := make([]byte, 300*(1<<10))
	stackSize := runtime.Stack(buf, true)
	stackTrace := string(buf[:stackSize])

	errPrint := fmt.Sprintf(
		"Unrecoverable error in GCS: %v\n%s",
		err, stackTrace,
	)
	isSnp := amdsevsnp.IsSNP()
	if isSnp {
		errPrint += "\nThis thread will now enter an infinite loop."
	}
	log.G(context.Background()).WithError(err).Logf(
		logrus.FatalLevel,
		"%s",
		errPrint,
	)

	if !isSnp {
		panic("Unrecoverable error in GCS: " + err.Error())
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", errPrint)
		for {
			time.Sleep(time.Hour)
		}
	}
}
