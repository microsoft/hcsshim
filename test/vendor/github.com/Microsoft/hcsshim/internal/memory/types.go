package memory

import "github.com/pkg/errors"

type classType uint32

const (
	MegaByte uint64 = 1024 * 1024
	GigaByte uint64 = 1024 * MegaByte
)

var (
	ErrNotEnoughSpace = errors.New("not enough space")
	ErrNotAllocated   = errors.New("no memory allocated at the given offset")
)

// MappedRegion represents a memory block with an offset
type MappedRegion interface {
	Offset() uint64
	Size() uint64
	Type() classType
}

// Allocator is an interface for memory allocation
type Allocator interface {
	Allocate(uint64) (MappedRegion, error)
	Release(MappedRegion) error
}
