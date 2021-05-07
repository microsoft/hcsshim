package dmverity

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/pkg/errors"

	"github.com/Microsoft/hcsshim/ext4/internal/compactext4"
)

const (
	blockSize = compactext4.BlockSize
)

var (
	salt                      = bytes.Repeat([]byte{0}, 32)
	ErrSuperBlockReadFailure  = errors.New("failed to read dm-verity super block")
	ErrSuperBlockParseFailure = errors.New("failed to parse dm-verity super block")
	ErrRootHashReadFailure    = errors.New("failed to read dm-verity root hash")
)

type dmveritySuperblock struct {
	/* (0) "verity\0\0" */
	Signature [8]byte
	/* (8) superblock version, 1 */
	Version uint32
	/* (12) 0 - Chrome OS, 1 - normal */
	HashType uint32
	/* (16) UUID of hash device */
	UUID [16]byte
	/* (32) Name of the hash algorithm (e.g., sha256) */
	Algorithm [32]byte
	/* (64) The data block size in bytes */
	DataBlockSize uint32
	/* (68) The hash block size in bytes */
	HashBlockSize uint32
	/* (72) The number of data blocks */
	DataBlocks uint64
	/* (80) Size of the salt */
	SaltSize uint16
	/* (82) Padding */
	_ [6]byte
	/* (88) The salt */
	Salt [256]byte
	/* (344) Padding */
	_ [168]byte
}

// VerityInfo is minimal exported version of dmveritySuperblock
type VerityInfo struct {
	// Offset in blocks on hash device
	HashOffsetInBlocks int64
	// Set to true, when dm-verity super block is also written on the hash device
	SuperBlock    bool
	RootDigest    string
	Salt          string
	Algorithm     string
	DataBlockSize uint32
	HashBlockSize uint32
	DataBlocks    uint64
}

func Tree(data []byte) []byte {
	layers := make([][]byte, 0)

	var current_level = bytes.NewBuffer(data)

	for current_level.Len() != blockSize {
		var blocks = current_level.Len() / blockSize
		var next_level = bytes.NewBuffer(make([]byte, 0))

		for i := 0; i < blocks; i++ {
			block := make([]byte, blockSize)
			_, _ = current_level.Read(block)
			h := hash2(salt, block)
			next_level.Write(h)
		}

		padding := bytes.Repeat([]byte{0}, blockSize-(next_level.Len()%blockSize))
		next_level.Write(padding)

		current_level = next_level
		layers = append(layers, current_level.Bytes())
	}

	var tree = bytes.NewBuffer(make([]byte, 0))
	for i := len(layers) - 1; i >= 0; i-- {
		tree.Write(layers[i])
	}

	return tree.Bytes()
}

func RootHash(tree []byte) []byte {
	return hash2(salt, tree[:blockSize])
}

func MakeDMVeritySuperblock(size uint64) *dmveritySuperblock {
	superblock := &dmveritySuperblock{
		Version:       1,
		HashType:      1,
		UUID:          generateUUID(),
		DataBlockSize: blockSize,
		HashBlockSize: blockSize,
		DataBlocks:    size / blockSize,
		SaltSize:      uint16(len(salt)),
	}

	copy(superblock.Signature[:], "verity")
	copy(superblock.Algorithm[:], "sha256")
	copy(superblock.Salt[:], salt)

	return superblock
}

func hash2(a, b []byte) []byte {
	h := sha256.New()
	h.Write(append(a, b...))
	return h.Sum(nil)
}

func generateUUID() [16]byte {
	res := [16]byte{}
	if _, err := rand.Read(res[:]); err != nil {
		panic(err)
	}
	return res
}

// ReadDMVeritySuperBlockAndRootHash extracts dm-verity super block information and merkle tree root hash
func ReadDMVeritySuperBlockAndRootHash(vhdPath string, offsetInBytes int64) (*VerityInfo, error) {
	vhd, err := os.OpenFile(vhdPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer vhd.Close()

	// Skip the ext4 data to get to dm-verity super block
	if s, err := vhd.Seek(offsetInBytes, io.SeekStart); err != nil || s != offsetInBytes {
		return nil, errors.Wrap(err, "failed to seek dm-verity super block")
	}

	block := make([]byte, blockSize)
	if s, err := vhd.Read(block); err != nil || s != blockSize {
		return nil, ErrSuperBlockReadFailure
	}

	var dmvSB = &dmveritySuperblock{}
	b := bytes.NewBuffer(block)
	if err := binary.Read(b, binary.LittleEndian, dmvSB); err != nil {
		return nil, ErrSuperBlockParseFailure
	}

	// read the merkle tree root
	if s, err := vhd.Read(block); err != nil || s != blockSize {
		return nil, ErrRootHashReadFailure
	}
	rootHash := hash2(dmvSB.Salt[:dmvSB.SaltSize], block)
	return &VerityInfo{
		RootDigest:         fmt.Sprintf("%x", rootHash),
		Algorithm:          string(bytes.Trim(dmvSB.Algorithm[:], "\x00")),
		Salt:               fmt.Sprintf("%x", dmvSB.Salt[:dmvSB.SaltSize]),
		HashOffsetInBlocks: int64(dmvSB.DataBlocks),
		SuperBlock:         true,
		DataBlocks:         dmvSB.DataBlocks,
		DataBlockSize:      dmvSB.DataBlockSize,
		HashBlockSize:      blockSize,
	}, nil
}
