//go:build windows

package hcs

import (
	"github.com/Microsoft/hcsshim/internal/computecore"
	"github.com/Microsoft/hcsshim/internal/vmcompute"
)

// Function variables for HCS compute system and process API calls.
// Production code calls these directly; tests swap them to intercept.

// --- Compute System Lifecycle ---

var hcsCreateComputeSystem = vmcompute.HcsCreateComputeSystem
var hcsOpenComputeSystem = vmcompute.HcsOpenComputeSystem
var hcsCloseComputeSystem = vmcompute.HcsCloseComputeSystem
var hcsShutdownComputeSystem = vmcompute.HcsShutdownComputeSystem
var hcsTerminateComputeSystem = vmcompute.HcsTerminateComputeSystem
var hcsPauseComputeSystem = vmcompute.HcsPauseComputeSystem
var hcsResumeComputeSystem = vmcompute.HcsResumeComputeSystem
var hcsSaveComputeSystem = vmcompute.HcsSaveComputeSystem

// --- Compute System Operations ---

var hcsGetComputeSystemProperties = vmcompute.HcsGetComputeSystemProperties
var hcsModifyComputeSystem = vmcompute.HcsModifyComputeSystem
var hcsEnumerateComputeSystems = vmcompute.HcsEnumerateComputeSystems

// --- Compute System Callbacks ---

var hcsRegisterComputeSystemCallback = vmcompute.HcsRegisterComputeSystemCallback
var hcsUnregisterComputeSystemCallback = vmcompute.HcsUnregisterComputeSystemCallback

// --- Computecore Operation API (used by Start) ---
//
// HcsStartComputeSystem migrated from vmcompute to the operation-based
// computecore API. The wrappers below preserve testability by allowing tests
// to substitute fake implementations of each call in the Start path.

var hcsCreateOperation = computecore.HcsCreateOperation
var hcsCloseOperation = computecore.HcsCloseOperation
var hcsStartComputeSystem = computecore.HcsStartComputeSystem
var hcsWaitForOperationResult = computecore.HcsWaitForOperationResult

// --- Process Lifecycle ---

var hcsCreateProcess = vmcompute.HcsCreateProcess
var hcsOpenProcess = vmcompute.HcsOpenProcess
var hcsCloseProcess = vmcompute.HcsCloseProcess
var hcsTerminateProcess = vmcompute.HcsTerminateProcess

// --- Process Operations ---

var hcsSignalProcess = vmcompute.HcsSignalProcess
var hcsGetProcessInfo = vmcompute.HcsGetProcessInfo
var hcsGetProcessProperties = vmcompute.HcsGetProcessProperties
var hcsModifyProcess = vmcompute.HcsModifyProcess

// --- Process Callbacks ---

var hcsRegisterProcessCallback = vmcompute.HcsRegisterProcessCallback
var hcsUnregisterProcessCallback = vmcompute.HcsUnregisterProcessCallback
