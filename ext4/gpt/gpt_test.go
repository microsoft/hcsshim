package gpt

import "testing"

func Test_FindNextUnusedLogicalBlock(t *testing.T) {
	type config struct {
		name          string
		bytePosition  uint64
		expectedBlock uint64
	}
	tests := []config{
		{
			name:         "Beginning of a block",
			bytePosition: 0,
			// since the block hasn't actually been used, the 0th block is the "next unused"
			expectedBlock: 0,
		},
		{
			name:         "Middle of a block",
			bytePosition: 513,
			// the 0th block has been used and the 1st byte into the 1st block has been used
			// so the next unused block is the 2nd
			expectedBlock: 2,
		},
		{
			name:         "End of a block",
			bytePosition: 512,
			// the 0th block has been fully used, so the next unused block is the 1st
			expectedBlock: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(subtest *testing.T) {
			actual := FindNextUnusedLogicalBlock(test.bytePosition)
			if actual != test.expectedBlock {
				subtest.Fatalf("expected to get block %v, instead got block %v", test.expectedBlock, actual)
			}
		})

	}
}
