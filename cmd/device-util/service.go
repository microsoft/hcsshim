//go:build windows

package main

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	cli "github.com/urfave/cli/v2"
	"golang.org/x/sys/windows"
)

var _scm scm

//todo (helsaawy): create a LazyHandle type (similar to windows.LazyDll)

// SC Manager
type scm struct {
	h    windows.Handle
	once sync.Once
}

func (s *scm) handle() windows.Handle {
	s.once.Do(func() {
		var err error
		s.h, err = windows.OpenSCManager(nil, nil, windows.SC_MANAGER_ALL_ACCESS)
		if err != nil {
			panic(fmt.Errorf("could not open SC Manager: %w", err))
		}
	})
	return s.h
}

var serviceCommand = &cli.Command{
	Name:    "service",
	Aliases: []string{"svc"},
	Usage:   "Manage Windows Services",
	Before:  verifyElevated,
	Subcommands: []*cli.Command{
		//todo (helsaawy): add stop and delete service commands
		{
			Name:    "list",
			Aliases: []string{"ls"},
			Usage:   "List all services",
			Action: func(ctx *cli.Context) error {
				return printServices(ctx, windows.SERVICE_TYPE_ALL, windows.SERVICE_STATE_ALL)
			},
		},
	},
}

type svcInfo struct {
	name, display  string
	svcType, state uint32
}

func printServices(_ *cli.Context, serviceType, serviceState uint32) error {
	sis := make([]svcInfo, 0, 256) // preallocate
	// max size of svcInfo strings
	var nameLen, dispLen int
	nbytes := uint32(1024) // 1KiB
	var rh uint32
	for {
		var nSvcs uint32
		b := make([]byte, nbytes)
		b0 := &b[0]
		err := windows.EnumServicesStatusEx(
			_scm.handle(),
			windows.SC_ENUM_PROCESS_INFO,
			serviceType,
			serviceState,
			b0,
			uint32(len(b)),
			&nbytes,
			&nSvcs,
			&rh,
			nil, // groupName
		)
		if err != nil && !errors.Is(err, windows.ERROR_MORE_DATA) {
			return fmt.Errorf("could not enumerate services : %w", err)
		}

		svcs := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(b0)), nSvcs)
		for _, svc := range svcs {
			si := svcInfo{
				name:    windows.UTF16PtrToString(svc.ServiceName),
				display: windows.UTF16PtrToString(svc.DisplayName),
				svcType: svc.ServiceStatusProcess.ServiceType,
				state:   svc.ServiceStatusProcess.CurrentState,
			}
			// get the maximum width of the service and display names
			if len(si.name) > nameLen {
				nameLen = len(si.name)
			}
			if len(si.display) > dispLen {
				dispLen = len(si.display)
			}
			sis = append(sis, si)
		}

		if err == nil {
			break
		}
	}
	for _, si := range sis {
		fmt.Printf("%-[1]*[2]s %-[3]*[4]s %-10s %s\n",
			dispLen,
			si.display,
			nameLen,
			si.name,
			svcState(si.state),
			svcType(si.svcType),
		)
	}

	return nil
}

func svcType(t uint32) string {
	types := map[uint32]string{
		windows.SERVICE_KERNEL_DRIVER:       "Kernel Driver",
		windows.SERVICE_FILE_SYSTEM_DRIVER:  "FS Driver",
		windows.SERVICE_ADAPTER:             "Adapter",
		windows.SERVICE_RECOGNIZER_DRIVER:   "Recognizer Driver",
		windows.SERVICE_WIN32_OWN_PROCESS:   "Process",
		windows.SERVICE_WIN32_SHARE_PROCESS: "Share Process",
		windows.SERVICE_INTERACTIVE_PROCESS: "Interactive Process",
	}

	if s, ok := types[t]; ok {
		return s
	}
	return fmt.Sprintf("Unknown Type [0x%x]", t)
}

func svcState(st uint32) string {
	states := map[uint32]string{
		windows.SERVICE_STOPPED:          "Stopped",
		windows.SERVICE_START_PENDING:    "Starting",
		windows.SERVICE_STOP_PENDING:     "Stopping",
		windows.SERVICE_RUNNING:          "Running",
		windows.SERVICE_CONTINUE_PENDING: "Continuing",
		windows.SERVICE_PAUSE_PENDING:    "Pausing",
		windows.SERVICE_PAUSED:           "Paused",
	}

	if s, ok := states[st]; ok {
		return s
	}
	return fmt.Sprintf("Unknown [0x%x]", st)
}
