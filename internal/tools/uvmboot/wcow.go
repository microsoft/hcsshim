package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/hcsoci"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var (
	wcowDockerImage string
	wcowCommandLine string
	wcowImage       string
)

var wcowCommand = cli.Command{
	Name:  "wcow",
	Usage: "boot a WCOW UVM",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:        "exec",
			Usage:       "Command to execute in the UVM.",
			Destination: &wcowCommandLine,
		},
		cli.StringFlag{
			Name:        "docker-image",
			Usage:       "Docker image to use for the UVM image",
			Destination: &wcowDockerImage,
		},
		cli.StringFlag{
			Name:        "image",
			Usage:       "Path for the UVM image",
			Destination: &wcowImage,
		},
	},
	Action: func(c *cli.Context) error {
		runMany(c, func(id string) error {
			options := uvm.NewDefaultOptionsWCOW(id, "")
			setGlobalOptions(c, options.Options)
			var layers []string
			if wcowImage != "" {
				layer, err := filepath.Abs(wcowImage)
				if err != nil {
					return err
				}
				layers = []string{layer}
			} else {
				if wcowDockerImage == "" {
					wcowDockerImage = "mcr.microsoft.com/windows/nanoserver:1809"
				}
				var err error
				layers, err = getLayers(wcowDockerImage)
				if err != nil {
					return err
				}
			}
			tempDir, err := ioutil.TempDir("", "uvmboot")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tempDir)
			options.LayerFolders = append(layers, tempDir)
			vm, err := uvm.CreateWCOW(options)
			if err != nil {
				return err
			}
			defer vm.Close()
			if err := vm.Start(); err != nil {
				return err
			}
			if wcowCommandLine != "" {
				cmd := hcsoci.Command(vm, "cmd.exe", "/c", wcowCommandLine)
				cmd.Spec.User.Username = `NT AUTHORITY\SYSTEM`
				cmd.Log = logrus.NewEntry(logrus.StandardLogger())
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stdout
				err = cmd.Run()
				if err != nil {
					return err
				}
			}
			vm.Terminate()
			vm.Wait()
			return vm.ExitError()
		})
		return nil
	},
}

func getLayers(imageName string) ([]string, error) {
	cmd := exec.Command("docker", "inspect", imageName, "-f", `"{{.GraphDriver.Data.dir}}"`)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to find layers for %s", imageName)
	}
	imagePath := strings.Replace(strings.TrimSpace(string(out)), `"`, ``, -1)
	layers, err := getLayerChain(imagePath)
	if err != nil {
		return nil, err
	}
	return append([]string{imagePath}, layers...), nil
}

func getLayerChain(layerFolder string) ([]string, error) {
	jPath := filepath.Join(layerFolder, "layerchain.json")
	content, err := ioutil.ReadFile(jPath)
	if err != nil {
		return nil, err
	}
	var layerChain []string
	err = json.Unmarshal(content, &layerChain)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal layerchain: %s", err)
	}
	return layerChain, nil
}
