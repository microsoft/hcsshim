package shim

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/containerd/containerd/runtime/v2/shim"
	"github.com/urfave/cli"
)

var startCommand = cli.Command{
	Name: "start",
	Usage: `This command will launch new shims.

The start command, as well as all binary calls to the shim, has the bundle for the container set as the cwd.

The start command MUST return an address to a shim for containerd to issue API requests for container operations.

The start command can either start a new shim or return an address to an existing shim based on the shim's logic.`,
	Action: func(context *cli.Context) (err error) {
		// On Windows there are two scenarios that will launch a shim.
		//
		// 1. The config.json in the bundle path contains the kubernetes
		// annotation `io.kubernetes.cri.container-type = sandbox`. This shim
		// will be served for the POD itself and all
		// `io.kubernetes.cri.container-type = container` with a matching
		// `io.kubernetes.cri.sandbox-id`. For any calls to start where the
		// config.json contains the `io.kubernetes.cri.container-type =
		// container` annotation a shim path to the
		// `io.kubernetes.cri.sandbox-id` will be returned.
		//
		// 2. The container does not have any kubernetes annotations and
		// therefore is a process isolated Windows Container, a hypervisor
		// isolated Windows Container, or a hypervisor isolated Linux Container
		// on Windows.

		const addrFmt = "\\\\.\\pipe\\ProtectedPrefix\\Administrators\\containerd-shim-%s-%s-pipe"
		var address string

		a, err := getSpecAnnotationsFromBundleRoot()
		if err != nil {
			return err
		}

		if v := a["io.kubernetes.cri.container-type"]; v == "container" {
			id := a["io.kubernetes.cri.sandbox-id"]
			if id == "" {
				return errors.New("invalid 'io.kubernetes.cri.sandbox-id' for 'io.kubernetes.cri.container-type == container'")
			}
			address = fmt.Sprintf(addrFmt, namespaceFlag, idFlag)
		}

		// We need to serve a new one.
		if address == "" {
			self, err := os.Executable()
			if err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			r, w, err := os.Pipe()
			if err != nil {
				return err
			}
			defer r.Close()
			defer w.Close()

			address := fmt.Sprintf(addrFmt, namespaceFlag, idFlag)
			cmd := &exec.Cmd{
				Path: self,
				Args: []string{
					"--namespace", namespaceFlag,
					"--address", addressFlag,
					"--publish-binary", containerdBinaryFlag,
					"--id", idFlag,
					"serve",
					"--socket", address,
				},
				Env:    os.Environ(),
				Dir:    cwd,
				Stderr: w,
			}

			if err := cmd.Start(); err != nil {
				return err
			}
			defer func() {
				if err != nil {
					cmd.Process.Kill()
				}
			}()

			buf := bytes.Buffer{}
			if _, err := io.Copy(&buf, r); err != nil {
				return err
			}
			stderrOut := buf.String()
			if stderrOut != "" {
				return errors.New("failed to serve shim: " + stderrOut)
			}
			if err := shim.WritePidFile(filepath.Join(cwd, "shim.pid"), cmd.Process.Pid); err != nil {
				return err
			}
			if err := shim.WriteAddress(filepath.Join(cwd, "address"), address); err != nil {
				return err
			}
		}

		// Write the address new or existing to stdout
		if _, err := os.Stdout.WriteString(address); err != nil {
			return err
		}
		return nil
	},
}

func getSpecAnnotationsFromBundleRoot() (map[string]string, error) {
	// specAnnotations is a minimal representation for oci.Spec that we need
	// to serve a shim.
	type specAnnotations struct {
		// Annotations contains arbitrary metadata for the container.
		Annotations map[string]string `json:"annotations,omitempty"`
	}
	path, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var spec specAnnotations
	if err := json.NewDecoder(f).Decode(&spec); err != nil {
		return nil, err
	}
	return spec.Annotations, nil
}
