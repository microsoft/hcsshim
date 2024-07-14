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

type SiloSettings struct {
	// If running this virtual machine inside a silo, the base OS path to use for the silo.
	SiloBaseOSPath string `json:"SiloBaseOsPath,omitempty"`
	// Request a notification when the job object for the silo is available.
	NotifySiloJobCreated bool `json:"NotifySiloJobCreated,omitempty"`
	// The filesystem layers to use for the silo.
	FileSystemLayers []Layer `json:"FileSystemLayers,omitempty"`
	// The bindings to use for the silo.
	Bindings []BatchedBinding `json:"Bindings,omitempty"`
}
