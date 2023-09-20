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

type LinuxResources struct {
	Devices        []LinuxDeviceCgroup  `json:"devices,omitempty"`
	Memory         *LinuxMemory         `json:"memory,omitempty"`
	Cpu            *LinuxCpu            `json:"cpu,omitempty"`
	Pids           *LinuxPids           `json:"pids,omitempty"`
	BlockIO        *LinuxBlockIO        `json:"blockIO,omitempty"`
	HugepageLimits []LinuxHugepageLimit `json:"hugepageLimits,omitempty"`
	Network        *LinuxNetwork        `json:"network,omitempty"`
	Rdma           map[string]LinuxRdma `json:"rdma,omitempty"`
}
