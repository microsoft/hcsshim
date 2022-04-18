package memory

import (
	"testing"
)

// helper function to test and validate minimal allocation scenario
func testAllocate(t *testing.T, ma *PoolAllocator, sz uint64) {
	_, err := ma.Allocate(sz)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(ma.pools[0].busy) != 1 {
		t.Fatal("memory slot wasn't marked as busy")
	}
}

func Test_MemAlloc_findNextOffset(t *testing.T) {
	ma := NewPoolMemoryAllocator()
	cls, offset, err := ma.findNextOffset(0)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if cls != memoryClassNumber-1 {
		t.Fatalf("expected class=%d, got %d", memoryClassNumber-1, cls)
	}
	if offset != 0 {
		t.Fatalf("expected offset=%d, got %d", 0, offset)
	}
}

func Test_MemAlloc_allocate_without_expand(t *testing.T) {
	ma := &PoolAllocator{}
	ma.pools[0] = newEmptyMemoryPool()
	ma.pools[0].free[0] = &region{
		class:  0,
		offset: 0,
	}

	testAllocate(t, ma, MiB)
}

func Test_MemAlloc_allocate_not_enough_space(t *testing.T) {
	ma := &PoolAllocator{}

	_, err := ma.Allocate(MiB)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != ErrNotEnoughSpace {
		t.Fatalf("expected error=%s, got error=%s", ErrNotEnoughSpace, err)
	}
}

func Test_MemAlloc_expand(t *testing.T) {
	pa := &PoolAllocator{}
	pa.pools[1] = newEmptyMemoryPool()
	pa.pools[1].free[0] = &region{
		class:  1,
		offset: 0,
	}

	err := pa.split(0)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if _, o, err := pa.findNextOffset(1); err == nil {
		t.Fatalf("no free offset should be found for class 1, got offset=%d", o)
	}

	poolCls0 := pa.pools[0]
	for i := 0; i < 4; i++ {
		offset := uint64(i) * MiB
		_, ok := poolCls0.free[offset]
		if !ok {
			t.Fatalf("did not find region with offset=%d", offset)
		}
		delete(poolCls0.free, offset)
	}

	if len(poolCls0.free) > 0 {
		t.Fatalf("extra memory regions: %v", poolCls0.free)
	}
}

func Test_MemAlloc_allocate_automatically_expands(t *testing.T) {
	pa := &PoolAllocator{}
	pa.pools[2] = newEmptyMemoryPool()
	pa.pools[2].free[MiB] = &region{
		class:  2,
		offset: MiB,
	}

	testAllocate(t, pa, MiB)

	if pa.pools[1] == nil {
		t.Fatalf("memory not extended for class type 1")
	}
	if len(pa.pools[2].free) > 0 {
		t.Fatalf("expected no free regions for class type 2, got: %v", pa.pools[2].free)
	}
}

func Test_MemAlloc_alloc_and_release(t *testing.T) {
	pa := &PoolAllocator{}
	pa.pools[0] = newEmptyMemoryPool()
	r := &region{
		class:  0,
		offset: 0,
	}
	pa.pools[0].free[0] = r

	testAllocate(t, pa, MiB)

	err := pa.Release(r)
	if err != nil {
		t.Fatalf("error releasing resources: %s", err)
	}
	if len(pa.pools[0].busy) != 0 {
		t.Fatalf("resources not marked as free: %v", pa.pools[0].busy)
	}
	if len(pa.pools[0].free) == 0 {
		t.Fatal("resource not assigned back to the free pool")
	}
}

func Test_MemAlloc_alloc_invalid_larger_than_max(t *testing.T) {
	pa := &PoolAllocator{}

	_, err := pa.Allocate(maximumClassSize + 1)
	if err == nil {
		t.Fatal("no error returned")
	}
	if err != ErrInvalidMemoryClass {
		t.Fatalf("expected error=%s, got error=%s", ErrInvalidMemoryClass, err)
	}
}

func Test_MemAlloc_release_invalid_offset(t *testing.T) {
	pa := &PoolAllocator{}
	pa.pools[0] = newEmptyMemoryPool()
	r := &region{
		class:  0,
		offset: 0,
	}
	pa.pools[0].free[0] = r

	testAllocate(t, pa, MiB)

	// change the actual offset
	r.offset = MiB
	err := pa.Release(r)
	if err == nil {
		t.Fatal("no error returned")
	}
	if err != ErrNotAllocated {
		t.Fatalf("wrong error returned: %s", err)
	}
}

func Test_MemAlloc_Max_Out(t *testing.T) {
	ma := NewPoolMemoryAllocator()
	for i := 0; i < 4096; i++ {
		_, err := ma.Allocate(MiB)
		if err != nil {
			t.Fatalf("unexpected error during memory allocation: %s", err)
		}
	}
	if len(ma.pools[0].busy) != 4096 {
		t.Fatalf("expected 4096 busy blocks of class 0, got %d instead", len(ma.pools[0].busy))
	}
	for i := 0; i < 4096; i++ {
		offset := uint64(i) * MiB
		if _, ok := ma.pools[0].busy[offset]; !ok {
			t.Fatalf("expected to find offset %d", offset)
		}
	}
}

func Test_GetMemoryClass(t *testing.T) {
	type config struct {
		name     string
		size     uint64
		expected classType
	}

	testCases := []config{
		{
			name:     "Size_1MB_Class_0",
			size:     MiB,
			expected: 0,
		},
		{
			name:     "Size_6MB_Class_2",
			size:     6 * MiB,
			expected: 2,
		},
		{
			name:     "Size_2GB_Class_6",
			size:     2 * GiB,
			expected: 6,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := GetMemoryClassType(tc.size)
			if c != tc.expected {
				t.Fatalf("expected classType for size: %d is %d, got %d instead", tc.size, tc.expected, c)
			}
		})
	}
}

func Test_GetMemoryClassSize(t *testing.T) {
	type config struct {
		name     string
		clsType  classType
		expected uint64
		err      error
	}

	testCases := []config{
		{
			name:     "Class_0_Size_1MB",
			clsType:  0,
			expected: minimumClassSize,
		},
		{
			name:     "Class_8_Size_4GB",
			clsType:  6,
			expected: maximumClassSize,
		},
		{
			name:    "Class_7_ErrorInvalidMemoryClass",
			clsType: 7,
			err:     ErrInvalidMemoryClass,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := GetMemoryClassSize(tc.clsType)
			if err != tc.err {
				t.Fatalf("expected error to be %s, got %s instead", tc.err, err)
			}
			if s != tc.expected {
				t.Fatalf("expected size to be %d, got %d instead", tc.expected, s)
			}
		})
	}
}
