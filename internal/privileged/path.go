package privileged

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/winapi"
	"golang.org/x/sys/windows"
)

// Checks the sandbox volume and PATH env variable to find application name. If the path
// is relative, E.g. path\to\binary, it will be searched for in the sandbox volume. If it
// is solely an application name then the base directory of the sandbox volume and all directories
// in PATH will be searched. Privileged containers can run anything on the host so C:\path\to\exe,
// D:\path\to\exe etc are just passed through as is.
func findExecutable(path string, imagePath string) (string, error) {
	// Absolute path, just return the path. User specified (hopefully)
	// a path to an executable on the host. C:\path\to\binary.exe
	if filepath.IsAbs(path) {
		return path, nil
	}
	// Not an absolute path, if no path seperators search in image path and if this fails
	// search in PATH. If both of these fail then we error out. Otherwise if there are path
	// seperators treat this as the user is trying to run an executable from the image directory
	// and append the directory to the path supplied.
	// E.g. path\to\binary.exe ---> C:\path\to\sandbox\ + path\to\binary.exe.
	if filepath.Base(path) == path {
		// User specified just the application name E.g. name_of_binary or name_of_binary.exe
		// Check payload directory first and if this fails check PATH.
		if absPath, err := searchPath(imagePath, path); err == nil {
			return absPath, nil
		}

		// Error in searching in the payload path, try PATH.
		if absPath, err := exec.LookPath(path); err == nil {
			return absPath, nil
		}
	} else {
		// This is a relative path E.g path\to\binary.exe. Append the image directory to
		// it and hope it's there.
		absPath := filepath.Join(imagePath, path)
		if _, err := os.Stat(absPath); err != nil {
			return "", err
		}
		return absPath, nil
	}
	return "", errors.New("failed to find executable on the system")
}

// searchPath is just a wrapper around SearchPathW. Checks a path for the executable
// specified.
func searchPath(hostPath, fileName string) (string, error) {
	size, err := winapi.SearchPath(
		windows.StringToUTF16Ptr(hostPath),
		windows.StringToUTF16Ptr(fileName),
		windows.StringToUTF16Ptr(".exe"),
		0,
		nil,
		nil,
	)
	if err != nil {
		return "", err
	}
	lpBuffer := make([]uint16, size)
	size, err = winapi.SearchPath(
		windows.StringToUTF16Ptr(hostPath),
		windows.StringToUTF16Ptr(fileName),
		windows.StringToUTF16Ptr(".exe"),
		size,
		&lpBuffer[0],
		nil,
	)
	if err != nil {
		return "", err
	}
	return windows.UTF16ToString(lpBuffer), nil
}
