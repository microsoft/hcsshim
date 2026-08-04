//go:build windows

package vmutils

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultLCOWOSBootFilesPath returns the default path used to locate the LCOW
// OS kernel and root FS files. This default is the subdirectory
// `LinuxBootFiles` in the directory of the executable that started the current
// process; or, if it does not exist, `%ProgramFiles%\Linux Containers`.
func DefaultLCOWOSBootFilesPath() string {
	localDirPath := filepath.Join(filepath.Dir(os.Args[0]), "LinuxBootFiles")
	if _, err := os.Stat(localDirPath); err == nil {
		return localDirPath
	}
	return filepath.Join(os.Getenv("ProgramFiles"), "Linux Containers")
}

// LinuxLogForwarderCommand is the guest command that streams GCS stdout/stderr to the
// host log port. In reconnect mode it re-dials the host listener so logs survive migration.
func LinuxLogForwarderCommand(reconnect bool) string {
	if reconnect {
		return fmt.Sprintf("/bin/vsockexec -r -e %d", LinuxLogVsockPort)
	}
	return fmt.Sprintf("/bin/vsockexec -e %d", LinuxLogVsockPort)
}
