//go:build windows

/*
Package windows provides an abstraction layer over Windows API calls to enable better testability.

# Purpose

The API interface abstracts Windows system calls, allowing them to be mocked in tests.
This is particularly useful for testing code that interacts with processes, tokens, and security
identifiers without requiring actual Windows system resources.

# Usage

## Production Code

In production code, use the NewWindowsAPI() function to get the real implementation:

	import (
		"github.com/Microsoft/hcsshim/internal/windows"
	)

	func MyFunction(ctx context.Context) error {
		winAPI := &windows.WinAPI{}
		// Use winAPI instead of calling windows package directly
		snapshot, err := winAPI.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
		// ...
	}

## Testing

In tests, create a mock implementation of the API interface. Here's a simple example:

	// Function to test
	func GetProcessSnapshot(api windows.API) (windows.Handle, error) {
		snapshot, err := api.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
		if err != nil {
			return 0, err
		}
		return snapshot, nil
	}

	// Mock implementation for testing
	type mockAPI struct {
		snapshot windows.Handle
		err      error
	}

	func (m *mockAPI) CreateToolhelp32Snapshot(flags uint32, processID uint32) (windows.Handle, error) {
		return m.snapshot, m.err
	}

	func (m *mockAPI) CloseHandle(h windows.Handle) error {
		return nil
	}

	// ... implement other API methods as needed ...

	// Test using the mock
	func TestGetProcessSnapshot(t *testing.T) {
		mockWinAPI := &mockAPI{
			snapshot: windows.Handle(12345),
			err:      nil,
		}

		snapshot, err := GetProcessSnapshot(mockWinAPI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if snapshot != windows.Handle(12345) {
			t.Fatalf("expected snapshot 12345, got %v", snapshot)
		}
	}

All method names match the underlying Windows API calls for easy understanding and reference.
*/
package windows
