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

// AMD SEV-SNP (secure nested paging) information required for attestation.
type SecureNestedPagingInfo struct {
	// List of IDs per socket.
	CpuID []SecureNestedPagingCpuID `json:"CpuId,omitempty"`
	// SNP TCB (trusted computing base) version.
	TcbVersion uint64 `json:"TcbVersion,omitempty"`
}
