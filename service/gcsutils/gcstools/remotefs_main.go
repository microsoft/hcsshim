package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"github.com/Microsoft/opengcs/service/gcsutils/gcstools/commoncli"
	"github.com/Microsoft/opengcs/service/libs/commonutils"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/symlink"
)

var UnknownCommandErr = errors.New("unkown command")
var InvalidParams = errors.New("invalid params")

type fileinfo struct {
	NameVar    string
	SizeVar    int64
	ModeVar    os.FileMode
	ModTimeVar time.Time
	IsDirVar   bool
}

func (f *fileinfo) Name() string       { return f.NameVar }
func (f *fileinfo) Size() int64        { return f.SizeVar }
func (f *fileinfo) Mode() os.FileMode  { return f.ModeVar }
func (f *fileinfo) ModTime() time.Time { return f.ModTimeVar }
func (f *fileinfo) IsDir() bool        { return f.IsDirVar }
func (f *fileinfo) Sys() interface{}   { return nil }

func writeStat(fi os.FileInfo, w io.Writer) error {
	info := &fileinfo{
		NameVar:    fi.Name(),
		SizeVar:    fi.Size(),
		ModeVar:    fi.Mode(),
		ModTimeVar: fi.ModTime(),
		IsDirVar:   fi.IsDir(),
	}

	buf, err := json.Marshal(info)
	if err != nil {
		return err
	}

	if _, err := w.Write(buf); err != nil {
		return err
	}

	return nil
}

func getTarOpts(r io.Reader) (*archive.TarOptions, error) {
	var size uint64
	if err := binary.Read(os.Stdin, binary.BigEndian, &size); err != nil {
		return nil, err
	}

	rawJSON := make([]byte, size)
	if _, err := io.ReadFull(os.Stdin, rawJSON); err != nil {
		return nil, err
	}

	var opts archive.TarOptions
	if err := json.Unmarshal(rawJSON, &opts); err != nil {
		return nil, err
	}

	// Write the file for testing.
	b := &bytes.Buffer{}
	if err := binary.Write(b, binary.BigEndian, size); err != nil {
		return nil, err
	}

	if _, err := b.Write(rawJSON); err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile("/tmp/test.data", b.Bytes(), 0644); err != nil {
		return nil, err
	}

	return &opts, nil
}

func stat() error {
	if len(os.Args) < 3 {
		return InvalidParams
	}

	fi, err := os.Stat(os.Args[2])
	if err != nil {
		return err
	}

	if err := writeStat(fi, os.Stdout); err != nil {
		return err
	}
	return nil
}

func lstat() error {
	if len(os.Args) < 3 {
		return InvalidParams
	}

	fi, err := os.Lstat(os.Args[2])
	if err != nil {
		return err
	}

	if err := writeStat(fi, os.Stdout); err != nil {
		return err
	}
	return nil
}

func resolvePath() error {
	if len(os.Args) < 4 {
		return InvalidParams
	}
	res, err := symlink.FollowSymlinkInScope(os.Args[2], os.Args[3])
	if err != nil {
		return err
	}
	if _, err = os.Stdout.Write([]byte(res)); err != nil {
		return err
	}
	return nil
}

func extractArchive() error {
	if len(os.Args) < 3 {
		return InvalidParams
	}

	utils.LogMsg("extrachArchive(): Getting opts struct from stdin\n")
	opts, err := getTarOpts(os.Stdin)
	if err != nil {
		utils.LogMsgf("extractArchive(): Failed to get opts struct: %s\n", err)
		return err
	}

	utils.LogMsgf("extractArchive(): Untar %s with opts %+v\n", os.Args[2], opts)
	if err := archive.Untar(os.Stdin, os.Args[2], opts); err != nil {
		utils.LogMsgf("extractArchive(): failed to untar: %s\n", err)
		return err
	}

	utils.LogMsgf("extractArchive(): sucess!!!\n")
	return nil
}

func archivePath() error {
	if len(os.Args) < 3 {
		return InvalidParams
	}

	utils.LogMsg("archivePath(): Getting opts struct from stdin\n")
	opts, err := getTarOpts(os.Stdin)
	if err != nil {
		utils.LogMsgf("archivePath(): unmarshal failure: %s\n", err)
		return err
	}

	utils.LogMsgf("archivePath(): Path=%s Tar=%+v\n", os.Args[2], opts)
	r, err := archive.TarWithOptions(os.Args[2], opts)
	if err != nil {
		utils.LogMsgf("archivePath(): tar on path %s failed: %s", os.Args[2], err)
		return err
	}

	utils.LogMsg("archivePath(): Copying reader to stdout")
	if _, err := io.Copy(os.Stdout, r); err != nil {
		utils.LogMsgf("archivePath(): Copying teader failed: %s\n", err)
		return err
	}

	utils.LogMsg("archivePath(): success!!!\n")
	return nil
}

func mkdirall() error {
	if len(os.Args) < 4 {
		return InvalidParams
	}

	perm, err := strconv.ParseUint(os.Args[3], 8, 32)
	if err != nil {
		return err
	}
	return os.MkdirAll(os.Args[2], os.FileMode(perm))
}

func remotefs() error {
	logArgs := commoncli.SetFlagsForLogging()
	flag.Parse()

	err := commoncli.SetupLogging(logArgs...)
	if err != nil {
		return err
	}

	if len(os.Args) < 2 {
		return UnknownCommandErr
	}

	command := os.Args[1]

	utils.LogMsgf("remotefs(): got command %s\n", command)
	switch command {
	case "stat":
		return stat()
	case "lstat":
		return lstat()
	case "resolvepath":
		return resolvePath()
	case "archivepath":
		return archivePath()
	case "extractarchive":
		return extractArchive()
	case "mkdirall":
		return mkdirall()
	}
	return UnknownCommandErr
}

func remotefsMain() {
	if err := remotefs(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
