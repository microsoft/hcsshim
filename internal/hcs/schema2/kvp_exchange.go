// Autogenerated code; DO NOT EDIT.

/*
 * Schema Open API
 *
 * No description provided (generated by Swagger Codegen https://github.com/swagger-api/swagger-codegen)
 *
 * API version: 2.4
 * Generated by: Swagger Codegen (https://github.com/swagger-api/swagger-codegen.git)
 */

package hcsschema

type KvpExchange struct {
	EnableHostOSInfoKvpItems *StateOverride    `json:"EnableHostOSInfoKvpItems,omitempty"`
	EntriesToBeAdded         map[string]string `json:"EntriesToBeAdded,omitempty"`
	EntriesToBeRemoved       []string          `json:"EntriesToBeRemoved,omitempty"`
	// Represents the IPAddress settings of network adapters within the guest operating system
	GuestNetworkAdapterSettings []GuestNetworkAdapterSetting `json:"GuestNetworkAdapterSettings,omitempty"`
}
