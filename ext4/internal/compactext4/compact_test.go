package compactext4

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/ext4/internal/format"
)

type testFile struct {
	Path string
	File *File
	Data []byte
	Link string
}

func createTestFile(t *testing.T, w *Writer, tf testFile) {
	if tf.File != nil {
		tf.File.Size = int64(len(tf.Data))
		err := w.Create(tf.Path, tf.File)
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(tf.Data)
		if err != nil {
			t.Fatal(err)
		}
	} else if tf.Link != "" {
		err := w.Link(tf.Link, tf.Path)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreateFs(t *testing.T) {
	data := make([]byte, blockSize*2)
	for i := range data {
		data[i] = uint8(i)
	}

	nameb := make([]byte, 300)
	for i := range nameb {
		nameb[i] = byte('0' + i%10)
	}
	name := string(nameb)

	now := time.Now()
	testFiles := []testFile{
		{Path: "empty", File: &File{Mode: 0644}},
		{Path: "time", File: &File{Atime: now, Ctime: now.Add(time.Second), Mtime: now.Add(time.Hour)}},
		{Path: "inline_30", File: &File{Mode: 0644}, Data: data[:30]},
		{Path: "inline_60", File: &File{Mode: 0644}, Data: data[:60]},
		{Path: "inline_120", File: &File{Mode: 0644}, Data: data[:120]},
		{Path: "inline_full", File: &File{Mode: 0644}, Data: data[:inlineDataSize]},
		{Path: "block_min", File: &File{Mode: 0644}, Data: data[:inlineDataSize+1]},
		{Path: "block_1", File: &File{Mode: 0644}, Data: data[:blockSize]},
		{Path: "block_2", File: &File{Mode: 0644}, Data: data[:blockSize*2]},
		{Path: "symlink", File: &File{Linkname: "block_1", Mode: format.S_IFLNK}},
		{Path: "symlink_59", File: &File{Linkname: name[:59], Mode: format.S_IFLNK}},
		{Path: "symlink_60", File: &File{Linkname: name[:60], Mode: format.S_IFLNK}},
		{Path: "symlink_120", File: &File{Linkname: name[:120], Mode: format.S_IFLNK}},
		{Path: "symlink_300", File: &File{Linkname: name[:300], Mode: format.S_IFLNK}},
		{Path: "dir", File: &File{Mode: format.S_IFDIR | 0755}},
		{Path: "dir/fifo", File: &File{Mode: format.S_IFIFO}},
		{Path: "dir/sock", File: &File{Mode: format.S_IFSOCK}},
		{Path: "dir/blk", File: &File{Mode: format.S_IFBLK, Devmajor: 0x5678, Devminor: 0x1234}},
		{Path: "dir/chr", File: &File{Mode: format.S_IFCHR, Devmajor: 0x5678, Devminor: 0x1234}},
		{Path: "dir/hard_link", Link: "block_min"},
		{Path: "bigdir", File: &File{Mode: format.S_IFDIR | 0755}},
		{Path: "withsmallxattrs",
			File: &File{
				Mode: format.S_IFREG | 0644,
				Xattrs: map[string][]byte{
					"user.foo": []byte("test"),
					"user.bar": []byte("test2"),
				},
			},
		},
		{Path: "withlargexattrs",
			File: &File{
				Mode: format.S_IFREG | 0644,
				Xattrs: map[string][]byte{
					"user.foo": data[:100],
					"user.bar": data[:50],
				},
			},
		},
	}

	image := "testfs.img"
	imagef, err := os.Create(image)
	if err != nil {
		t.Fatal(err)
	}
	defer imagef.Close()
	w := NewWriter(imagef)

	for _, tf := range testFiles {
		createTestFile(t, w, tf)
	}

	for i := 0; i < 50000; i++ {
		createTestFile(t, w, testFile{Path: fmt.Sprintf("bigdir/%d", i), File: &File{Mode: 0644}})
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	fsck(t, image)

	mountPath := "testmnt"

	if mountImage(t, image, mountPath) {
		defer unmountImage(t, mountPath)
		for _, tf := range testFiles {
			verifyTestFile(t, mountPath, tf)
		}
		for i := 0; i < 50000; i++ {
			verifyTestFile(t, mountPath, testFile{Path: fmt.Sprintf("bigdir/%d", i), File: &File{Mode: 0644}})
		}
	}
}
