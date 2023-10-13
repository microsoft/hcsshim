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

// WorkerExitDetail : Detailed reasons for a VM stop
type WorkerExitDetail string

// List of WorkerExitDetail
const (
	WorkerExitDetail_INVALID                                      WorkerExitDetail = "Invalid"
	WorkerExitDetail_POWER_OFF                                    WorkerExitDetail = "PowerOff"
	WorkerExitDetail_POWER_OFF_CRITICAL                           WorkerExitDetail = "PowerOffCritical"
	WorkerExitDetail_RESET                                        WorkerExitDetail = "Reset"
	WorkerExitDetail_GUEST_CRASH                                  WorkerExitDetail = "GuestCrash"
	WorkerExitDetail_GUEST_FIRMWARE_CRASH                         WorkerExitDetail = "GuestFirmwareCrash"
	WorkerExitDetail_TRIPLE_FAULT                                 WorkerExitDetail = "TripleFault"
	WorkerExitDetail_DEVICE_FATAL_APIC_REQUEST                    WorkerExitDetail = "DeviceFatalApicRequest"
	WorkerExitDetail_DEVICE_FATAL_MSR_REQUEST                     WorkerExitDetail = "DeviceFatalMsrRequest"
	WorkerExitDetail_DEVICE_FATAL_EXCEPTION                       WorkerExitDetail = "DeviceFatalException"
	WorkerExitDetail_DEVICE_FATAL_ERROR                           WorkerExitDetail = "DeviceFatalError"
	WorkerExitDetail_DEVICE_MACHINE_CHECK                         WorkerExitDetail = "DeviceMachineCheck"
	WorkerExitDetail_EMULATOR_ERROR                               WorkerExitDetail = "EmulatorError"
	WorkerExitDetail_VID_TERMINATE                                WorkerExitDetail = "VidTerminate"
	WorkerExitDetail_PROCESS_UNEXPECTED_EXIT                      WorkerExitDetail = "ProcessUnexpectedExit"
	WorkerExitDetail_INITIALIZATION_FAILURE                       WorkerExitDetail = "InitializationFailure"
	WorkerExitDetail_INITIALIZATION_START_TIMEOUT                 WorkerExitDetail = "InitializationStartTimeout"
	WorkerExitDetail_COLD_START_FAILURE                           WorkerExitDetail = "ColdStartFailure"
	WorkerExitDetail_RESET_START_FAILURE                          WorkerExitDetail = "ResetStartFailure"
	WorkerExitDetail_FAST_RESTORE_START_FAILURE                   WorkerExitDetail = "FastRestoreStartFailure"
	WorkerExitDetail_RESTORE_START_FAILURE                        WorkerExitDetail = "RestoreStartFailure"
	WorkerExitDetail_FAST_SAVE_PRESERVE_PARTITION                 WorkerExitDetail = "FastSavePreservePartition"
	WorkerExitDetail_FAST_SAVE_PRESERVE_PARTITION_HANDLE_TRANSFER WorkerExitDetail = "FastSavePreservePartitionHandleTransfer"
	WorkerExitDetail_FAST_SAVE                                    WorkerExitDetail = "FastSave"
	WorkerExitDetail_CLONE_TEMPLATE                               WorkerExitDetail = "CloneTemplate"
	WorkerExitDetail_SAVE                                         WorkerExitDetail = "Save"
	WorkerExitDetail_MIGRATE                                      WorkerExitDetail = "Migrate"
	WorkerExitDetail_MIGRATE_FAILURE                              WorkerExitDetail = "MigrateFailure"
	WorkerExitDetail_CANNOT_REFERENCE_VM                          WorkerExitDetail = "CannotReferenceVm"
	WorkerExitDetail_MGOT_UNREGISTER                              WorkerExitDetail = "MgotUnregister"
)
