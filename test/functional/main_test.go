package functional

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/resources"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
	"github.com/containerd/containerd"
	"github.com/sirupsen/logrus"
)

const (
	bytesPerMB = 1024 * 1024
)

var (
	debug                                 bool
	pauseDurationOnCreateContainerFailure time.Duration

	// flags
	flagContainerdEndpoint  = flag.String("ctr-endpoint", "tcp://127.0.0.1:2376", "Address for containerd's GRPC server")
	flagContainerdNamespace = flag.String("ctr-namespace", "k8s.io", "Containerd namespace")
	flagCtrPath             = flag.String("ctr-path", testutilities.DefaultCtrPath(), "Path to ctr.exe")
	flagLinuxBootFilesPath  = flag.String("linux-bootfiles",
		"C:\\ContainerPlat\\LinuxBootFiles",
		"Path to LCOW UVM boot files (rootfs.vhd, initrd.img, kernel, etc.)")
)

func init() {
	if len(os.Getenv("HCSSHIM_FUNCTIONAL_TESTS_DEBUG")) > 0 {
		debug = true
	}
	flag.BoolVar(&debug, "debug", debug, "Set logging level to debug [%%HCSSHIM_FUNCTIONAL_TESTS_DEBUG%%]")

	// This allows for debugging a utility VM.
	if s := os.Getenv("HCSSHIM_FUNCTIONAL_TESTS_PAUSE_ON_CREATECONTAINER_FAIL_IN_MINUTES"); s != "" {
		if t, err := strconv.Atoi(s); err == nil {
			pauseDurationOnCreateContainerFailure = time.Duration(t) * time.Minute
		}
	}
	flag.DurationVar(&pauseDurationOnCreateContainerFailure,
		"container-creation-failure-pause",
		pauseDurationOnCreateContainerFailure,
		"The number of minutes to wait after a container creation failure to try again "+
			"[%%HCSSHIM_FUNCTIONAL_TESTS_PAUSE_ON_CREATECONTAINER_FAIL_IN_MINUTES%%]")

	if debug {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	}

	// Try to stop any pre-existing compute processes
	cmd := exec.Command("powershell", `get-computeprocess | stop-computeprocess -force`)
	_ = cmd.Run()

}

func TestMain(m *testing.M) {
	flag.Parse()

	ctro := testutilities.GetCtrClientOptions()
	ctro.Path = *flagCtrPath
	ctro.Address = *flagContainerdEndpoint
	ctro.Namespace = *flagContainerdNamespace

	os.Exit(m.Run())
}

func CreateContainerTestWrapper(ctx context.Context, options *hcsoci.CreateOptions) (cow.Container, *resources.Resources, error) {
	if pauseDurationOnCreateContainerFailure != 0 {
		options.DoNotReleaseResourcesOnFailure = true
	}
	s, r, err := hcsoci.CreateContainer(ctx, options)
	if err != nil {
		logrus.Warnf("Test is pausing for %s for debugging CreateContainer failure", pauseDurationOnCreateContainerFailure)
		time.Sleep(pauseDurationOnCreateContainerFailure)
		_ = resources.ReleaseResources(ctx, r, options.HostingSystem, true)
	}
	return s, r, err
}

func getCtrOptions() testutilities.CtrClientOptions {
	return testutilities.CtrClientOptions{
		Path: *flagLinuxBootFilesPath,
		CtrdClientOptions: getCtrdOptions(),
	}
}

func getCtrdOptions() testutilities.CtrdClientOptions {
	return testutilities.CtrdClientOptions{
		Address:   *flagContainerdEndpoint,
		Namespace: *flagContainerdNamespace,
	}
}

func getCtrdClient(t *testing.T) (*containerd.Client, context.Context) {
	return getCtrdOptions().NewClient(context.Background(), t)
}