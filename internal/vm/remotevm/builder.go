//go:build windows

package remotevm

import (
	"context"
	"io"
	"net"
	"os/exec"

	"github.com/containerd/ttrpc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/vm"
	"github.com/Microsoft/hcsshim/internal/vmservice"
)

var _ vm.UVMBuilder = &utilityVMBuilder{}

type utilityVMBuilder struct {
	id      string
	guestOS vm.GuestOS
	job     *jobobject.JobObject
	config  *vmservice.VMConfig
	client  vmservice.VMService
}

func NewUVMBuilder(ctx context.Context, id, owner, binPath, addr string, guestOS vm.GuestOS) (_ vm.UVMBuilder, err error) {
	var job *jobobject.JobObject
	if binPath != "" {
		log.G(ctx).WithFields(logrus.Fields{
			"binary":  binPath,
			"address": addr,
		}).Debug("starting remotevm server process")

		opts := &jobobject.Options{
			Name: id,
		}
		job, err = jobobject.Create(ctx, opts)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create job object for remotevm process")
		}

		cmd := exec.Command(binPath, "--ttrpc", addr)
		p, err := cmd.StdoutPipe()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create stdout pipe")
		}

		if err := cmd.Start(); err != nil {
			return nil, errors.Wrap(err, "failed to start remotevm server process")
		}

		if err := job.Assign(uint32(cmd.Process.Pid)); err != nil {
			return nil, errors.Wrap(err, "failed to assign remotevm process to job")
		}

		if err := job.SetTerminateOnLastHandleClose(); err != nil {
			return nil, errors.Wrap(err, "failed to set terminate on last handle closed for remotevm job object")
		}

		// Wait for stdout to close. This is our signal that the server is successfully up and running.
		_, _ = io.Copy(io.Discard, p)
	}

	conn, err := net.Dial("unix", addr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to dial remotevm address %q", addr)
	}

	c := ttrpc.NewClient(conn, ttrpc.WithOnClose(func() { conn.Close() }))
	vmClient := vmservice.NewVMClient(c)

	return &utilityVMBuilder{
		id:      id,
		guestOS: guestOS,
		config: &vmservice.VMConfig{
			MemoryConfig:    &vmservice.MemoryConfig{},
			DevicesConfig:   &vmservice.DevicesConfig{},
			ProcessorConfig: &vmservice.ProcessorConfig{},
			SerialConfig:    &vmservice.SerialConfig{},
			ExtraData:       make(map[string]string),
		},
		job:    job,
		client: vmClient,
	}, nil
}

func (uvmb *utilityVMBuilder) Create(ctx context.Context) (vm.UVM, error) {
	// Grab what capabilities the virtstack supports up front.
	capabilities, err := uvmb.client.CapabilitiesVM(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get virtstack capabilities from vmservice")
	}

	if _, err := uvmb.client.CreateVM(ctx, &vmservice.CreateVMRequest{Config: uvmb.config, LogID: uvmb.id}); err != nil {
		return nil, errors.Wrap(err, "failed to create remote VM")
	}

	return &utilityVM{
		id:           uvmb.id,
		job:          uvmb.job,
		config:       uvmb.config,
		client:       uvmb.client,
		capabilities: capabilities,
	}, nil
}
