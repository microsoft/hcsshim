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

type GPUConfiguration struct {
	AssignmentMode *GPUAssignmentMode `json:"AssignmentMode,omitempty"`
	// This only applies to List mode, and is ignored in other modes. In GPU-P, string is GPU device interface, and unit16 is partition id. HCS simply assigns the partition with the input id. In GPU-PV, string is GPU device interface, and unit16 is 0xffff. HCS needs to find an available partition to assign.
	AssignmentRequest map[string]int64 `json:"AssignmentRequest,omitempty"`
	// Whether we allow vendor extension.
	AllowVendorExtension bool `json:"AllowVendorExtension,omitempty"`
}
