package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/Microsoft/opengcs/internal/oc"
	"github.com/Microsoft/opengcs/internal/runtime/hcsv2"
	"github.com/Microsoft/opengcs/service/gcs/bridge"
	"github.com/Microsoft/opengcs/service/gcs/core/gcs"
	"github.com/Microsoft/opengcs/service/gcs/runtime/runc"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	"github.com/containerd/cgroups"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

func main() {
	logLevel := flag.String("loglevel", "debug", "Logging Level: debug, info, warning, error, fatal, panic.")
	logFile := flag.String("logfile", "", "Logging Target: An optional file name/path. Omit for console output.")
	logFormat := flag.String("log-format", "text", "Logging Format: text or json")
	useInOutErr := flag.Bool("use-inouterr", false, "If true use stdin/stdout for bridge communication and stderr for logging")
	v4 := flag.Bool("v4", false, "enable the v4 protocol support and v2 schema")
	rootMemReserveBytes := flag.Uint64("root-mem-reserve-bytes", 75*1024*1024, "the amount of memory reserved for the orchestration, the rest will be assigned to containers")
	gcsMemLimitBytes := flag.Uint64("gcs-mem-limit-bytes", 10*1024*1024, "the maximum amount of memory the gcs can use")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\nUsage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "    %s -loglevel=debug -logfile=/run/gcs/gcs.log\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    %s -loglevel=info -logfile=stdout\n", os.Args[0])
	}

	flag.Parse()

	// If v4 enable opencensus
	if *v4 {
		trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
		trace.RegisterExporter(&oc.LogrusExporter{})
	}

	// Use a file instead of stdout
	if *logFile != "" {
		logFileHandle, err := os.OpenFile(*logFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"path":          *logFile,
				logrus.ErrorKey: err,
			}).Fatal("opengcs::main - failed to create log file")
		}
		logrus.SetOutput(logFileHandle)
	}

	switch *logFormat {
	case "text":
		// retain logrus's default.
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano, // include ns for accurate comparisons on the host
		})
	default:
		logrus.WithFields(logrus.Fields{
			"log-format": *logFormat,
		}).Fatal("opengcs::main - unknown log-format")
	}

	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		logrus.Fatal(err)
	}

	logrus.SetLevel(level)

	baseLogPath := "/run/gcs/c"
	baseStoragePath := "/run/gcs/c"

	logrus.Info("GCS started")
	tport := &transport.VsockTransport{}
	rtime, err := runc.NewRuntime(baseLogPath)
	if err != nil {
		logrus.WithError(err).Fatal("opengcs::main - failed to initialize new runc runtime")
	}
	coreint := gcs.NewGCSCore(baseLogPath, baseStoragePath, rtime, tport)
	mux := bridge.NewBridgeMux()
	b := bridge.Bridge{
		Handler:  mux,
		EnableV4: *v4,
	}
	h := hcsv2.NewHost(rtime, tport)
	b.AssignHandlers(mux, coreint, h)

	var bridgeIn io.ReadCloser
	var bridgeOut io.WriteCloser
	if *useInOutErr {
		bridgeIn = os.Stdin
		bridgeOut = os.Stdout
	} else {
		const commandPort uint32 = 0x40000000
		bridgeCon, err := tport.Dial(commandPort)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"port":          commandPort,
				logrus.ErrorKey: err,
			}).Fatal("opengcs::main - failed to dial host vsock connection")
		}
		bridgeIn = bridgeCon
		bridgeOut = bridgeCon
	}

	// Setup the UVM cgroups to protect against a workload taking all available
	// memory and causing the GCS to malfunction we create two cgroups: gcs,
	// containers.
	//
	// The containers cgroup is limited only by {Totalram - 75 MB
	// (reservation)}.
	//
	// The gcs cgroup is limited to 10 MB to prevent unknown memory leaks over
	// time from affecting workload memory.
	sinfo := syscall.Sysinfo_t{}
	if err := syscall.Sysinfo(&sinfo); err != nil {
		logrus.WithError(err).Fatal("opengcs::main - failed to get sys info")
	}
	containersLimit := int64(sinfo.Totalram - *rootMemReserveBytes)
	containersControl, err := cgroups.New(cgroups.V1, cgroups.StaticPath("/containers"), &oci.LinuxResources{
		Memory: &oci.LinuxMemory{
			Limit: &containersLimit,
		},
	})
	if err != nil {
		logrus.WithError(err).Fatal("opengcs::main - failed to create containers cgroup")
	}
	defer containersControl.Delete()
	gcsLimit := int64(*gcsMemLimitBytes)
	gcsControl, err := cgroups.New(cgroups.V1, cgroups.StaticPath("/gcs"), &oci.LinuxResources{
		Memory: &oci.LinuxMemory{
			Limit: &gcsLimit,
		},
	})
	if err != nil {
		logrus.WithError(err).Fatal("opengcs::main - failed to create gcs cgroup")
	}
	defer gcsControl.Delete()
	if err := gcsControl.Add(cgroups.Process{Pid: os.Getpid()}); err != nil {
		logrus.WithError(err).Fatal("opengcs::main - failed add gcs pid to gcs cgroup")
	}

	err = b.ListenAndServe(bridgeIn, bridgeOut)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
		}).Fatal("opengcs::main - failed to serve gcs service")
	}
}
