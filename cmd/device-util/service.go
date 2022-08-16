//go:build windows

package main

import (
	"errors"
	"fmt"
	"unsafe"

	cli "github.com/urfave/cli/v2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

var serviceCommand = &cli.Command{
	Name:    "service",
	Aliases: []string{"svc"},
	Usage:   "manage Windows services",
	Before:  verifyElevated,
	Subcommands: []*cli.Command{
		{
			Name:    "list",
			Aliases: []string{"ls"},
			Usage:   "list Windows services",
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
	scm, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("could not create service manager: %w", err)
	}

	// scm.ListServices() does not allow specifying service type and state.
	svcInfos := make([]svcInfo, 0, 256) // preallocate
	// max size of svcInfo strings
	var nameLen, dispLen int
	var rh uint32      // resume handle
	sz := uint32(1024) // 1KiB: initial buffer size
	for {
		var nSvcs uint32
		b := make([]byte, sz)
		b0 := &b[0]
		err := windows.EnumServicesStatusEx(
			scm.Handle,
			windows.SC_ENUM_PROCESS_INFO,
			serviceType,
			serviceState,
			b0,
			uint32(len(b)),
			&sz,
			&nSvcs,
			&rh,
			nil, // groupName
		)
		if err != nil && !errors.Is(err, windows.ERROR_MORE_DATA) {
			return fmt.Errorf("could not enumerate services : %w", err)
		}

		// expand svcInfos to hold new objects
		if n := len(svcInfos) + int(nSvcs); cap(svcInfos) <= n {
			si := make([]svcInfo, len(svcInfos), n)
			copy(si, svcInfos)
			svcInfos = si
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
			svcInfos = append(svcInfos, si)
		}

		if err == nil {
			break
		}
	}
	for _, si := range svcInfos {
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
