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

// Set properties operation settings
type SetPropertyOperation struct {
	// Pattern: /^[0-9A-Fa-f]{8}-([0-9A-Fa-f]{4}-){3}[0-9A-Fa-f]{12}$/
	GroupID       string `json:"GroupId,omitempty"`
	PropertyCode  uint32 `json:"PropertyCode,omitempty"`
	PropertyValue uint64 `json:"PropertyValue,omitempty"`
}
