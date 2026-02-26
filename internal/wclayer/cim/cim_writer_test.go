//go:build windows

package cim

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/pkg/cimfs"
)

func TestSingleFileWriterTypeMismatch(t *testing.T) {
	if !cimfs.IsBlockCimWriteSupported() {
		t.Skipf("BlockCIM not supported")
	}

	layer := &cimfs.BlockCIM{
		Type:      cimfs.BlockCIMTypeSingleFile,
		BlockPath: "",
		CimName:   "",
	}

	parent := &cimfs.BlockCIM{
		Type:      cimfs.BlockCIMTypeDevice,
		BlockPath: "",
		CimName:   "",
	}

	_, err := NewBlockCIMLayerWriter(context.TODO(), layer, []*cimfs.BlockCIM{parent})
	if !errors.Is(err, ErrBlockCIMParentTypeMismatch) {
		t.Fatalf("expected error `%s`, got `%s`", ErrBlockCIMParentTypeMismatch, err)
	}
}

func TestSingleFileWriterInvalidBlockType(t *testing.T) {
	if !cimfs.IsBlockCimWriteSupported() {
		t.Skipf("BlockCIM not supported")
	}

	layer := &cimfs.BlockCIM{
		BlockPath: "",
		CimName:   "",
	}

	parent := &cimfs.BlockCIM{
		BlockPath: "",
		CimName:   "",
	}

	_, err := NewBlockCIMLayerWriter(context.TODO(), layer, []*cimfs.BlockCIM{parent})
	if !errors.Is(err, ErrBlockCIMWriterNotSupported) {
		t.Fatalf("expected error `%s`, got `%s`", ErrBlockCIMWriterNotSupported, err)
	}
}
