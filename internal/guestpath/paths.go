package guestpath

const (
	// LCOWRootPrefixInUVM is the path inside UVM where LCOW container's root
	// file system will be mounted
	LCOWRootPrefixInUVM = "/run/gcs/c"
	// WCOWRootPrefixInUVM is the path inside UVM where WCOW container's root
	// file system will be mounted
	WCOWRootPrefixInUVM = `C:\c`
	// SandboxMountPrefix is mount prefix used in container spec to mark a
	// sandbox-mount
	SandboxMountPrefix = "sandbox://"
	// HugePagesMountPrefix is mount prefix used in container spec to mark a
	// huge-pages mount
	HugePagesMountPrefix = "hugepages://"
	// LCOWMountPathPrefixFmt is the path format in the LCOW UVM where
	// non-global mounts, such as Plan9 mounts are added
	LCOWMountPathPrefixFmt = "/mounts/m%d"
	// LCOWGlobalMountPrefixFmt is the path format in the LCOW UVM where global
	// mounts are added
	LCOWGlobalMountPrefixFmt = "/run/mounts/m%d"
	// LCOWGlobalDriverPrefixFmt is the path format in the LCOW UVM where drivers
	// are mounted as read/write
	LCOWGlobalDriverPrefixFmt = "/run/drivers/%s"
	// WCOWGlobalMountPrefixFmt is the path prefix format in the WCOW UVM where
	// mounts are added
	WCOWGlobalMountPrefixFmt = "C:\\mounts\\m%d"
	// RootfsPath is part of the container's rootfs path
	RootfsPath = "rootfs"
)
