// Autogenerated code; DO NOT EDIT.

// Schema retrieved from branch 'fe_release' and build '20348.1.210507-1500'.

/*
 * Schema Open API
 *
 * No description provided (generated by Swagger Codegen https://github.com/swagger-api/swagger-codegen)
 *
 * API version: 2.4
 * Generated by: Swagger Codegen (https://github.com/swagger-api/swagger-codegen.git)
 */

package hcsschema

// Represents the flush state of the registry hive for a Windows container's job object.
type RegistryFlushState struct {
	// Determines whether the flush state of the registry hive is enabled or not. When not enabled, flushes are ignored and changes to the registry are not preserved.
	Enabled bool `json:"Enabled,omitempty"`
}
