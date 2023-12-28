// This package contains annotations that are not exposed to end users and mainly for
// testing and debugging purposes.
//
// Do not rely on these annotations to customize production workload behavior.
package annotations

// uVM specific annotations

const (
	// AdditionalRegistryValues specifies additional registry keys and their values to set in the WCOW UVM.
	// The format is a JSON-encoded string of an array containing [HCS RegistryValue] objects.
	//
	// Registry values will be available under `HKEY_LOCAL_MACHINE` root key.
	//
	// For example:
	//
	//	"[{\"Key\": {\"Hive\": \"System\", \"Name\": \"registry\\key\\path"}, \"Name\": \"ValueName\", \"Type\": \"String\", \"StringValue\": \"value\"}]"
	//
	// [HCS RegistryValue]: https://learn.microsoft.com/en-us/virtualization/api/hcs/schemareference#registryvalue
	AdditionalRegistryValues = "io.microsoft.virtualmachine.wcow.additional-reg-keys"

	// ExtraVSockPorts adds additional ports to the list of ports that the UVM is allowed to use.
	ExtraVSockPorts = "io.microsoft.virtualmachine.lcow.extra-vsock-ports"
)
