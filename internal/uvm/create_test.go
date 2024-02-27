//go:build windows

package uvm

import (
	"context"
	"testing"
)

// Unit tests for negative testing of input to uvm.Create()

func TestCreateBadBootFilesPath(t *testing.T) {
	ctx := context.Background()
	opts := NewDefaultOptionsLCOW(t.Name(), "")
	opts.UpdateBootFilesPath(ctx, `c:\does\not\exist\I\hope`)

	_, err := CreateLCOW(ctx, opts)
	if err == nil || err.Error() != `kernel: 'c:\does\not\exist\I\hope\kernel' not found` {
		t.Fatal(err)
	}
}
