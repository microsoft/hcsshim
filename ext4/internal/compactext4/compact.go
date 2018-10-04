package compactext4

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/Microsoft/hcsshim/ext4/internal/format"
)

// Writer writes a compact ext4 file system.
type Writer struct {
	f                    io.WriteSeeker
	bw                   *bufio.Writer
	inodes               []*inode
	curInode             *inode
	pos                  int64
	dataWritten, dataMax int64
	initialized          bool
	supportInlineData    bool
}

// Mode flags for Linux files.
const (
	S_IXOTH  = format.S_IXOTH
	S_IWOTH  = format.S_IWOTH
	S_IROTH  = format.S_IROTH
	S_IXGRP  = format.S_IXGRP
	S_IWGRP  = format.S_IWGRP
	S_IRGRP  = format.S_IRGRP
	S_IXUSR  = format.S_IXUSR
	S_IWUSR  = format.S_IWUSR
	S_IRUSR  = format.S_IRUSR
	S_ISVTX  = format.S_ISVTX
	S_ISGID  = format.S_ISGID
	S_ISUID  = format.S_ISUID
	S_IFIFO  = format.S_IFIFO
	S_IFCHR  = format.S_IFCHR
	S_IFDIR  = format.S_IFDIR
	S_IFBLK  = format.S_IFBLK
	S_IFREG  = format.S_IFREG
	S_IFLNK  = format.S_IFLNK
	S_IFSOCK = format.S_IFSOCK

	TypeMask = format.TypeMask
)

type inode struct {
	Size                        int64
	Atime, Ctime, Mtime, Crtime uint64
	Number                      format.InodeNumber
	Mode                        uint16
	Uid, Gid                    uint32
	LinkCount                   uint32
	XattrBlock                  uint32
	BlockCount                  uint32
	Devmajor, Devminor          uint32
	Flags                       format.InodeFlag
	Data                        []byte
	XattrInline                 []byte
	Children                    directory
}

func (inode *inode) IsDir() bool {
	return inode.Mode&format.TypeMask == S_IFDIR
}

// A File represents a file to be added to an ext4 file system.
type File struct {
	Linkname                    string
	Size                        int64
	Mode                        uint16
	Uid, Gid                    uint32
	Atime, Ctime, Mtime, Crtime time.Time
	Devmajor, Devminor          uint32
	Xattrs                      map[string][]byte
}

const (
	inodeFirst        = 11
	inodeLostAndFound = inodeFirst

	blockSize               = 4096
	blocksPerGroup          = blockSize * 8
	inodeSize               = 256
	maxInodesPerGroup       = blockSize * 8 // Limited by the inode bitmap
	inodesPerGroupIncrement = blockSize / inodeSize

	maxFileSize             = 128 * 1024 * 1024 * 1024 // 128GB file size maximum for now
	smallSymlinkSize        = 59                       // max symlink size that goes directly in the inode
	maxBlocksPerExtent      = 0x8000                   // maximum number of blocks in an extent
	inodeDataSize           = 60
	inodeUsedSize           = 152 // fields through CrtimeExtra
	inodeExtraSize          = inodeSize - inodeUsedSize
	xattrInodeOverhead      = 4 + 4                       // magic number + empty next entry value
	xattrBlockOverhead      = 32 + 4                      // header + empty next entry value
	inlineDataXattrOverhead = xattrInodeOverhead + 16 + 4 // entry + "data"
	inlineDataSize          = inodeDataSize + inodeExtraSize - inlineDataXattrOverhead
)

var directoryEntrySize = binary.Size(format.DirectoryEntry{})
var extraIsize = uint16(inodeUsedSize - 128)

type directory map[string]*inode

func splitFirst(p string) (string, string) {
	n := strings.IndexByte(p, '/')
	if n >= 0 {
		return p[:n], p[n+1:]
	}
	return p, ""
}

func (w *Writer) findPath(root *inode, p string) *inode {
	inode := root
	for inode != nil && len(p) != 0 {
		name, rest := splitFirst(p)
		p = rest
		inode = inode.Children[name]
	}
	return inode
}

func timeToFsTime(t time.Time) uint64 {
	if t.IsZero() {
		return 0
	}
	s := t.Unix()
	if s < -0x80000000 {
		return 0x80000000
	}
	if s > 0x37fffffff {
		return 0x37fffffff
	}
	return uint64(s) | uint64(t.Nanosecond())<<34
}

func (w *Writer) getInode(i format.InodeNumber) *inode {
	if i == 0 || int(i) > len(w.inodes) {
		return nil
	}
	return w.inodes[i-1]
}

func compressXattrName(name string) (uint8, string) {
	prefixes := []struct {
		Index  uint8
		Prefix string
	}{
		{2, "system.posix_acl_access"},
		{3, "system.posix_acl_default"},
		{8, "system.richacl"},
		{7, "system."},
		{1, "user."},
		{4, "trusted."},
		{6, "security."},
	}

	for _, p := range prefixes {
		if strings.HasPrefix(name, p.Prefix) {
			return p.Index, name[len(p.Prefix):]
		}
	}
	return 0, name
}

func hashXattrEntry(name string, value []byte) uint32 {
	var hash uint32
	for i := 0; i < len(name); i++ {
		hash = (hash << 5) ^ (hash >> 27) ^ uint32(name[i])
	}

	for i := 0; i+3 < len(value); i += 4 {
		hash = (hash << 16) ^ (hash >> 16) ^ binary.LittleEndian.Uint32(value[i:i+4])
	}

	if len(value)%4 != 0 {
		var last [4]byte
		copy(last[:], value[len(value)&^3:])
		hash = (hash << 16) ^ (hash >> 16) ^ binary.LittleEndian.Uint32(last[:])
	}
	return hash
}

type xattr struct {
	Name  string
	Index uint8
	Value []byte
}

func (x *xattr) EntryLen() int {
	return (len(x.Name)+3)&^3 + 16
}

func (x *xattr) ValueLen() int {
	return (len(x.Value) + 3) &^ 3
}

type xattrState struct {
	inode, block         []xattr
	inodeLeft, blockLeft int
}

func (s *xattrState) init() {
	s.inodeLeft = inodeExtraSize - xattrInodeOverhead
	s.blockLeft = blockSize - xattrBlockOverhead
}

func (s *xattrState) addXattr(name string, value []byte) bool {
	index, name := compressXattrName(name)
	x := xattr{
		Index: index,
		Name:  name,
		Value: value,
	}
	length := x.EntryLen() + x.ValueLen()
	if s.inodeLeft >= length {
		s.inode = append(s.inode, x)
		s.inodeLeft -= length
	} else if s.blockLeft >= length {
		s.block = append(s.block, x)
		s.blockLeft -= length
	} else {
		return false
	}
	return true
}

func putXattrs(xattrs []xattr, b []byte, offsetDelta uint16) {
	offset := uint16(len(b)) + offsetDelta
	eb := b
	db := b
	for _, xattr := range xattrs {
		vl := xattr.ValueLen()
		offset -= uint16(vl)
		eb[0] = uint8(len(xattr.Name))
		eb[1] = xattr.Index
		binary.LittleEndian.PutUint16(eb[2:], offset)
		binary.LittleEndian.PutUint32(eb[8:], uint32(len(xattr.Value)))
		binary.LittleEndian.PutUint32(eb[12:], hashXattrEntry(xattr.Name, xattr.Value))
		copy(eb[16:], xattr.Name)
		eb = eb[xattr.EntryLen():]
		copy(db[len(db)-vl:], xattr.Value)
		db = db[:len(db)-vl]
	}
}

func (w *Writer) writeXattrs(inode *inode, state *xattrState) error {
	if len(state.inode) != 0 {
		inode.XattrInline = make([]byte, inodeExtraSize)
		binary.LittleEndian.PutUint32(inode.XattrInline[0:], format.XAttrHeaderMagic) // Magic
		putXattrs(state.inode, inode.XattrInline[4:], 0)
	}

	if len(state.block) != 0 {
		sort.Slice(state.block, func(i, j int) bool {
			return state.block[i].Index < state.block[j].Index ||
				len(state.block[i].Name) < len(state.block[j].Name) ||
				state.block[i].Name < state.block[j].Name
		})

		var b [blockSize]byte
		binary.LittleEndian.PutUint32(b[0:], format.XAttrHeaderMagic) // Magic
		binary.LittleEndian.PutUint32(b[4:], 1)                       // ReferenceCount
		binary.LittleEndian.PutUint32(b[8:], 1)                       // Blocks
		putXattrs(state.block, b[32:], 32)

		inode.XattrBlock = uint32(w.pos / blockSize)
		inode.BlockCount++
		if _, err := w.bw.Write(b[:]); err != nil {
			return err
		}
		w.pos += blockSize
	}

	return nil
}

func (w *Writer) makeInode(f *File, node *inode) (*inode, error) {
	mode := f.Mode
	if mode&format.TypeMask == 0 {
		mode |= format.S_IFREG
	}
	typ := mode & format.TypeMask
	ino := format.InodeNumber(len(w.inodes) + 1)
	if node == nil {
		node = &inode{
			Number: ino,
		}
		if typ == S_IFDIR {
			node.Children = make(directory)
			node.LinkCount = 1 // A directory is linked to itself.
		}
	} else if node.XattrBlock != 0 || node.Flags&format.InodeFlagExtents != 0 {
		// Since we cannot deallocate or reuse blocks, don't allow updates that
		// would invalidate data that has already been written.
		return nil, errors.New("cannot overwrite file with xattrs or data")
	}
	node.Mode = mode
	node.Uid = f.Uid
	node.Gid = f.Gid
	node.Flags = format.InodeFlagHugeFile
	node.Atime = timeToFsTime(f.Atime)
	node.Ctime = timeToFsTime(f.Ctime)
	node.Mtime = timeToFsTime(f.Mtime)
	node.Crtime = timeToFsTime(f.Crtime)
	node.Devmajor = f.Devmajor
	node.Devminor = f.Devminor
	node.Data = nil
	node.XattrInline = nil

	var xstate xattrState
	xstate.init()

	var size int64
	switch typ {
	case format.S_IFREG:
		size = f.Size
		if f.Size > maxFileSize {
			return nil, fmt.Errorf("file too big: %d > %d", f.Size, maxFileSize)
		}
		if f.Size <= inlineDataSize && w.supportInlineData {
			node.Data = make([]byte, f.Size)
			extra := 0
			if f.Size > inodeDataSize {
				extra = int(f.Size - inodeDataSize)
			}
			// Add a dummy entry for now.
			if !xstate.addXattr("system.data", node.Data[:extra]) {
				panic("not enough room for inline data")
			}
			node.Flags |= format.InodeFlagInlineData
		}
	case format.S_IFLNK:
		node.Mode |= 0777 // Symlinks should appear as ugw rwx
		size = int64(len(f.Linkname))
		if size <= smallSymlinkSize {
			// Special case: small symlinks go directly in Block without setting
			// an inline data flag.
			node.Data = make([]byte, len(f.Linkname))
			copy(node.Data, f.Linkname)
		}
	case format.S_IFDIR, format.S_IFIFO, format.S_IFSOCK, format.S_IFCHR, format.S_IFBLK:
	default:
		return nil, fmt.Errorf("invalid mode %o", mode)
	}

	// Accumulate the extended attributes.
	if len(f.Xattrs) != 0 {
		// Sort the xattrs to avoid non-determinism in map iteration.
		var xattrs []string
		for name := range f.Xattrs {
			xattrs = append(xattrs, name)
		}
		sort.Strings(xattrs)
		for _, name := range xattrs {
			if !xstate.addXattr(name, f.Xattrs[name]) {
				return nil, fmt.Errorf("could not fit xattr %s", name)
			}
		}
	}

	if err := w.writeXattrs(node, &xstate); err != nil {
		return nil, err
	}

	node.Size = size
	if typ == format.S_IFLNK && size > smallSymlinkSize {
		// Write the link name as data.
		w.startInode(node, size)
		if _, err := w.Write([]byte(f.Linkname)); err != nil {
			return nil, err
		}
		if err := w.finishInode(); err != nil {
			return nil, err
		}
	}

	if int(node.Number-1) >= len(w.inodes) {
		w.inodes = append(w.inodes, node)
	}
	return node, nil
}

func (w *Writer) root() *inode {
	return w.getInode(format.InodeRoot)
}

func (w *Writer) lookup(name string, mustExist bool) (*inode, *inode, string, error) {
	root := w.root()
	cleanname := path.Clean("/" + name)[1:]
	if len(cleanname) == 0 {
		return root, root, "", nil
	}
	dirname, childname := path.Split(cleanname)
	if len(childname) == 0 || len(childname) > 0xff {
		return nil, nil, "", fmt.Errorf("%s: invalid name", name)
	}
	dir := w.findPath(root, dirname)
	if dir == nil || !dir.IsDir() {
		return nil, nil, "", fmt.Errorf("%s: path not found", name)
	}
	child := dir.Children[childname]
	if child == nil && mustExist {
		return nil, nil, "", fmt.Errorf("%s: file not found", name)
	}
	return dir, child, childname, nil
}

// Create adds a file to the file system.
func (w *Writer) Create(name string, f *File) error {
	if err := w.finishInode(); err != nil {
		return err
	}
	dir, existing, childname, err := w.lookup(name, false)
	if err != nil {
		return err
	}
	var reuse *inode
	if existing != nil {
		if existing.IsDir() {
			if f.Mode&TypeMask != S_IFDIR {
				return fmt.Errorf("%s: cannot replace a directory with a file", name)
			}
			reuse = existing
		} else if f.Mode&TypeMask == S_IFDIR {
			return fmt.Errorf("%s: cannot replace a file with a directory", name)
		} else if existing.LinkCount < 2 {
			reuse = existing
		}
	}
	child, err := w.makeInode(f, reuse)
	if err != nil {
		return fmt.Errorf("%s: %s", name, err)
	}
	if existing != child {
		if existing != nil {
			existing.LinkCount--
		}
		dir.Children[childname] = child
		child.LinkCount++
		if child.IsDir() {
			dir.LinkCount++
		}
	}
	if child.Mode&format.TypeMask == format.S_IFREG && f.Size != 0 {
		w.startInode(child, f.Size)
	}
	return nil
}

// Link adds a hard link to the file system.
func (w *Writer) Link(oldname, newname string) error {
	if err := w.finishInode(); err != nil {
		return err
	}
	newdir, existing, newchildname, err := w.lookup(newname, false)
	if err != nil {
		return err
	}
	if existing != nil && (existing.IsDir() || existing.LinkCount < 2) {
		return fmt.Errorf("%s: cannot orphan existing file or directory", newname)
	}

	_, oldfile, _, err := w.lookup(oldname, true)
	if err != nil {
		return err
	}
	switch oldfile.Mode & format.TypeMask {
	case format.S_IFDIR, format.S_IFLNK:
		return fmt.Errorf("link target cannot be a directory or symlink: %s", oldname)
	}

	if existing != nil {
		existing.LinkCount--
	}
	oldfile.LinkCount++
	newdir.Children[newchildname] = oldfile
	return nil
}

func (w *Writer) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if w.dataWritten+int64(len(b)) > w.dataMax {
		return 0, fmt.Errorf("wrote too much: %d > %d", w.dataWritten, w.dataMax)
	}

	if w.curInode.Flags&format.InodeFlagInlineData != 0 {
		copy(w.curInode.Data[w.dataWritten:], b)
		w.dataWritten += int64(len(b))
		return len(b), nil
	}

	n, err := w.bw.Write(b)
	w.dataWritten += int64(n)
	w.pos += int64(n)
	return n, err
}

func (w *Writer) startInode(inode *inode, size int64) {
	if w.curInode != nil {
		panic("inode already in progress")
	}
	w.curInode = inode
	w.dataWritten = 0
	w.dataMax = size
}

func (w *Writer) nextBlock() error {
	if w.pos%blockSize != 0 {
		_, err := io.CopyN(w.bw, zero, blockSize-w.pos%blockSize)
		if err != nil {
			return err
		}
		w.pos += blockSize - w.pos%blockSize
	}
	return nil
}

func fillExtents(hdr *format.ExtentHeader, extents []format.ExtentLeafNode, startBlock, offset, inodeSize uint32) {
	*hdr = format.ExtentHeader{
		Magic:   format.ExtentHeaderMagic,
		Entries: uint16(len(extents)),
		Max:     uint16(cap(extents)),
		Depth:   0,
	}
	for i := range extents {
		block := offset + uint32(i)*maxBlocksPerExtent
		length := inodeSize - block
		if length > maxBlocksPerExtent {
			length = maxBlocksPerExtent
		}
		start := startBlock + block
		extents[i] = format.ExtentLeafNode{
			Block:    block,
			Length:   uint16(length),
			StartLow: start,
		}
	}
}

func (w *Writer) writeExtents(inode *inode) error {
	start := w.pos - w.dataWritten
	if start%blockSize != 0 {
		panic("unaligned")
	}
	if err := w.nextBlock(); err != nil {
		return err
	}

	startBlock := uint32(start / blockSize)
	blocks := uint32(w.pos/blockSize) - startBlock
	usedBlocks := blocks

	const extentNodeSize = 12
	const extentsPerBlock = blockSize/extentNodeSize - 1

	extents := (blocks + maxBlocksPerExtent - 1) / maxBlocksPerExtent
	var b bytes.Buffer
	if extents == 0 {
		// Nothing to do.
	} else if extents <= 4 {
		var root struct {
			hdr     format.ExtentHeader
			extents [4]format.ExtentLeafNode
		}
		fillExtents(&root.hdr, root.extents[:extents], startBlock, 0, blocks)
		binary.Write(&b, binary.LittleEndian, root)
	} else if extents <= 4*extentsPerBlock {
		const extentsPerBlock = blockSize/extentNodeSize - 1
		extentBlocks := extents/extentsPerBlock + 1
		usedBlocks += extentBlocks

		var root struct {
			hdr   format.ExtentHeader
			nodes [4]format.ExtentIndexNode
		}
		root.hdr = format.ExtentHeader{
			Magic:   format.ExtentHeaderMagic,
			Entries: uint16(extentBlocks),
			Max:     4,
			Depth:   1,
		}
		for i := uint32(0); i < extentBlocks; i++ {
			root.nodes[i] = format.ExtentIndexNode{
				Block:   i * extentsPerBlock * maxBlocksPerExtent,
				LeafLow: uint32(w.pos / blockSize),
			}
			extentsInBlock := extents - i*extentBlocks
			if extentsInBlock > extentsPerBlock {
				extentsInBlock = extentsPerBlock
			}

			var node struct {
				hdr     format.ExtentHeader
				extents [extentsPerBlock]format.ExtentLeafNode
				_       [blockSize - (extentsPerBlock+1)*extentNodeSize]byte
			}

			offset := i * extentsPerBlock * maxBlocksPerExtent
			fillExtents(&node.hdr, node.extents[:extentsInBlock], startBlock+offset, offset, blocks)
			if err := binary.Write(w.bw, binary.LittleEndian, node); err != nil {
				return err
			}
			w.pos += blockSize
		}
		binary.Write(&b, binary.LittleEndian, root)
	} else {
		panic("file too big")
	}

	inode.Data = b.Bytes()
	inode.Flags |= format.InodeFlagExtents
	inode.BlockCount += usedBlocks
	return nil
}

func (w *Writer) finishInode() error {
	if !w.initialized {
		if err := w.init(); err != nil {
			return err
		}
	}
	if w.curInode == nil {
		return nil
	}
	if w.dataWritten != w.dataMax {
		return fmt.Errorf("did not write the right amount: %d != %d", w.dataWritten, w.dataMax)
	}

	if w.curInode.Flags&format.InodeFlagInlineData == 0 {
		if err := w.writeExtents(w.curInode); err != nil {
			return err
		}
	}

	w.dataWritten = 0
	w.dataMax = 0
	w.curInode = nil
	return nil
}

func modeToFileType(mode uint16) format.FileType {
	switch mode & format.TypeMask {
	default:
		return format.FileTypeUnknown
	case format.S_IFREG:
		return format.FileTypeRegular
	case format.S_IFDIR:
		return format.FileTypeDirectory
	case format.S_IFCHR:
		return format.FileTypeCharacter
	case format.S_IFBLK:
		return format.FileTypeBlock
	case format.S_IFIFO:
		return format.FileTypeFIFO
	case format.S_IFSOCK:
		return format.FileTypeSocket
	case format.S_IFLNK:
		return format.FileTypeSymbolicLink
	}
}

type constReader byte

var zero = constReader(0)

func (r constReader) Read(b []byte) (int, error) {
	for i := range b {
		b[i] = byte(r)
	}
	return len(b), nil
}

func (w *Writer) writeDirectory(dir, parent *inode) error {
	if err := w.finishInode(); err != nil {
		return err
	}

	// The size of the directory is not known yet.
	w.startInode(dir, 0x7fffffffffffffff)
	left := blockSize
	finishBlock := func() error {
		if left > 0 {
			e := format.DirectoryEntry{
				RecordLength: uint16(left),
			}
			err := binary.Write(w, binary.LittleEndian, e)
			if err != nil {
				return err
			}
			left -= directoryEntrySize
			if left < 4 {
				panic("not enough space for trailing entry")
			}
			_, err = io.CopyN(w, zero, int64(left))
			if err != nil {
				return err
			}
		}
		left = blockSize
		return nil
	}

	writeEntry := func(ino format.InodeNumber, name string) error {
		rlb := directoryEntrySize + len(name)
		rl := (rlb + 3) & ^3
		if left < rl+12 {
			if err := finishBlock(); err != nil {
				return err
			}
		}
		e := format.DirectoryEntry{
			Inode:        ino,
			RecordLength: uint16(rl),
			NameLength:   uint8(len(name)),
			FileType:     modeToFileType(w.getInode(ino).Mode),
		}
		err := binary.Write(w, binary.LittleEndian, e)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(name))
		if err != nil {
			return err
		}
		var zero [4]byte
		_, err = w.Write(zero[:rl-rlb])
		if err != nil {
			return err
		}
		left -= rl
		return nil
	}
	if err := writeEntry(dir.Number, "."); err != nil {
		return err
	}
	if err := writeEntry(parent.Number, ".."); err != nil {
		return err
	}

	// Follow e2fsck's convention and sort the children by inode number.
	var children []string
	for name := range dir.Children {
		children = append(children, name)
	}
	sort.Slice(children, func(i, j int) bool {
		return dir.Children[children[i]].Number < dir.Children[children[j]].Number
	})

	for _, name := range children {
		child := dir.Children[name]
		if err := writeEntry(child.Number, name); err != nil {
			return err
		}
	}
	if err := finishBlock(); err != nil {
		return err
	}
	w.curInode.Size = w.dataWritten
	w.dataMax = w.dataWritten
	return nil
}

func (w *Writer) writeDirectoryRecursive(dir, parent *inode) error {
	if err := w.writeDirectory(dir, parent); err != nil {
		return err
	}
	for _, child := range dir.Children {
		if child.IsDir() {
			if err := w.writeDirectoryRecursive(child, dir); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Writer) writeInodeTable(tableSize uint32) error {
	var b bytes.Buffer
	for _, inode := range w.inodes {
		if inode != nil {
			binode := format.Inode{
				Mode:          inode.Mode,
				Uid:           uint16(inode.Uid & 0xffff),
				Gid:           uint16(inode.Gid & 0xffff),
				SizeLow:       uint32(inode.Size & 0xffffffff),
				SizeHigh:      uint32(inode.Size >> 32),
				LinksCount:    uint16(inode.LinkCount), // BUGBUG overflow
				BlocksLow:     inode.BlockCount,
				Flags:         inode.Flags,
				XattrBlockLow: inode.XattrBlock,
				UidHigh:       uint16(inode.Uid >> 16),
				GidHigh:       uint16(inode.Gid >> 16),
				ExtraIsize:    uint16(inodeUsedSize - 128),
				Atime:         uint32(inode.Atime),
				AtimeExtra:    uint32(inode.Atime >> 32),
				Ctime:         uint32(inode.Ctime),
				CtimeExtra:    uint32(inode.Ctime >> 32),
				Mtime:         uint32(inode.Mtime),
				MtimeExtra:    uint32(inode.Mtime >> 32),
				Crtime:        uint32(inode.Crtime),
				CrtimeExtra:   uint32(inode.Crtime >> 32),
			}
			switch inode.Mode & format.TypeMask {
			case format.S_IFDIR, format.S_IFREG, format.S_IFLNK:
				n := copy(binode.Block[:], inode.Data)
				if n < len(inode.Data) {
					// Rewrite the first xattr with the data.
					xattr := [1]xattr{{
						Name:  "data",
						Index: 7, // "system."
						Value: inode.Data[n:],
					}}
					putXattrs(xattr[:], inode.XattrInline[4:], 0)
				}
			case format.S_IFBLK, format.S_IFCHR:
				dev := inode.Devminor&0xff | inode.Devmajor<<8 | (inode.Devminor&0xffffff00)<<12
				binary.LittleEndian.PutUint32(binode.Block[4:], dev)
			}

			binary.Write(&b, binary.LittleEndian, binode)
			b.Truncate(inodeUsedSize)
			n, _ := b.Write(inode.XattrInline)
			io.CopyN(&b, zero, int64(inodeExtraSize-n))
		} else {
			io.CopyN(&b, zero, inodeSize)
		}
		if _, err := w.bw.Write(b.Next(inodeSize)); err != nil {
			return err
		}
	}
	rest := tableSize - uint32(len(w.inodes)*inodeSize)
	if _, err := io.CopyN(w.bw, zero, int64(rest)); err != nil {
		return err
	}
	w.pos += int64(tableSize)
	return nil
}

// NewWriter returns a Writer that writes an ext4 file system to the provided
// WriteSeeker.
func NewWriter(f io.WriteSeeker, opts ...Option) *Writer {
	w := &Writer{
		f:  f,
		bw: bufio.NewWriterSize(f, 65536*8),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// An Option provides extra options to NewWriter.
type Option func(*Writer)

// InlineData instructs the Writer to write small files into the inode
// structures directly. This creates smaller images but currently is not
// compatible with DAX.
func InlineData(w *Writer) {
	w.supportInlineData = true
}

func (w *Writer) init() error {
	// Skip the defective block inode.
	w.inodes = make([]*inode, 1, 32)
	// Create the root directory.
	root, _ := w.makeInode(&File{
		Mode: format.S_IFDIR | 0755,
	}, nil)
	root.LinkCount++ // The root is linked to itself.
	// Skip until the first non-reserved inode.
	w.inodes = append(w.inodes, make([]*inode, inodeFirst-len(w.inodes)-1)...)

	if _, err := w.f.Seek(blockSize*2, io.SeekStart); err != nil {
		return err
	}

	w.pos = blockSize * 2
	w.initialized = true

	// The lost+found directory is required to exist for e2fsck to pass.
	if err := w.Create("lost+found", &File{Mode: format.S_IFDIR | 0700}); err != nil {
		return err
	}
	return nil
}

func groupCount(blocks uint32, inodes uint32, inodesPerGroup uint32) uint32 {
	inodeBlocksPerGroup := inodesPerGroup * inodeSize / blockSize
	dataBlocksPerGroup := blocksPerGroup - inodeBlocksPerGroup - 2 // save room for the bitmaps

	// Increase the block count to ensure there are enough groups for all the
	// inodes.
	minBlocks := (inodes-1)/inodesPerGroup*dataBlocksPerGroup + 1
	if blocks < minBlocks {
		blocks = minBlocks
	}

	return (blocks + dataBlocksPerGroup - 1) / dataBlocksPerGroup
}

func bestGroupCount(blocks uint32, inodes uint32) (groups uint32, inodesPerGroup uint32) {
	groups = 0xffffffff
	for ipg := uint32(inodesPerGroupIncrement); ipg <= maxInodesPerGroup; ipg += inodesPerGroupIncrement {
		g := groupCount(blocks, inodes, ipg)
		if g < groups {
			groups = g
			inodesPerGroup = ipg
		}
	}
	return
}

func (w *Writer) Close() error {
	if err := w.finishInode(); err != nil {
		return err
	}
	root := w.root()
	if err := w.writeDirectoryRecursive(root, root); err != nil {
		return err
	}
	// Finish the last inode (probably a directory).
	if err := w.finishInode(); err != nil {
		return err
	}

	// Write the inode table
	inodeTableOffset := uint32(w.pos / blockSize)
	groups, inodesPerGroup := bestGroupCount(uint32(w.pos/blockSize), uint32(len(w.inodes)))
	err := w.writeInodeTable(groups * inodesPerGroup * inodeSize)
	if err != nil {
		return err
	}

	// Write the bitmaps.
	bitmapOffset := uint32(w.pos / blockSize)
	bitmapSize := groups * 2
	validDataSize := bitmapOffset + bitmapSize
	diskSize := validDataSize
	minSize := (groups-1)*blocksPerGroup + 1
	if diskSize < minSize {
		diskSize = minSize
	}

	gdSize := uint32(binary.Size(format.GroupDescriptor{}))
	if groups > blockSize/gdSize {
		return errors.New("too many groups")
	}

	gds := make([]format.GroupDescriptor, blockSize/gdSize)
	inodeTableSizePerGroup := inodesPerGroup * inodeSize / blockSize
	var totalUsedBlocks, totalUsedInodes uint32
	for g := uint32(0); g < groups; g++ {
		var b [blockSize * 2]byte
		var dirCount, usedInodeCount, usedBlockCount uint16

		// Block bitmap
		if (g+1)*blocksPerGroup <= validDataSize {
			// This group is fully allocated.
			for j := range b[:blockSize] {
				b[j] = 0xff
			}
			usedBlockCount = blocksPerGroup
		} else if g*blocksPerGroup < validDataSize {
			for j := uint32(0); j < validDataSize-g*blocksPerGroup; j++ {
				b[j/8] |= 1 << (j % 8)
				usedBlockCount++
			}
		}
		if g == groups-1 && diskSize%blocksPerGroup != 0 {
			// Blocks that aren't present in the disk should be marked as
			// allocated.
			for j := diskSize % blocksPerGroup; j < blocksPerGroup; j++ {
				b[j/8] |= 1 << (j % 8)
				usedBlockCount++
			}
		}
		// Inode bitmap
		for j := uint32(0); j < inodesPerGroup; j++ {
			ino := format.InodeNumber(1 + g*inodesPerGroup + j)
			inode := w.getInode(ino)
			if ino < inodeFirst || inode != nil {
				b[blockSize+j/8] |= 1 << (j % 8)
				usedInodeCount++
			}
			if inode != nil && inode.Mode&format.TypeMask == format.S_IFDIR {
				dirCount++
			}
		}
		_, err := w.bw.Write(b[:])
		if err != nil {
			return err
		}
		gds[g] = format.GroupDescriptor{
			BlockBitmapLow:     bitmapOffset + 2*g,
			InodeBitmapLow:     bitmapOffset + 2*g + 1,
			InodeTableLow:      inodeTableOffset + g*inodeTableSizePerGroup,
			UsedDirsCountLow:   dirCount,
			FreeInodesCountLow: uint16(inodesPerGroup) - usedInodeCount,
			FreeBlocksCountLow: blocksPerGroup - usedBlockCount,
		}

		totalUsedBlocks += uint32(usedBlockCount)
		totalUsedInodes += uint32(usedInodeCount)
	}

	// Zero up to the disk size.
	_, err = io.CopyN(w.bw, zero, int64(diskSize-bitmapOffset-bitmapSize)*blockSize)
	if err != nil {
		return err
	}
	w.pos = int64(diskSize) * blockSize

	// Write the block descriptors
	err = w.bw.Flush()
	if err != nil {
		return err
	}
	_, err = w.f.Seek(blockSize, io.SeekStart)
	if err != nil {
		return err
	}
	err = binary.Write(w.bw, binary.LittleEndian, gds)
	if err != nil {
		return err
	}

	// Write the super block
	err = w.bw.Flush()
	if err != nil {
		return err
	}
	_, err = w.f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	if _, err := io.CopyN(w.bw, zero, 1024); err != nil {
		return err
	}
	sb := &format.SuperBlock{
		InodesCount:        inodesPerGroup * groups,
		BlocksCountLow:     diskSize,
		FreeBlocksCountLow: blocksPerGroup*groups - totalUsedBlocks,
		FreeInodesCount:    inodesPerGroup*groups - totalUsedInodes,
		FirstDataBlock:     0,
		LogBlockSize:       2, // 2^(10 + 2)
		LogClusterSize:     2,
		BlocksPerGroup:     blocksPerGroup,
		ClustersPerGroup:   blocksPerGroup,
		InodesPerGroup:     inodesPerGroup,
		Magic:              format.SuperBlockMagic,
		State:              1, // cleanly unmounted
		Errors:             1, // continue on error?
		CreatorOS:          0, // Linux
		RevisionLevel:      1, // dynamic inode sizes
		FirstInode:         inodeFirst,
		LpfInode:           inodeLostAndFound,
		InodeSize:          inodeSize,
		FeatureCompat:      format.CompatSparseSuper2 | format.CompatExtAttr,
		FeatureIncompat:    format.IncompatFiletype | format.IncompatExtents | format.IncompatFlexBg,
		FeatureRoCompat:    format.RoCompatLargeFile | format.RoCompatHugeFile | format.RoCompatExtraIsize | format.RoCompatReadonly,
		MinExtraIsize:      extraIsize,
		WantExtraIsize:     extraIsize,
		LogGroupsPerFlex:   31,
	}
	if w.supportInlineData {
		sb.FeatureIncompat |= format.IncompatInlineData
	}
	if err := binary.Write(w.bw, binary.LittleEndian, sb); err != nil {
		return err
	}
	if _, err := io.CopyN(w.bw, zero, int64(blockSize-1024-binary.Size(sb))); err != nil {
		return err
	}

	return w.bw.Flush()
}
