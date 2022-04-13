//go:build linux
// +build linux

package amdsevsnp

import (
	"testing"
)

func Test_Mirror_NonEmpty_Byte_Slices(t *testing.T) {
	type config struct {
		name     string
		input    []byte
		expected []byte
	}

	for _, conf := range []config{
		{
			name:     "Length0",
			input:    []byte{},
			expected: []byte{},
		},
		{
			name:     "Length1",
			input:    []byte{100},
			expected: []byte{100},
		},
		{
			name:     "LengthOdd",
			input:    []byte{100, 101, 102, 103, 104},
			expected: []byte{104, 103, 102, 101, 100},
		},
		{
			name:     "LengthEven",
			input:    []byte{100, 101, 102, 103, 104, 105},
			expected: []byte{105, 104, 103, 102, 101, 100},
		},
	} {
		t.Run(conf.name, func(t *testing.T) {
			result := mirrorBytes(conf.input)
			if string(result[:]) != string(conf.expected[:]) {
				t.Fatalf("the ipnut byte array %+v was not mirrored; %+v", conf.input, result)
			}
		})
	}
}

func Test_Mirror_Nil_Slice(t *testing.T) {
	result := mirrorBytes(nil)
	if result != nil {
		t.Fatalf("expected nil slice, got: %+v", result)
	}
}
