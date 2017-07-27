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

	// No error
	if len(b) == 0 {
		return nil
	}

	var raw json.RawMessage
	exportedErr := &ExportedError{
		Err: &raw,
	}
	if err := json.Unmarshal(b, exportedErr); err != nil {
		return err
	}

	switch exportedErr.ErrType {
	case PathError:
		err = &os.PathError{}
	case LinkError:
		err = &os.LinkError{}
	case SyscallError:
		err = &os.SyscallError{}
	case GenericErrorString:
		err = &ErrorString{}
	default:
		err = nil
	}

	// If none of the cases match, then json was invalid
	if err == nil {
		return ErrInvalid
	}

	if err1 := json.Unmarshal([]byte(raw), err); err1 != nil {
		return err1
	}
	return err
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
	if err == nil {
		return nil
	}

	err = fixOSError(err)

	var errType string
	switch err.(type) {
	case *os.PathError:
		errType = PathError
	case *os.LinkError:
		errType = LinkError
	case *os.SyscallError:
		errType = SyscallError
	default:
		// We wrap the error in the ErrorString struct because the underlying
		// struct that implements the error might not have been exported. For
		// example, the errors created from errors.New and fmt.Errorf don't
		// export the struct, so json.Marshal will ignore it.
		errType = GenericErrorString
		err = &ErrorString{err.Error()}
	}

	exportedError := &ExportedError{
		ErrType: errType,
		Err:     err,
	}

	b, err1 := json.Marshal(exportedError)
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
