package main

import (
	"os"

	"github.com/Microsoft/hcsshim/hcn"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(1)
	}
	network := &hcn.HostComputeNetwork{
		Name: os.Args[1],
		Type: hcn.NAT,
		SchemaVersion: hcn.SchemaVersion{
			Major: 2,
			Minor: 2,
		},
	}
	network, err := network.Create()
	if err != nil {
		os.Exit(1)
	}
}
