package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcs/resourcepaths"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

func main() {
	isAdd := flag.Bool("add", true, "Add or remove the pipe operation")
	containerIdFlag := flag.String("container-id", "", "The id of the container to map the bind to")
	hostPipePathFlag := flag.String("host-pipe-path", "", "The host '\\\\.\\pipe' path to map to the container")
	containerPipePathFlag := flag.String("container-pipe-path", "", "The container '\\\\.\\pipe' path to map the host path to inside the container namespace")
	flag.Parse()

	ctx := context.Background()
	var err error
	if *isAdd {
		err = addPipe(ctx, *containerIdFlag, *hostPipePathFlag, *containerPipePathFlag)
	} else {
		err = removePipe(ctx, *containerIdFlag, *containerPipePathFlag)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func addPipe(ctx context.Context, cid, hpp, cpp string) error {
	err := doWork(ctx, true, cid, hpp, cpp)
	if err != nil {
		return fmt.Errorf("failed to map pipe: %w", err)
	}
	fmt.Println("successfully mapped pipe")
	return nil
}

func removePipe(ctx context.Context, cid, cpp string) error {
	err := doWork(ctx, false, cid, "", cpp)
	if err != nil {
		return fmt.Errorf("failed to unmap pipe: %w", err)
	}
	fmt.Println("successfully unmapped pipe")
	return nil
}

func doWork(ctx context.Context, add bool, cid, hpp, cpp string) error {
	c, err := hcs.OpenComputeSystem(ctx, cid)
	if err != nil {
		return fmt.Errorf("failed to open container handle: %w", err)
	}
	// CPP Fails to map if it contains the pipe prefix. So always remove before modify.
	cpp = strings.TrimPrefix(cpp, `\\.\pipe\`)

	set := &hcsschema.MappedPipe{
		ContainerPipeName: cpp,
	}
	req := &hcsschema.ModifySettingRequest{
		ResourcePath: resourcepaths.SiloMappedPipeResourcePath,
		Settings:     set,
	}
	if add {
		set.HostPath = hpp
	} else {
		req.RequestType = guestrequest.RequestTypeRemove
	}
	err = c.Modify(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to modify container resoources: %w", err)
	}
	return nil
}
