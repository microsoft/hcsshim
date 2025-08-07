// This package contains annotations that are not exposed to end users and are either:
//  1. intended for testing and debugging purposes; or
//  2. rely on undocumented Windows APIs that are subject to change.
//
// Do not rely on these annotations to customize production workload behavior.
package annotations

// uVM annotations.
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

	// UVMConsolePipe is the name of the named pipe that the UVM console is connected to. This works only for non-SNP
	// scenario, for SNP use a debugger.
	UVMConsolePipe = "io.microsoft.virtualmachine.console.pipe"
)

// LCOW uVM annotations.
const (
	// ExtraVSockPorts adds additional ports to the list of ports that the UVM is allowed to use.
	ExtraVSockPorts = "io.microsoft.virtualmachine.lcow.extra-vsock-ports"

	// NetworkingPolicyBasedRouting toggles on the ability to set policy based routing in the
	// guest for LCOW.
	//
	// TODO(katiewasnothere): The goal of this annotation was to be used as a fallback if the
	// work to support multiple custom network routes per adapter in LCOW breaks existing
	// LCOW scenarios. Ideally, this annotation should be removed if no issues are found.
	NetworkingPolicyBasedRouting = "io.microsoft.virtualmachine.lcow.network.policybasedrouting"
)

// WCOW uVM annotations.
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
)
