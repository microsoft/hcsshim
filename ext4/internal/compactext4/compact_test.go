package compactext4

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/ext4/internal/format"
)

type testFile struct {
	Path        string
	File        *File
	Data        []byte
	Link        string
	ExpectError bool
}

const ()

var (
	data []byte
	name string
)

func init() {
	data = make([]byte, blockSize*2)
	for i := range data {
		data[i] = uint8(i)
	}

	nameb := make([]byte, 300)
	for i := range nameb {
		nameb[i] = byte('0' + i%10)
	}
	name = string(nameb)
}

func createTestFile(t *testing.T, w *Writer, tf testFile) {
	var err error
	if tf.File != nil {
		tf.File.Size = int64(len(tf.Data))
		err = w.Create(tf.Path, tf.File)
	} else {
		err = w.Link(tf.Link, tf.Path)
	}
	if tf.ExpectError && err == nil {
		t.Fatalf("%s: expected error", tf.Path)
	}
	if !tf.ExpectError && err != nil {
		t.Fatal(err)
	}
	if tf.File != nil {
		_, err = w.Write(tf.Data)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func runTestsOnFiles(t *testing.T, testFiles []testFile, opts ...Option) {
	image := "testfs.img"
	imagef, err := os.Create(image)
	if err != nil {
		t.Fatal(err)
	}
	defer imagef.Close()
	defer os.Remove(image)

	w := NewWriter(imagef, opts...)
	for _, tf := range testFiles {
		createTestFile(t, w, tf)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	fsck(t, image)

	mountPath := "testmnt"

	if mountImage(t, image, mountPath) {
		defer unmountImage(t, mountPath)
		validated := make(map[string]*testFile)
		for i := range testFiles {
			tf := testFiles[len(testFiles)-i-1]
			if validated[tf.Link] != nil {
				// The link target was subsequently replaced. Find the
				// earlier instance.
				for j := range testFiles[:len(testFiles)-i-1] {
					otf := testFiles[j]
					if otf.Path == tf.Link && !otf.ExpectError {
						tf = otf
						break
					}
				}
			}
			if !tf.ExpectError && validated[tf.Path] == nil {
				verifyTestFile(t, mountPath, tf)
				validated[tf.Path] = &tf
			}
		}
	}
}

func TestBasic(t *testing.T) {
	now := time.Now()
	testFiles := []testFile{
		{Path: "empty", File: &File{Mode: 0644}},
		{Path: "small", File: &File{Mode: 0644}, Data: data[:40]},
		{Path: "time", File: &File{Atime: now, Ctime: now.Add(time.Second), Mtime: now.Add(time.Hour)}},
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
		{Path: "dir/hard_link", Link: "small"},
	}

	runTestsOnFiles(t, testFiles)
}

func TestLargeDirectory(t *testing.T) {
	testFiles := []testFile{
		{Path: "bigdir", File: &File{Mode: format.S_IFDIR | 0755}},
	}
	for i := 0; i < 50000; i++ {
		testFiles = append(testFiles, testFile{
			Path: fmt.Sprintf("bigdir/%d", i), File: &File{Mode: 0644},
		})
	}

	runTestsOnFiles(t, testFiles)
}

func TestInlineData(t *testing.T) {
	testFiles := []testFile{
		{Path: "inline_30", File: &File{Mode: 0644}, Data: data[:30]},
		{Path: "inline_60", File: &File{Mode: 0644}, Data: data[:60]},
		{Path: "inline_120", File: &File{Mode: 0644}, Data: data[:120]},
		{Path: "inline_full", File: &File{Mode: 0644}, Data: data[:inlineDataSize]},
		{Path: "block_min", File: &File{Mode: 0644}, Data: data[:inlineDataSize+1]},
	}

	runTestsOnFiles(t, testFiles, InlineData)
}

func TestXattrs(t *testing.T) {
	testFiles := []testFile{
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
	runTestsOnFiles(t, testFiles)
}

func TestReplace(t *testing.T) {
	testFiles := []testFile{
		{Path: "lost+found", ExpectError: true, File: &File{}}, // can't change type
		{Path: "lost+found", File: &File{Mode: format.S_IFDIR | 0777}},

		{Path: "dir", File: &File{Mode: format.S_IFDIR | 0777}},
		{Path: "dir/file", File: &File{}},
		{Path: "dir", File: &File{Mode: format.S_IFDIR | 0700}},

		{Path: "file", File: &File{}},
		{Path: "file", File: &File{Mode: 0600}},
		{Path: "file2", File: &File{}},
		{Path: "link", Link: "file2"},
		{Path: "file2", File: &File{Mode: 0600}},

		{Path: "nolinks", File: &File{}},
		{Path: "nolinks", ExpectError: true, Link: "file"}, // would orphan nolinks

		{Path: "onelink", File: &File{}},
		{Path: "onelink2", Link: "onelink"},
		{Path: "onelink", Link: "file"},

		{Path: "", ExpectError: true, File: &File{}},
		{Path: "", ExpectError: true, Link: "file"},
		{Path: "", File: &File{Mode: format.S_IFDIR | 0777}},
	}
	runTestsOnFiles(t, testFiles)
}

func TestFileLinkLimit(t *testing.T) {
	testFiles := []testFile{
		{Path: "file", File: &File{}},
	}
	for i := 0; i < format.MaxLinks; i++ {
		testFiles = append(testFiles, testFile{Path: fmt.Sprintf("link%d", i), Link: "file"})
	}
	testFiles[len(testFiles)-1].ExpectError = true
	runTestsOnFiles(t, testFiles)
}

func TestDirLinkLimit(t *testing.T) {
	testFiles := []testFile{
		{Path: "dir", File: &File{Mode: S_IFDIR}},
	}
	for i := 0; i < format.MaxLinks-1; i++ {
		testFiles = append(testFiles, testFile{Path: fmt.Sprintf("dir/%d", i), File: &File{Mode: S_IFDIR}})
	}
	testFiles[len(testFiles)-1].ExpectError = true
	runTestsOnFiles(t, testFiles)
}
