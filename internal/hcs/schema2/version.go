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

// Object that describes a version with a Major.Minor format.
type Version struct {
	// The major version value. Individual major versions are not compatible with one another.
	Major uint32 `json:"Major,omitempty"`
	// The minor version value. A lower minor version is considered a compatible subset of a higher minor version.
	Minor uint32 `json:"Minor,omitempty"`
}
