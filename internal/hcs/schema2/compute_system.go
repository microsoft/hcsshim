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

// Describes the configuration of a compute system to create with all of the necessary resources it requires for a successful boot.
type ComputeSystem struct {
	// A string identifying the owning client for this system.
	Owner         string   `json:"Owner,omitempty"`
	SchemaVersion *Version `json:"SchemaVersion,omitempty"`
	// The identifier of the compute system that will host the system described by HostedSystem. The hosting system must already have been created.
	HostingSystemID string `json:"HostingSystemId,omitempty"`
	// The JSON describing the compute system that will be launched inside of the system identified by HostingSystemId. This property is mutually exclusive with the Container and VirtualMachine properties.
	HostedSystem   *json.RawMessage `json:"HostedSystem,omitempty"`
	Container      *Container       `json:"Container,omitempty"`
	VirtualMachine *VirtualMachine  `json:"VirtualMachine,omitempty"`
	// If true, this system will be forcibly terminated when the last HCS_SYSTEM handle corresponding to it is closed.
	ShouldTerminateOnLastHandleClosed bool   `json:"ShouldTerminateOnLastHandleClosed,omitempty"`
	JobName                           string `json:"JobName,omitempty"`
}
