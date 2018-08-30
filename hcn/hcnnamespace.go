package hcn

import (
	"encoding/json"

	"github.com/Microsoft/hcsshim/internal/guid"
	"github.com/Microsoft/hcsshim/internal/interop"
	"github.com/sirupsen/logrus"
)

// NamespaceResourceEndpoint represents an Endpoint attached to a Namespace.
type NamespaceResourceEndpoint struct {
	Id string `json:"ID,"`
}

// NamespaceResourceContainer represents a Container attached to a Namespace.
type NamespaceResourceContainer struct {
	Id string `json:"ID,"`
}

// NamespaceResource is associated with a namespace
type NamespaceResource struct {
	Type string          `json:","` // Container, Endpoint
	Data json.RawMessage `json:","`
}

// HostComputeNamespace represents a namespace (AKA compartment) in
type HostComputeNamespace struct {
	Id            string              `json:"ID,omitempty"`
	NamespaceId   uint32              `json:",omitempty"`
	Type          string              `json:",omitempty"` // Host, HostDefault, Guest, GuestDefault
	Resources     []NamespaceResource `json:",omitempty"`
	SchemaVersion SchemaVersion       `json:",omitempty"`
}

// ModifyNamespaceSettingRequest is the structure used to send request to modify a namespace.
// Used to Add/Remove an endpoints and containers to/from a namespace.
type ModifyNamespaceSettingRequest struct {
	ResourceType string          `json:",omitempty"` // Container, Endpoint
	RequestType  string          `json:",omitempty"` // Add, Remove, Update, Refresh
	Settings     json.RawMessage `json:",omitempty"`
}

func getNamespace(namespaceGuid guid.GUID, query string) (*HostComputeNamespace, error) {
	if err := V2ApiSupported(); err != nil {
		return nil, err
	}
	// Open namespace.
	var (
		namespaceHandle  hcnNamespace
		resultBuffer     *uint16
		propertiesBuffer *uint16
	)
	hr := hcnOpenNamespace(&namespaceGuid, &namespaceHandle, &resultBuffer)
	if err := CheckForErrors("hcnOpenNamespace", hr, resultBuffer); err != nil {
		return nil, err
	}
	// Query namespace.
	hr = hcnQueryNamespaceProperties(namespaceHandle, query, &propertiesBuffer, &resultBuffer)
	if err := CheckForErrors("hcnQueryNamespaceProperties", hr, resultBuffer); err != nil {
		return nil, err
	}
	properties := interop.ConvertAndFreeCoTaskMemString(propertiesBuffer)
	// Close namespace.
	hr = hcnCloseNamespace(namespaceHandle)
	if err := CheckForErrors("hcnCloseNamespace", hr, nil); err != nil {
		return nil, err
	}
	// Convert output to HostComputeNamespace
	var outputNamespace HostComputeNamespace
	if err := json.Unmarshal([]byte(properties), &outputNamespace); err != nil {
		return nil, err
	}
	return &outputNamespace, nil
}

func enumerateNamespaces(query string) ([]HostComputeNamespace, error) {
	if err := V2ApiSupported(); err != nil {
		return nil, err
	}
	// Enumerate all Namespace Guids
	var (
		resultBuffer    *uint16
		namespaceBuffer *uint16
	)
	hr := hcnEnumerateNamespaces(query, &namespaceBuffer, &resultBuffer)
	if err := CheckForErrors("hcnEnumerateNamespaces", hr, resultBuffer); err != nil {
		return nil, err
	}

	namespaces := interop.ConvertAndFreeCoTaskMemString(namespaceBuffer)
	var namespaceIds []guid.GUID
	if err := json.Unmarshal([]byte(namespaces), &namespaceIds); err != nil {
		return nil, err
	}

	var outputNamespaces []HostComputeNamespace
	for _, namespaceGuid := range namespaceIds {
		namespace, err := getNamespace(namespaceGuid, query)
		if err != nil {
			return nil, err
		}
		outputNamespaces = append(outputNamespaces, *namespace)
	}
	return outputNamespaces, nil
}

func createNamespace(settings string) (*HostComputeNamespace, error) {
	if err := V2ApiSupported(); err != nil {
		return nil, err
	}
	// Create new namespace.
	var (
		namespaceHandle  hcnNamespace
		resultBuffer     *uint16
		propertiesBuffer *uint16
	)
	namespaceGuid := guid.GUID{}
	hr := hcnCreateNamespace(&namespaceGuid, settings, &namespaceHandle, &resultBuffer)
	if err := CheckForErrors("hcnCreateNamespace", hr, resultBuffer); err != nil {
		return nil, err
	}
	// Query namespace.
	hcnQuery := QuerySchema(2)
	query, err := json.Marshal(hcnQuery)
	if err != nil {
		return nil, err
	}
	hr = hcnQueryNamespaceProperties(namespaceHandle, string(query), &propertiesBuffer, &resultBuffer)
	if err := CheckForErrors("hcnQueryNamespaceProperties", hr, resultBuffer); err != nil {
		return nil, err
	}
	properties := interop.ConvertAndFreeCoTaskMemString(propertiesBuffer)
	// Close namespace.
	hr = hcnCloseNamespace(namespaceHandle)
	if err := CheckForErrors("hcnCloseNamespace", hr, nil); err != nil {
		return nil, err
	}
	// Convert output to HostComputeNamespace
	var outputNamespace HostComputeNamespace
	if err := json.Unmarshal([]byte(properties), &outputNamespace); err != nil {
		return nil, err
	}
	return &outputNamespace, nil
}

func modifyNamespace(namespaceId string, settings string) (*HostComputeNamespace, error) {
	if err := V2ApiSupported(); err != nil {
		return nil, err
	}
	namespaceGuid := guid.FromString(namespaceId)
	// Open namespace.
	var (
		namespaceHandle  hcnNamespace
		resultBuffer     *uint16
		propertiesBuffer *uint16
	)
	hr := hcnOpenNamespace(&namespaceGuid, &namespaceHandle, &resultBuffer)
	if err := CheckForErrors("hcnOpenNamespace", hr, resultBuffer); err != nil {
		return nil, err
	}
	// Modify namespace.
	hr = hcnModifyNamespace(namespaceHandle, settings, &resultBuffer)
	if err := CheckForErrors("hcnModifyNamespace", hr, resultBuffer); err != nil {
		return nil, err
	}
	// Query namespace.
	hcnQuery := QuerySchema(2)
	query, err := json.Marshal(hcnQuery)
	if err != nil {
		return nil, err
	}
	hr = hcnQueryNamespaceProperties(namespaceHandle, string(query), &propertiesBuffer, &resultBuffer)
	if err := CheckForErrors("hcnQueryNamespaceProperties", hr, resultBuffer); err != nil {
		return nil, err
	}
	properties := interop.ConvertAndFreeCoTaskMemString(propertiesBuffer)
	// Close namespace.
	hr = hcnCloseNamespace(namespaceHandle)
	if err := CheckForErrors("hcnCloseNamespace", hr, nil); err != nil {
		return nil, err
	}
	// Convert output to Namespace
	var outputNamespace HostComputeNamespace
	if err := json.Unmarshal([]byte(properties), &outputNamespace); err != nil {
		return nil, err
	}
	return &outputNamespace, nil
}

func deleteNamespace(namespaceId string) error {
	if err := V2ApiSupported(); err != nil {
		return err
	}
	namespaceGuid := guid.FromString(namespaceId)
	var resultBuffer *uint16
	hr := hcnDeleteNamespace(&namespaceGuid, &resultBuffer)
	if err := CheckForErrors("hcnDeleteNamespace", hr, resultBuffer); err != nil {
		return err
	}
	return nil
}

// ListNamespaces makes a call to list all available namespaces.
func ListNamespaces() ([]HostComputeNamespace, error) {
	hcnQuery := QuerySchema(2)
	namespaces, err := ListNamespacesQuery(hcnQuery)
	if err != nil {
		return nil, err
	}
	return namespaces, nil
}

// ListNamespacesQuery makes a call to query the list of available namespaces.
func ListNamespacesQuery(query HostComputeQuery) ([]HostComputeNamespace, error) {
	queryJson, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}

	namespaces, err := enumerateNamespaces(string(queryJson))
	if err != nil {
		return nil, err
	}
	return namespaces, nil
}

// GetNamespaceByID returns the Namespace specified by Id.
func GetNamespaceByID(namespaceId string) (*HostComputeNamespace, error) {
	hcnQuery := QuerySchema(2)
	mapA := map[string]string{"ID": namespaceId}
	filter, err := json.Marshal(mapA)
	if err != nil {
		return nil, err
	}
	hcnQuery.Filter = string(filter)

	namespaces, err := ListNamespacesQuery(hcnQuery)
	if err != nil {
		return nil, err
	}
	if len(namespaces) == 0 {
		return nil, nil
	}
	return &namespaces[0], err
}

// GetNamespaceEndpointIds returns the endpoints of the Namespace specified by Id.
func GetNamespaceEndpointIds(namespaceId string) ([]string, error) {
	namespace, err := GetNamespaceByID(namespaceId)
	if err != nil {
		return nil, err
	}
	var endpointsIds []string
	for _, resource := range namespace.Resources {
		if resource.Type == "Endpoint" {
			var endpointResource NamespaceResourceEndpoint
			if err := json.Unmarshal([]byte(resource.Data), &endpointResource); err != nil {
				return nil, err
			}
			endpointsIds = append(endpointsIds, endpointResource.Id)
		}
	}
	return endpointsIds, nil
}

// GetNamespaceContainerIds returns the containers of the Namespace specified by Id.
func GetNamespaceContainerIds(namespaceId string) ([]string, error) {
	namespace, err := GetNamespaceByID(namespaceId)
	if err != nil {
		return nil, err
	}
	var containerIds []string
	for _, resource := range namespace.Resources {
		if resource.Type == "Container" {
			var contaienrResource NamespaceResourceContainer
			if err := json.Unmarshal([]byte(resource.Data), &contaienrResource); err != nil {
				return nil, err
			}
			containerIds = append(containerIds, contaienrResource.Id)
		}
	}
	return containerIds, nil
}

// Create Namespace.
func (namespace *HostComputeNamespace) Create() (*HostComputeNamespace, error) {
	logrus.Debugf("hcn::HostComputeNamespace::Create id=%s", namespace.Id)

	jsonString, err := json.Marshal(namespace)
	if err != nil {
		return nil, err
	}

	namespace, hcnErr := createNamespace(string(jsonString))
	if hcnErr != nil {
		return nil, hcnErr
	}
	return namespace, nil
}

// Delete Namespace.
func (namespace *HostComputeNamespace) Delete() (*HostComputeNamespace, error) {
	logrus.Debugf("hcn::HostComputeNamespace::Delete id=%s", namespace.Id)

	if err := deleteNamespace(namespace.Id); err != nil {
		return nil, err
	}
	return nil, nil
}

// ModifyNamespaceSettings updates the Endpoints/Containers of a Namespace.
func ModifyNamespaceSettings(namespaceId string, request *ModifyNamespaceSettingRequest) error {
	logrus.Debugf("hcn::HostComputeNamespace::ModifyNamespaceSettings id=%s", namespaceId)

	namespaceSettings, err := json.Marshal(request)
	if err != nil {
		return err
	}

	_, err = modifyNamespace(namespaceId, string(namespaceSettings))
	if err != nil {
		return err
	}
	return nil
}

// AddNamespaceEndpoint adds an endpoint to a Namespace.
func AddNamespaceEndpoint(namespaceId string, endpointId string) error {
	logrus.Debugf("hcn::HostComputeEndpoint::AddNamespaceEndpoint id=%s", endpointId)

	mapA := map[string]string{"EndpointId": endpointId}
	settingsJson, err := json.Marshal(mapA)
	if err != nil {
		return err
	}
	requestMessage := &ModifyNamespaceSettingRequest{
		ResourceType: "Endpoint",
		RequestType:  "Add",
		Settings:     settingsJson,
	}

	return ModifyNamespaceSettings(namespaceId, requestMessage)
}

// RemoveNamespaceEndpoint removes an endpoint from a Namespace.
func RemoveNamespaceEndpoint(namespaceId string, endpointId string) error {
	logrus.Debugf("hcn::HostComputeNamespace::RemoveNamespaceEndpoint id=%s", endpointId)

	mapA := map[string]string{"EndpointId": endpointId}
	settingsJson, err := json.Marshal(mapA)
	if err != nil {
		return err
	}
	requestMessage := &ModifyNamespaceSettingRequest{
		ResourceType: "Endpoint",
		RequestType:  "Remove",
		Settings:     settingsJson,
	}

	return ModifyNamespaceSettings(namespaceId, requestMessage)
}
