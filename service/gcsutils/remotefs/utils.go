package remotefs

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"

	"io/ioutil"

	"github.com/docker/docker/pkg/archive"
)

// ReadError is an utility function that reads a serialized error from the given reader
// and deserializes it.
func ReadError(in io.Reader) error {
	b, err := ioutil.ReadAll(in)
	if err != nil {
		return err
	}

	var exportedErr ExportedError
	if err := json.Unmarshal(b, &exportedErr); err != nil {
		return err
	}

	switch exportedErr.ErrType {
	case PathError:
		return exportedErr.PathError
	case LinkError:
		return exportedErr.LinkError
	case SyscallError:
		return exportedErr.SyscallError
	case GenericErrorString:
		return fixStringError(exportedErr.ErrorString)
	}
	// If none of the cases match, then the remotefs process succeeded, so return nil
	return nil
}

// fixStringError fixes some of the string errors returned by the remote fs, so that they
// can be compared with the errors in docker. For example, if the error returned has the
// same string as os.ErrExist, the function will return os.ErrExist.
func fixStringError(err *ErrorString) error {
	// Since Go will compare the pointers instead of the value, we compare the strings.
	if err.Error() == os.ErrExist.Error() {
		return os.ErrExist
	} else if err.Error() == os.ErrNotExist.Error() {
		return os.ErrNotExist
	} else if err.Error() == os.ErrPermission.Error() {
		return os.ErrPermission
	}
	return err
}

// WriteError is an utility function that serializes the error
// and writes it to the output writer.
func WriteError(err error, out io.Writer) error {
	err = fixOSError(err)

	var exportedErr = &ExportedError{}
	switch errWithType := err.(type) {
	case *os.PathError:
		exportedErr.ErrType = PathError
		exportedErr.PathError = errWithType
	case *os.LinkError:
		exportedErr.ErrType = LinkError
		exportedErr.LinkError = errWithType
	case *os.SyscallError:
		exportedErr.ErrType = SyscallError
		exportedErr.SyscallError = errWithType
	default:
		// We wrap the error in the ErrorString struct because the underlying
		// struct that implements the error might not have been exported. For
		// example, the errors created from errors.New and fmt.Errorf don't
		// export the struct, so json.Marshal will ignore it.
		exportedErr.ErrType = GenericErrorString
		exportedErr.ErrorString = &ErrorString{err.Error()}
	}

	b, err1 := json.Marshal(exportedErr)
	if err1 != nil {
		return err1
	}

	_, err1 = out.Write(b)
	if err1 != nil {
		return err1
	}
	return nil
}

// fixOSError converts possible platform dependent error into the portable errors in the
// Go os package if possible.
func fixOSError(err error) error {
	// The os.IsExist, os.IsNotExist, and os.IsPermissions functions are platform
	// dependent, so sending the raw error might break those functions on a different OS.
	// Go defines portable errors for these.
	if os.IsExist(err) {
		return os.ErrExist
	} else if os.IsNotExist(err) {
		return os.ErrNotExist
	} else if os.IsPermission(err) {
		return os.ErrPermission
	}
	return err
}

// ReadTarOptions reads from the specified reader and deserializes an archive.TarOptions struct.
func ReadTarOptions(r io.Reader) (*archive.TarOptions, error) {
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
	return &opts, nil
}

// WriteTarOptions serializes a archive.TarOptions struct and writes it to the writer.
func WriteTarOptions(w io.Writer, opts *archive.TarOptions) error {
	optsBuf, err := json.Marshal(opts)
	if err != nil {
		return err
	}

	optsSize := uint64(len(optsBuf))
	optsSizeBuf := &bytes.Buffer{}
	if err := binary.Write(optsSizeBuf, binary.BigEndian, optsSize); err != nil {
		return err
	}

	if _, err := optsSizeBuf.WriteTo(w); err != nil {
		return err
	}

	if _, err := w.Write(optsBuf); err != nil {
		return err
	}

	return nil
}
