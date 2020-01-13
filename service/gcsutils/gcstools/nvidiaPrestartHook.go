package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

const (
	nvidiaToolBinary    = "nvidia-container-cli"
	pidPrefix           = "--pid"
	lcowNvidiaMountPath = "/run/nvidia"
	nvidiaDebugFilePath = "/nvidia-container.log"
)

// from containerd oci-hook.go
func loadHookState(r io.Reader) (*specs.State, error) {
	var s *specs.State
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return nil, err
	}
	return s, nil
}

func runHook() error {
	state, err := loadHookState(os.Stdin)
	if err != nil {
		return err
	}

	// find the pid argument and replace with correct pid
	args := os.Args[1:]
	for i, a := range args {
		if strings.HasPrefix(a, pidPrefix) {
			args[i] = fmt.Sprintf(a, state.Pid)
		}
	}

	// run ldconfig to ensure the host's ld cache has the nvidia files
	libPath := filepath.Join(lcowNvidiaMountPath, "lib")
	ldconfigOut, err := exec.Command("ldconfig", libPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run ldconfig in nvidiaPrestartHook with: %v, %v", string(ldconfigOut), err)
	}

	nvidiaToolPath, err := exec.LookPath(nvidiaToolBinary)
	if err != nil {
		return fmt.Errorf("failed to find %v in path: %v", nvidiaToolBinary, err)
	}

	out, err := exec.Command(nvidiaToolPath, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run nvidia cli tool with: %v, %v", string(out), err)
	}

	return nil
}

func logDebugFile() {
	contents, err := ioutil.ReadFile(nvidiaDebugFilePath)
	if err != nil {
		logrus.Errorf("failed to read nvidia-container-cli debug file: %s", err)
		return
	}
	numBytesInContents := len(contents)

	// since we forward logs on windows to etw, limit log size to 8KB to avoid issues
	maxLogSize := 8000
	startBytes := 0
	i := 0
	for startBytes < numBytesInContents {
		bytesLeft := len(contents[startBytes:])
		chunkSize := maxLogSize
		if bytesLeft < maxLogSize {
			chunkSize = bytesLeft
		}
		stopBytes := startBytes + chunkSize
		output := string(contents[startBytes:stopBytes])
		logrus.WithField("output", output).Infof("nvidia-container-cli debug part %d", i)
		i += 1
		startBytes += chunkSize
	}

}

func nvidiaPrestartHookMain() {
	if err := runHook(); err != nil {
		logrus.Errorf("error in nvidia prestart hook: %s", err)
		logDebugFile()
		os.Exit(-1)
	}
	os.Exit(0)
}
