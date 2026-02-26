//go:build windows

package vmutils

import (
	"fmt"

	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/Microsoft/hcsshim/osversion"
)

const (
	// MaxVPMEMCount is the maximum number of VPMem devices that may be added to an LCOW
	// utility VM.
	MaxVPMEMCount = 128

	// DefaultVPMEMCount is the default number of VPMem devices that may be added to an LCOW
	// utility VM if the create request doesn't specify how many.
	DefaultVPMEMCount = 64

	// DefaultVPMemSizeBytes is the default size of a VPMem device if the create request
	// doesn't specify.
	DefaultVPMemSizeBytes = 4 * memory.GiB // 4GB

	// DefaultDmVerityRootfsVhd is the default file name for a dm-verity rootfs VHD,
	// mounted by the GuestStateFile during boot and used as the root file system when
	// booting in the SNP case. Similar to layer VHDs, the Merkle tree is appended after
	// the ext4 filesystem ends.
	DefaultDmVerityRootfsVhd = "rootfs.vhd"
	// DefaultGuestStateFile is the default file name for a VMGS (VM Guest State) file,
	// which contains the kernel and kernel command that mounts DmVerityVhdFile when
	// booting in the SNP case.
	DefaultGuestStateFile = "kernel.vmgs"
	// DefaultUVMReferenceInfoFile is the default file name for a COSE_Sign1 reference
	// UVM info file, which can be made available to workload containers and used for
	// validation purposes.
	DefaultUVMReferenceInfoFile = "reference_info.cose"

	// InitrdFile is the default file name for an initrd.img used to boot LCOW.
	InitrdFile = "initrd.img"
	// VhdFile is the default file name for a rootfs.vhd used to boot LCOW.
	VhdFile = "rootfs.vhd"
	// KernelFile is the default file name for a kernel used to boot LCOW.
	KernelFile = "kernel"
	// UncompressedKernelFile is the default file name for an uncompressed
	// kernel used to boot LCOW with KernelDirect.
	UncompressedKernelFile = "vmlinux"

	// LinuxEntropyVsockPort is the vsock port used to inject initial entropy
	// into the LCOW guest VM during boot.
	LinuxEntropyVsockPort = 1
	// LinuxLogVsockPort is the vsock port used by the GCS (Guest Compute Service)
	// to forward stdout/stderr log data from the guest to the host.
	LinuxLogVsockPort = 109
	// LinuxEntropyBytes is the number of bytes of random data to send to a Linux UVM
	// during boot to seed the CRNG. There is not much point in making this too
	// large since the random data collected from the host is likely computed from a
	// relatively small key (256 bits?), so additional bytes would not actually
	// increase the entropy of the guest's pool. However, send enough to convince
	// containers that there is a large amount of entropy since this idea is
	// generally misunderstood.
	LinuxEntropyBytes = 512
)

var (
	// ErrCPUGroupCreateNotSupported is returned when a create request specifies a CPU group but the host build doesn't support it.
	ErrCPUGroupCreateNotSupported = fmt.Errorf("cpu group assignment on create requires a build of %d or higher", osversion.V21H1)
)
