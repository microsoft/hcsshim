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

import (
	"encoding/json"
)

// Structure used to perform property query
type ServicePropertyQuery struct {
	// Specifies the property array to query
	PropertyTypes []GetPropertyType `json:"PropertyTypes,omitempty"`
	// Perform filtered property queries
	FilteredQueries []FilteredPropertyQuery    `json:"FilteredQueries,omitempty"`
	PropertyQueries map[string]json.RawMessage `json:"PropertyQueries,omitempty"`
}
