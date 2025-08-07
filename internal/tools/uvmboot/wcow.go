//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/containerd/console"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"

	"github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

var (
	wcowDockerImage string
	wcowCommandLine string
	wcowImage       string
	wcowUseTerminal bool
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
		cli.BoolFlag{
			Name:        "tty,t",
			Usage:       "create the process in the UVM with a TTY enabled",
			Destination: &wcowUseTerminal,
		},
	},
	Action: func(c *cli.Context) error {
		runMany(c, func(id string) error {
			options := uvm.NewDefaultOptionsWCOW(id, "")
			setGlobalOptions(c, options.Options)
			var layerFolders []string
			if wcowImage != "" {
				layer, err := filepath.Abs(wcowImage)
				if err != nil {
					return err
				}
				layerFolders = []string{layer}
			} else {
				if wcowDockerImage == "" {
					wcowDockerImage = "mcr.microsoft.com/windows/nanoserver:1809"
				}
				var err error
				layerFolders, err = getLayers(wcowDockerImage)
				if err != nil {
					return err
				}
			}
			tempDir, err := os.MkdirTemp("", "uvmboot")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tempDir)
			layerFolders = append(layerFolders, tempDir)
			options.BootFiles, err = layers.GetWCOWUVMBootFilesFromLayers(context.TODO(), nil, layerFolders)
			if err != nil {
				return err
			}
			vm, err := uvm.CreateWCOW(context.TODO(), options)
			if err != nil {
				return err
			}
			defer vm.Close()
			if err := vm.Start(context.TODO()); err != nil {
				return err
			}
			if wcowCommandLine != "" {
				cmd := cmd.Command(vm, "cmd.exe", "/c", wcowCommandLine)
				cmd.Spec.User.Username = `NT AUTHORITY\SYSTEM`
				cmd.Log = log.L.Dup()
				if wcowUseTerminal {
					cmd.Spec.Terminal = true
					cmd.Stdin = os.Stdin
					cmd.Stdout = os.Stdout
					con, err := console.ConsoleFromFile(os.Stdin)
					if err == nil {
						csz, err := con.Size()
						if err != nil {
							return fmt.Errorf("failed to get console size: %w", err)
						}
						cmd.Spec.ConsoleSize = &specs.Box{
							Height: uint(csz.Height),
							Width:  uint(csz.Width),
						}
						err = con.SetRaw()
						if err != nil {
							return err
						}
						defer func() {
							_ = con.Reset()
						}()
					}
				} else {
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stdout
				}
				err = cmd.Run()
				if err != nil {
					return err
				}
			}
			_ = vm.Terminate(context.TODO())
			_ = vm.Wait()
			return vm.ExitError()
		})
		return nil
	},
}

func getLayers(imageName string) ([]string, error) {
	c := exec.Command("docker", "inspect", imageName, "-f", `"{{.GraphDriver.Data.dir}}"`)
	out, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to find layers for %s", imageName)
	}
	imagePath := strings.ReplaceAll(strings.TrimSpace(string(out)), `"`, ``)
	layers, err := getLayerChain(imagePath)
	if err != nil {
		return nil, err
	}
	return append([]string{imagePath}, layers...), nil
}

func getLayerChain(layerFolder string) ([]string, error) {
	jPath := filepath.Join(layerFolder, "layerchain.json")
	content, err := os.ReadFile(jPath)
	if err != nil {
		return nil, err
	}
	var layerChain []string
	err = json.Unmarshal(content, &layerChain)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal layerchain: %w", err)
	}
	return layerChain, nil
}
