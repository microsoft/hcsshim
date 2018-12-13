package shim

import (
	"errors"
	"fmt"
	"strings"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
)

const usage = ``

// Add a manifest to get proper Windows version detection.
//
// goversioninfo can be installed with "go get github.com/josephspurrier/goversioninfo/cmd/goversioninfo"

//go:generate goversioninfo -platform-specific

// version will be populated by the Makefile, read from
// VERSION file of the source code.
var version = ""

// gitCommit will be the hash that the binary was built from
// and will be populated by the Makefile
var gitCommit = ""

var (
	namespaceFlag        string
	addressFlag          string
	containerdBinaryFlag string

	idFlag string
)

func main() {
	app := cli.NewApp()
	app.Name = "containerd-shim-runhcs-v1"
	app.Usage = usage

	var v []string
	if version != "" {
		v = append(v, version)
	}
	if gitCommit != "" {
		v = append(v, fmt.Sprintf("commit: %s", gitCommit))
	}
	v = append(v, fmt.Sprintf("spec: %s", specs.Version))
	app.Version = strings.Join(v, "\n")

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "namespace",
			Usage: "the namespace of the container",
		},

		cli.StringFlag{
			Name:  "address",
			Usage: "the address of the containerd's main socket",
		},
		cli.StringFlag{
			Name:  "publish-binary",
			Usage: "the binary path to publish events back to containerd",
		},
		cli.StringFlag{
			Name:  "id",
			Usage: "the id of the container",
		},
	}
	app.Commands = []cli.Command{
		startCommand,
		deleteCommand,
		serveCommand,
	}
	app.Before = func(context *cli.Context) error {
		if namespaceFlag = context.GlobalString("namespace"); namespaceFlag == "" {
			return errors.New("namespace required")
		}
		if addressFlag = context.GlobalString("address"); addressFlag == "" {
			return errors.New("address required")
		}
		if containerdBinaryFlag = context.GlobalString("publish-binary"); containerdBinaryFlag == "" {
			return errors.New("publish-binary required")
		}
		return nil
	}
}
