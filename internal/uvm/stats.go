package uvm

import (
	"context"
	"errors"
	"strings"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/go-winio/pkg/process"
	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/schema1"
	"golang.org/x/sys/windows"
)

// checkProcess checks if the process identified by the given pid has a name
// matching `desiredProcessName`, and is running as a user with domain
// `desiredDomain` and user name `desiredUser`. If the process matches, it
// returns a handle to the process. If the process does not match, it returns
// 0.
func checkProcess(pid uint32, desiredProcessName string, desiredDomain string, desiredUser string) (p windows.Handle, err error) {
	desiredProcessName = strings.ToUpper(desiredProcessName)
	desiredDomain = strings.ToUpper(desiredDomain)
	desiredUser = strings.ToUpper(desiredUser)

	p, err = windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, pid)
	// It is expected that there will be processes we can't open, even when
	// running as SYSTEM. So just skip the process if we get access denied.
	if err == windows.ERROR_ACCESS_DENIED {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer func(openedProcess windows.Handle) {
		// If we don't return this process handle, close it so it doesn't leak.
		if p == 0 {
			windows.Close(openedProcess)
		}
	}(p)
	// Querying vmmem's image name as a win32 path returns ERROR_GEN_FAILURE
	// for some reason, so we query it as an NT path instead.
	name, err := process.QueryFullProcessImageName(p, process.ImageNameFormatNTPath)
	if err != nil {
		return 0, err
	}
	if strings.ToUpper(name) == desiredProcessName {
		var t windows.Token
		if err := windows.OpenProcessToken(p, windows.TOKEN_QUERY, &t); err != nil {
			return 0, err
		}
		defer t.Close()
		tUser, err := t.GetTokenUser()
		if err != nil {
			return 0, err
		}
		user, domain, _, err := tUser.User.Sid.LookupAccount("")
		if err != nil {
			return 0, err
		}
		if strings.ToUpper(domain) == desiredDomain && strings.ToUpper(user) == desiredUser {
			return p, nil
		}
	}
	return 0, nil
}

// lookupVMMEM locates the vmmem process for a VM given the VM ID. It returns
// a handle to the vmmem process. The lookup is implemented by enumerating all
// processes on the system, and finding a process with full name "vmmem",
// running as "NT VIRTUAL MACHINE\<VM ID>".
func lookupVMMEM(ctx context.Context, vmID guid.GUID) (proc windows.Handle, err error) {
	vmIDStr := strings.ToUpper(vmID.String())
	log.G(ctx).WithField("vmID", vmIDStr).Debug("looking up vmmem")

	pids, err := process.EnumProcesses()
	if err != nil {
		return 0, err
	}
	for _, pid := range pids {
		if pid == 0 {
			// Opening System process never works and fails with a different
			// error (ERROR_INVALID_PARAMETER) than the usual
			// ERROR_ACCESS_DENIED, so just skip it.
			continue
		}
		p, err := checkProcess(pid, "vmmem", "NT VIRTUAL MACHINE", vmIDStr)
		if err != nil {
			return 0, err
		}
		if p != 0 {
			log.G(ctx).WithField("pid", pid).Debug("found vmmem match")
			return p, nil
		}
	}
	return 0, errors.New("failed to find matching vmmem process")
}

// getVMMEMProcess returns a handle to the vmmem process associated with this
// UVM. It only does the actual process lookup once, after which it caches the
// process handle in the UVM object.
func (uvm *UtilityVM) getVMMEMProcess(ctx context.Context) (_ windows.Handle, err error) {
	uvm.vmmemOnce.Do(func() {
		uvm.vmmemProcess, err = lookupVMMEM(ctx, uvm.runtimeID)
	})
	return uvm.vmmemProcess, err
}

// Stats returns various UVM statistics.
func (uvm *UtilityVM) Stats(ctx context.Context) (*stats.VirtualMachineStatistics, error) {
	s := &stats.VirtualMachineStatistics{}
	statsV1, err := uvm.hcsSystem.Properties(ctx, schema1.PropertyTypeStatistics)
	if err != nil {
		return nil, err
	}
	s.Processor = &stats.VirtualMachineProcessorStatistics{}
	s.Processor.TotalRuntimeNS = statsV1.Statistics.Processor.TotalRuntime100ns * 100

	// The HCS properties does not return sufficient information to calculate
	// working set size for a VA-backed UVM. To work around this, we instead
	// locate the vmmem process for the VM, and query that process's working set
	// instead, which will be the working set for the VM.
	vmmemProc, err := uvm.getVMMEMProcess(ctx)
	if err != nil {
		return nil, err
	}
	memCounters, err := process.GetProcessMemoryInfo(vmmemProc)
	if err != nil {
		return nil, err
	}
	s.Memory = &stats.VirtualMachineMemoryStatistics{
		WorkingSetBytes: uint64(memCounters.WorkingSetSize),
	}

	return s, nil
}
