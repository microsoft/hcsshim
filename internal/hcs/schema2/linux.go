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

type Linux struct {
	UidMappings       []LinuxIDMapping  `json:"uidMappings,omitempty"`
	GidMappings       []LinuxIDMapping  `json:"gidMappings,omitempty"`
	Sysctl            map[string]string `json:"sysctl,omitempty"`
	Resources         *LinuxResources   `json:"resources,omitempty"`
	CgroupsPath       string            `json:"cgroupsPath,omitempty"`
	Namespaces        []LinuxNamespace  `json:"namespaces,omitempty"`
	Devices           []LinuxDevice     `json:"devices,omitempty"`
	Seccomp           *LinuxSeccomp     `json:"seccomp,omitempty"`
	RootfsPropagation string            `json:"rootfsPropagation,omitempty"`
	MaskedPaths       []string          `json:"maskedPaths,omitempty"`
	ReadonlyPaths     []string          `json:"readonlyPaths,omitempty"`
	MountLabel        string            `json:"mountLabel,omitempty"`
	IntelRdt          *LinuxIntelRdt    `json:"intelRdt,omitempty"`
}
