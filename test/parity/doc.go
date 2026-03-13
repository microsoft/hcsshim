//go:build windows

// Package parity validates that the v2 LCOW document builder
// (lcow.BuildSandboxConfig) produces HCS ComputeSystem documents equivalent
// to the legacy shim pipeline (oci → uvm.MakeLCOWDocument).
//
// # How it works
//
// Each test case defines a set of annotations, shim options, and devices.
// These inputs are fed identically to both pipelines:
//
//	Legacy: specs.Spec + runhcsopts.Options
//	  → oci.UpdateSpecFromOptions
//	  → oci.ProcessAnnotations
//	  → oci.SpecToUVMCreateOpts → *OptionsLCOW
//	  → uvm.MakeLCOWDocument → *hcsschema.ComputeSystem
//
//	V2: vm.Spec + runhcsopts.Options
//	  → lcow.BuildSandboxConfig → *hcsschema.ComputeSystem + *SandboxOptions
//
// The resulting documents are normalized (random GUID map keys sorted,
// owner zeroed, nil-vs-empty-struct equalized) then compared with go-cmp.
//
// The test also compares legacy OptionsLCOW fields against v2 SandboxOptions
// to verify configuration semantics are preserved.
//
// # File layout
//
//	doc.go                    — this file
//	lcow_doc_test.go          — test cases and input construction (inline)
//	legacy_pipeline_test.go   — buildLegacyDocument: wires the 4-step legacy pipeline
//	v2_pipeline_test.go       — buildV2Document: wraps lcow.BuildSandboxConfig
//	helpers_test.go           — setupBootFiles, normalizeDoc, sorted map helpers, mustJSON
//
// # Running
//
// Requires: Windows with Hyper-V enabled, admin (elevated) PowerShell.
//
//	cd test
//	go test -tags functional -v -count=1 ./parity/
package parity
