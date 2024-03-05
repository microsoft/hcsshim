// This package contains annotations that are not exposed to end users and are either:
//  1. intended for testing and debugging purposes; or
//  2. rely on undocumented Windows APIs that are subject to change.
//
// Do not rely on these annotations to customize production workload behavior.
package annotations

// uVM specific annotations

const (
	// UVMHyperVSocketConfigPrefix is the prefix of an annotation to map a [hyper-v socket] service GUID
	// to a JSON-encoded string of its [configuration].
	//
	// The service GUID should be part of the annotation.
	// For example:
	//
	// 	"io.microsoft.virtualmachine.hv-socket.service-table.00000000-0000-0000-0000-000000000000" =
	// 		"{\"AllowWildcardBinds\": true, \"BindSecurityDescriptor\": \"D:P(A;;FA;;;WD)\"}"
	//
	// If multiple annotations with the same GUID are present, then it is undefined which configuration will
	// take precedence.
	//
	// For LCOW, it is preferred to use [ExtraVSockPorts], as vsock ports specified there will take precedence.
	//
	// # Warning
	//
	// Setting the configuration for special services (e.g., the GCS) can cause catastrophic failures.
	//
	// [hyper-v socket]: https://learn.microsoft.com/en-us/virtualization/hyper-v-on-windows/user-guide/make-integration-service
	// [configuration]: https://learn.microsoft.com/en-us/virtualization/api/hcs/schemareference#HvSocketServiceConfig
	UVMHyperVSocketConfigPrefix = "io.microsoft.virtualmachine.hv-socket.service-table."

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
