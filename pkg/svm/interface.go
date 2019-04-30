package svm

// Instance is an instance of a (set of) managed service VMs.
type Instance interface {

	// Mode returns the mode of this instance
	Mode() Mode

	// Count returns the number of service VMs in this instance
	Count() int

	// Create creates a service VM in this instance. In global mode, a call to
	// create when the service VM is already running is a no-op. In per-instance
	// mode, a second call to create will create a second service.
	Create(id string) error

	// GenerateExt4 generates a formatted EXT4 for use as a scratch disk.
	//
	// If the file already exists in the cache directory (scratch.nn.vhdx), where
	// nn represents the size in GB, it's just a file copy to the target directory,
	// and named as just scratch.vhdx.
	//
	// If the file does not exist in the cache directory, it uses a service VM
	// to create on in the targetDir named scratch.vhdx. Once created, if
	// storeInCache is passed, then the file is copied to the cacheDir with the
	// name scratch.nn.vhdx.
	GenerateExt4(id string, sizeGB uint16, cacheDir string, targetDir string, storeInCache bool) error

	// Discard removes the callers reference to this instance. In global mode,
	// it's a no-op if the service VM is already running. In per-instance mode,
	// it will cause the service VM (if running) to be terminated.
	Discard(id string) error

	// Destroy is similar to Discard, but it does a complete nuke, terminating
	// all service VMs in this instance.
	Destroy() error

	// Mount creates an overlay of a set of read-only layers (no scratch involved)
	// at the requested path in the service VM. Layers are ref-counted so they
	// can be used by multiple callers when in global mode.
	Mount(id string, layers []string, svmPath string) error

	// Unmount performs the reverse of Mount.
	Unmount(id string, svmPath string) error

	// RunProcess is a simple wrapper for running a process in a service VM.
	// It returns the exit code and the combined stdout/stderr.
	RunProcess(id string, args []string, stdin string) (int, string, error)
}
