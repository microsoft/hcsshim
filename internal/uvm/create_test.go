//go:build windows

package uvm

import (
	"context"
	"fmt"
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

func TestCreateWCOWBadLayerFolders(t *testing.T) {
	opts := NewDefaultOptionsWCOW(t.Name(), "")
	_, err := CreateWCOW(context.Background(), opts)
	errMsg := fmt.Sprintf("%s: %s", errBadUVMOpts, "at least 2 LayerFolders must be supplied")
	if err == nil || err.Error() != errMsg {
		t.Fatal(err)
	}
}
