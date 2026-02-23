//go:build windows

// Package vmutils provides shared utility functions for working with Utility VMs.
//
// This package contains stateless utility functions that can be used by both the
// legacy UVM management code (internal/uvm) and the new controller-based architecture
// (internal/controller). Functions in this package are designed to be decoupled from
// specific UVM implementations.
//
// This allows different shims (containerd-shim-runhcs-v1, containerd-shim-lcow-v1)
// to share common logic while maintaining their own orchestration patterns.
package vmutils
