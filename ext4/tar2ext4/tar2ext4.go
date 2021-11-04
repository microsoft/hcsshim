package tar2ext4

import (
	"archive/tar"
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"unsafe"

	"github.com/pkg/errors"

	"github.com/Microsoft/hcsshim/ext4/dmverity"
	"github.com/Microsoft/hcsshim/ext4/internal/compactext4"
	"github.com/Microsoft/hcsshim/ext4/internal/format"
)

type params struct {
	convertWhiteout bool
	appendVhdFooter bool
	appendDMVerity  bool
	ext4opts        []compactext4.Option
}

// Option is the type for optional parameters to Convert.
type Option func(*params)

// ConvertWhiteout instructs the converter to convert OCI-style whiteouts
// (beginning with .wh.) to overlay-style whiteouts.
func ConvertWhiteout(p *params) {
	p.convertWhiteout = true
}

// AppendVhdFooter instructs the converter to add a fixed VHD footer to the
// file.
func AppendVhdFooter(p *params) {
	p.appendVhdFooter = true
}

// AppendDMVerity instructs the converter to add a dmverity merkle tree for
// the ext4 filesystem after the filesystem and before the optional VHD footer
func AppendDMVerity(p *params) {
	p.appendDMVerity = true
}

// InlineData instructs the converter to write small files into the inode
// structures directly. This creates smaller images but currently is not
// compatible with DAX.
func InlineData(p *params) {
	p.ext4opts = append(p.ext4opts, compactext4.InlineData)
}

// MaximumDiskSize instructs the writer to limit the disk size to the specified
// value. This also reserves enough metadata space for the specified disk size.
// If not provided, then 16GB is the default.
func MaximumDiskSize(size int64) Option {
	return func(p *params) {
		p.ext4opts = append(p.ext4opts, compactext4.MaximumDiskSize(size))
	}
}

const (
	whiteoutPrefix = ".wh."
	opaqueWhiteout = ".wh..wh..opq"
	ext4BlockSize  = compactext4.BlockSize
)

// ConvertTarToExt4 writes a compact ext4 file system image that contains the files in the
// input tar stream.
func ConvertTarToExt4(r io.Reader, w io.ReadWriteSeeker, options ...Option) error {
	var p params
	for _, opt := range options {
		opt(&p)
	}

	t := tar.NewReader(bufio.NewReader(r))
	fs := compactext4.NewWriter(w, p.ext4opts...)
	for {
		hdr, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err = fs.MakeParents(hdr.Name); err != nil {
			return errors.Wrapf(err, "failed to ensure parent directories for %s", hdr.Name)
		}

		if p.convertWhiteout {
			dir, name := path.Split(hdr.Name)
			if strings.HasPrefix(name, whiteoutPrefix) {
				if name == opaqueWhiteout {
					// Update the directory with the appropriate xattr.
					f, err := fs.Stat(dir)
					if err != nil {
						return errors.Wrapf(err, "failed to stat parent directory of whiteout %s", hdr.Name)
					}
					f.Xattrs["trusted.overlay.opaque"] = []byte("y")
					err = fs.Create(dir, f)
					if err != nil {
						return errors.Wrapf(err, "failed to create opaque dir %s", hdr.Name)
					}
				} else {
					// Create an overlay-style whiteout.
					f := &compactext4.File{
						Mode:     compactext4.S_IFCHR,
						Devmajor: 0,
						Devminor: 0,
					}
					err = fs.Create(path.Join(dir, name[len(whiteoutPrefix):]), f)
					if err != nil {
						return errors.Wrapf(err, "failed to create whiteout file for %s", hdr.Name)
					}
				}

				continue
			}
		}

		if hdr.Typeflag == tar.TypeLink {
			err = fs.Link(hdr.Linkname, hdr.Name)
			if err != nil {
				return err
			}
		} else {
			f := &compactext4.File{
				Mode:     uint16(hdr.Mode),
				Atime:    hdr.AccessTime,
				Mtime:    hdr.ModTime,
				Ctime:    hdr.ChangeTime,
				Crtime:   hdr.ModTime,
				Size:     hdr.Size,
				Uid:      uint32(hdr.Uid),
				Gid:      uint32(hdr.Gid),
				Linkname: hdr.Linkname,
				Devmajor: uint32(hdr.Devmajor),
				Devminor: uint32(hdr.Devminor),
				Xattrs:   make(map[string][]byte),
			}
			for key, value := range hdr.PAXRecords {
				const xattrPrefix = "SCHILY.xattr."
				if strings.HasPrefix(key, xattrPrefix) {
					f.Xattrs[key[len(xattrPrefix):]] = []byte(value)
				}
			}

			var typ uint16
			switch hdr.Typeflag {
			case tar.TypeReg, tar.TypeRegA:
				typ = compactext4.S_IFREG
			case tar.TypeSymlink:
				typ = compactext4.S_IFLNK
			case tar.TypeChar:
				typ = compactext4.S_IFCHR
			case tar.TypeBlock:
				typ = compactext4.S_IFBLK
			case tar.TypeDir:
				typ = compactext4.S_IFDIR
			case tar.TypeFifo:
				typ = compactext4.S_IFIFO
			}
			f.Mode &= ^compactext4.TypeMask
			f.Mode |= typ
			err = fs.Create(hdr.Name, f)
			if err != nil {
				return err
			}
			_, err = io.Copy(fs, t)
			if err != nil {
				return err
			}
		}
	}
	return fs.Close()
}

// Convert wraps ConvertTarToExt4 and conditionally computes (and appends) the file image's cryptographic
// hashes (merkle tree) or/and appends a VHD footer.
func Convert(r io.Reader, w io.ReadWriteSeeker, options ...Option) error {
	var p params
	for _, opt := range options {
		opt(&p)
	}

	if err := ConvertTarToExt4(r, w, options...); err != nil {
		return err
	}

	if p.appendDMVerity {
		// Rewind the stream for dm-verity processing
		if _, err := w.Seek(0, io.SeekStart); err != nil {
			return err
		}

		merkleTree, err := dmverity.MerkleTree(bufio.NewReaderSize(w, dmverity.MerkleTreeBufioSize))
		if err != nil {
			return errors.Wrap(err, "failed to build merkle tree")
		}

		// Write dm-verity super-block and then the merkle tree after the end of the
		// ext4 filesystem
		ext4size, err := w.Seek(0, io.SeekEnd)
		if err != nil {
			return err
		}

		superBlock := dmverity.NewDMVeritySuperblock(uint64(ext4size))
		if err = binary.Write(w, binary.LittleEndian, superBlock); err != nil {
			return err
		}

		// pad the super-block
		sbsize := int(unsafe.Sizeof(*superBlock))
		padding := bytes.Repeat([]byte{0}, ext4BlockSize-(sbsize%ext4BlockSize))
		if _, err = w.Write(padding); err != nil {
			return err
		}

		// write the tree
		if _, err = w.Write(merkleTree); err != nil {
			return err
		}
	}

	if p.appendVhdFooter {
		size, err := w.Seek(0, io.SeekEnd)
		if err != nil {
			return err
		}
		err = binary.Write(w, binary.BigEndian, makeFixedVHDFooter(size))
		if err != nil {
			return err
		}
	}
	return nil
}

// ReadExt4SuperBlock reads and returns ext4 super block from VHD
//
// The layout on disk is as follows:
// | Group 0 padding     | - 1024 bytes
// | ext4 SuperBlock     | - 1 block
// | Group Descriptors   | - many blocks
// | Reserved GDT Blocks | - many blocks
// | Data Block Bitmap   | - 1 block
// | inode Bitmap        | - 1 block
// | inode Table         | - many blocks
// | Data Blocks         | - many blocks
//
// More details can be found here https://ext4.wiki.kernel.org/index.php/Ext4_Disk_Layout
//
// Our goal is to skip the Group 0 padding, read and return the ext4 SuperBlock
func ReadExt4SuperBlock(vhdPath string) (*format.SuperBlock, error) {
	vhd, err := os.OpenFile(vhdPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer vhd.Close()

	// Skip padding at the start
	if _, err := vhd.Seek(1024, io.SeekStart); err != nil {
		return nil, err
	}
	var sb format.SuperBlock
	if err := binary.Read(vhd, binary.LittleEndian, &sb); err != nil {
		return nil, err
	}
	return &sb, nil
}

// ConvertAndComputeRootDigest writes a compact ext4 file system image that contains the files in the
// input tar stream, computes the resulting file image's cryptographic hashes (merkle tree) and returns
// merkle tree root digest. Convert is called with minimal options: ConvertWhiteout and MaximumDiskSize
// set to dmverity.RecommendedVHDSizeGB.
func ConvertAndComputeRootDigest(r io.Reader) (string, error) {
	out, err := ioutil.TempFile("", "")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %s", err)
	}
	defer func() {
		_ = os.Remove(out.Name())
	}()

	options := []Option{
		ConvertWhiteout,
		MaximumDiskSize(dmverity.RecommendedVHDSizeGB),
	}
	if err := ConvertTarToExt4(r, out, options...); err != nil {
		return "", fmt.Errorf("failed to convert tar to ext4: %s", err)
	}

	if _, err := out.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("failed to seek start on temp file when creating merkle tree: %s", err)
	}

	tree, err := dmverity.MerkleTree(bufio.NewReaderSize(out, dmverity.MerkleTreeBufioSize))
	if err != nil {
		return "", fmt.Errorf("failed to create merkle tree: %s", err)
	}

	hash := dmverity.RootHash(tree)
	return fmt.Sprintf("%x", hash), nil
}
