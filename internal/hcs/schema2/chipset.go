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

type Chipset struct {
	Uefi                     *Uefi                  `json:"Uefi,omitempty"`
	Pcat                     *Pcat                  `json:"Pcat,omitempty"`
	IsNumLockDisabled        bool                   `json:"IsNumLockDisabled,omitempty"`
	BaseBoardSerialNumber    string                 `json:"BaseBoardSerialNumber,omitempty"`
	ChassisSerialNumber      string                 `json:"ChassisSerialNumber,omitempty"`
	ChassisAssetTag          string                 `json:"ChassisAssetTag,omitempty"`
	EnableHibernation        bool                   `json:"EnableHibernation,omitempty"`
	UseUTC                   bool                   `json:"UseUtc,omitempty"`
	LinuxKernelDirect        *LinuxKernelDirect     `json:"LinuxKernelDirect,omitempty"`
	SystemInformation        *UefiSystemInformation `json:"SystemInformation,omitempty"`
	MemoryDeviceSerialNumber string                 `json:"MemoryDeviceSerialNumber,omitempty"`
}
