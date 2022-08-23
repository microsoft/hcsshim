//go:build windows && functional

// This package tests the internals of hcsshim, independent of the OCI interfaces it exposes
// and the container runtime (or CRI API) that normally would be communicating with the shim.
//
// While these tests may overlap with CRI/containerd or shim tests, they exercise `internal/*`
// code paths and primitives directly.
package functional

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/containerd/containerd"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/resources"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/winapi"

	testctrd "github.com/Microsoft/hcsshim/test/internal/containerd"
	testflag "github.com/Microsoft/hcsshim/test/internal/flag"
	"github.com/Microsoft/hcsshim/test/internal/require"
)

// owner field for uVMs
const hcsOwner = "hcsshim-functional-tests"

const (
	featureLCOW        = "LCOW"
	featureWCOW        = "WCOW"
	featureContainer   = "container"
	featureHostProcess = "HostProcess"
	featureUVMMem      = "UVMMem"
	featurePlan9       = "Plan9"
	featureSCSI        = "SCSI"
	featureScratch     = "Scratch"
	featureVSMB        = "vSMB"
	featureVPMEM       = "vPMEM"
)

var allFeatures = []string{
	featureLCOW,
	featureWCOW,
	featureHostProcess,
	featureContainer,
	featureUVMMem,
	featurePlan9,
	featureSCSI,
	featureScratch,
	featureVSMB,
	featureVPMEM,
}

// todo: use a new containerd namespace and then nuke everything in it

var (
	debug                                 bool
	pauseDurationOnCreateContainerFailure time.Duration

	// flags
	flagFeatures            = testflag.NewFeatureFlag(allFeatures)
	flagContainerdAddress   = flag.String("ctr-address", "tcp://127.0.0.1:2376", "Address for containerd's GRPC server")
	flagContainerdNamespace = flag.String("ctr-namespace", "k8s.io", "Containerd namespace")
	flagCtrExePath          = flag.String("ctr-path", `C:\ContainerPlat\ctr.exe`, "Path to ctr.exe")
	flagLinuxBootFilesPath  = flag.String("linux-bootfiles",
		`C:\\ContainerPlat\\LinuxBootFiles`,
		"Path to LCOW UVM boot files (rootfs.vhd, initrd.img, kernel, vmlinux)")
)

func init() {
	if !winapi.IsElevated() {
		log.Fatal("tests must be run in an elevated context")
	}

	if _, ok := os.LookupEnv("HCSSHIM_FUNCTIONAL_TESTS_DEBUG"); ok {
		debug = true
	}
	flag.BoolVar(&debug, "debug", debug, "Set logging level to debug [%HCSSHIM_FUNCTIONAL_TESTS_DEBUG%]")

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
			"[%HCSSHIM_FUNCTIONAL_TESTS_PAUSE_ON_CREATECONTAINER_FAIL_IN_MINUTES%]")
}

func TestMain(m *testing.M) {
	flag.Parse()

	lvl := logrus.WarnLevel
	if vf := flag.Lookup("test.v"); debug || (vf != nil && vf.Value.String() == strconv.FormatBool(true)) {
		lvl = logrus.DebugLevel
	}
	logrus.SetLevel(lvl)
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	logrus.Infof("using features %q", flagFeatures.S.Strings())

	e := m.Run()

	// close any uVMs that escaped
	cmdStr := ` foreach ($vm in Get-ComputeProcess -Owner '` + hcsOwner + `') { Write-Output "uVM $($vm.Id) was left running" ; Stop-ComputeProcess -Force -Id $vm.Id } `
	cmd := exec.Command("powershell", "-NoLogo", " -NonInteractive", "-Command", cmdStr)
	o, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Warningf("could not call %q to clean up remaining uVMs: %v", cmdStr, err)
	} else if len(o) > 0 {
		logrus.Warningf(string(o))
	}

	os.Exit(e)
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

func requireFeatures(t testing.TB, features ...string) {
	require.Features(t, flagFeatures.S, features...)
}

func getContainerdOptions() testctrd.ContainerdClientOptions {
	return testctrd.ContainerdClientOptions{
		Address:   *flagContainerdAddress,
		Namespace: *flagContainerdNamespace,
	}
}

func newContainerdClient(ctx context.Context, t testing.TB) (context.Context, context.CancelFunc, *containerd.Client) {
	return getContainerdOptions().NewClient(ctx, t)
}

func defaultLCOWOptions(t testing.TB) *uvm.OptionsLCOW {
	opts := uvm.NewDefaultOptionsLCOW(cleanName(t.Name()), "")
	opts.BootFilesPath = *flagLinuxBootFilesPath

	return opts
}

func defaultWCOWOptions(t testing.TB) *uvm.OptionsWCOW {
	opts := uvm.NewDefaultOptionsWCOW(cleanName(t.Name()), "")

	return opts
}

var _nameRegex = regexp.MustCompile(`[\\\/\s]`)

func cleanName(n string) string {
	return _nameRegex.ReplaceAllString(n, "")
}
