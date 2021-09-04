package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Microsoft/hcsshim/hcn"
)

func main() {
	if len(os.Args) < 4 {
		os.Exit(1)
	}

	path := filepath.Join(filepath.Dir(os.Args[0]), "Test.wprp")
	if err := exec.Command("wpr", "-start", path).Run(); err != nil {
		log.Fatalf("failed to wpr start: %s", err)
	}

	var (
		netName  = os.Args[1]
		etlFile  = os.Args[2]
		dumpFile = os.Args[3]
	)

	network := &hcn.HostComputeNetwork{
		Name: netName,
		Type: hcn.NAT,
		SchemaVersion: hcn.SchemaVersion{
			Major: 2,
			Minor: 2,
		},
	}
	network, err := network.Create()
	if err != nil {
		log.Fatalf("failed to create hns network: %s", err)
	}

	if err := network.Delete(); err != nil {
		log.Fatalf("failed to delete hns network: %s", err)
	}

	if err := exec.Command("wpr", "-stop", etlFile).Run(); err != nil {
		log.Fatal(err)
	}

	if err := exec.Command("tracerpt", etlFile, "-o", dumpFile, "-of", "XML").Run(); err != nil {
		log.Fatal(err)
	}
}
