package hcs

import (
	"context"
	"strings"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/go-winio/pkg/process"
	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/vm"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

// checkProcess checks if the process identified by the given pid has a name
// matching `desiredProcessName`, and is running as a user with domain
// `desiredDomain` and user name `desiredUser`. If the process matches, it
// returns a handle to the process. If the process does not match, it returns
// 0.
func checkProcess(ctx context.Context, pid uint32, desiredProcessName string, desiredDomain string, desiredUser string) (p windows.Handle, err error) {
	desiredProcessName = strings.ToUpper(desiredProcessName)
	desiredDomain = strings.ToUpper(desiredDomain)
	desiredUser = strings.ToUpper(desiredUser)

	p, err = windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, pid)
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
		log.G(ctx).WithFields(logrus.Fields{
			"name":   name,
			"domain": domain,
			"user":   user,
		}).Debug("checking vmmem process identity")
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
		return 0, errors.Wrap(err, "failed to enumerate processes")
	}
	for _, pid := range pids {
		p, err := checkProcess(ctx, pid, "vmmem", "NT VIRTUAL MACHINE", vmIDStr)
		if err != nil {
			// Checking the process could fail for a variety of reasons, such as
			// the process exiting since we called EnumProcesses, or not having
			// access to open the process (even as SYSTEM). In the case of an
			// error, we just log and continue looking at the other processes.
			log.G(ctx).WithField("pid", pid).Debug("failed to check process")
			continue
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
func (uvm *utilityVM) getVMMEMProcess(ctx context.Context) (windows.Handle, error) {
	uvm.vmmemOnce.Do(func() {
		uvm.vmmemProcess, uvm.vmmemErr = lookupVMMEM(ctx, uvm.vmID)
	})
	return uvm.vmmemProcess, uvm.vmmemErr
}

func (uvm *utilityVM) Stats(ctx context.Context) (*stats.VirtualMachineStatistics, error) {
	s := &stats.VirtualMachineStatistics{}
	props, err := uvm.cs.PropertiesV2(ctx, hcsschema.PTStatistics, hcsschema.PTMemory)
	if err != nil {
		return nil, err
	}

	s.Processor = &stats.VirtualMachineProcessorStatistics{}
	s.Processor.TotalRuntimeNS = uint64(props.Statistics.Processor.TotalRuntime100ns * 100)
	s.Memory = &stats.VirtualMachineMemoryStatistics{}

	if uvm.backingType == vm.MemoryBackingTypePhysical {
		// If the uvm is physically backed we set the working set to the total amount allocated
		// to the UVM. AssignedMemory returns the number of 4KB pages. Will always be 4KB
		// regardless of what the UVMs actual page size is so we don't need that information.
		if props.Memory != nil {
			s.Memory.WorkingSetBytes = props.Memory.VirtualMachineMemory.AssignedMemory * 4096
		}
	} else {
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
		s.Memory.WorkingSetBytes = uint64(memCounters.WorkingSetSize)
	}

	if props.Memory != nil {
		s.Memory.VirtualNodeCount = props.Memory.VirtualNodeCount
		s.Memory.VmMemory = &stats.VirtualMachineMemory{}
		s.Memory.VmMemory.AvailableMemory = props.Memory.VirtualMachineMemory.AvailableMemory
		s.Memory.VmMemory.AvailableMemoryBuffer = props.Memory.VirtualMachineMemory.AvailableMemoryBuffer
		s.Memory.VmMemory.ReservedMemory = props.Memory.VirtualMachineMemory.ReservedMemory
		s.Memory.VmMemory.AssignedMemory = props.Memory.VirtualMachineMemory.AssignedMemory
		s.Memory.VmMemory.SlpActive = props.Memory.VirtualMachineMemory.SlpActive
		s.Memory.VmMemory.BalancingEnabled = props.Memory.VirtualMachineMemory.BalancingEnabled
		s.Memory.VmMemory.DmOperationInProgress = props.Memory.VirtualMachineMemory.DmOperationInProgress
	}

	return s, nil
}
