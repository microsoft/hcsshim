//go:build windows

package jobcontainers

import (
	"fmt"
	"os"
	"strings"

	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// This file emulates the path resolution logic that is used for launching regular
// process and hypervisor isolated Windows containers.

// getApplicationName resolves a given command line string and returns the path to the executable that should be launched, and
// an adjusted commandline if needed. The resolution logic may appear overcomplicated but is designed to match the logic used by
// Windows Server containers, as well as that used by CreateProcess (see notes for the lpApplicationName parameter).
//
// The logic follows this set of steps:
//
//   - Construct a list of searchable paths to find the application. This includes the standard Windows system paths
//     which are generally located at C:\Windows, C:\Windows\System32 and C:\Windows\System. If a working directory or path is specified
//     via the `workingDirectory` or `pathEnv` parameters then these will be appended to the paths to search from as well. The
//     searching logic is handled by the Windows API function `SearchPathW` which accepts a semicolon separated list of paths to search
//     in.
//     https://docs.microsoft.com/en-us/windows/win32/api/processenv/nf-processenv-searchpathw
//
//   - If the commandline is quoted, simply grab whatever is in the quotes and search for this directly.
//     We don't try any other logic here, if the application can't be found from the quoted contents we return an error.
//
//   - If the commandline is not quoted, we iterate over each possible application name by splitting the arguments and iterating
//     over them one by one while appending the last search each time until we either find a match or don't and return
//     an error. If we don't find the application on the first try, this means that the application name has a space in it
//     and we must adjust the commandline to add quotes around the application name.
//
//   - If the application is found, we return the fullpath to the executable and the adjusted commandline (if needed).
//
//     Examples:
//
//   - Input: "C:\Program Files\sub dir\program name"
//     Search order:
//     1. C:\Program.exe
//     2. C:\Program Files\sub.exe
//     3. C:\Program Files\sub dir\program.exe
//     4. C:\Program Files\sub dir\program name.exe
//
//     Returned commandline: "\"C:\Program Files\sub dir\program name\""
//
//   - Input: "\"program name\""
//     Search order:
//     1. program name.exe
//
//     Returned commandline: "\"program name\"
//
//   - Input: "\"program name\" -flags -for -program"
//     Search order:
//     1. program.exe
//     2. program name.exe
//
//     Returned commandline: "\"program name\" -flags -for -program"
//
//   - Input: "\"C:\path\to\program name\""
//     Search Order:
//     1. "C:\path\to\program name.exe"
//
//     Returned commandline: "\"C:\path\to\program name""
//
//   - Input: "C:\path\to\program"
//     Search Order:
//     1. "C:\path\to\program.exe"
//
//     Returned commandline: "C:\path\to\program"
//
// CreateProcess documentation: https://docs.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessa
func getApplicationName(commandLine, workingDirectory, pathEnv string) (string, string, error) {
	var (
		searchPath string
		result     string
	)

	// First we get the system paths concatenated with semicolons (C:\windows;C:\windows\system32;C:\windows\system;)
	// and use this as the basis for the directories to search for the application.
	systemPaths, err := getSystemPaths()
	if err != nil {
		return "", "", err
	}

	// If there's a working directory we should also add this to the list of directories to search.
	if workingDirectory != "" {
		searchPath += workingDirectory + ";"
	}

	// Append the path environment to the list of directories to search.
	if pathEnv != "" {
		searchPath += pathEnv + ";"
	}
	searchPath += systemPaths

	if searchPath[len(searchPath)-1] == ';' {
		searchPath = searchPath[:len(searchPath)-1]
	}

	// Application name was quoted, just search directly.
	//
	// For example given the commandline: "hello goodbye" -foo -bar -baz
	// we would search for the executable 'hello goodbye.exe'
	if commandLine != "" && commandLine[0] == '"' {
		index := strings.Index(commandLine[1:], "\"")
		if index == -1 {
			return "", "", errors.New("no ending quotation mark found in command")
		}
		path, err := searchPathForExe(commandLine[1:index+1], searchPath)
		if err != nil {
			return "", "", err
		}
		return path, commandLine, nil
	}

	// Application name wasn't quoted, try each possible application name.
	// For example given the commandline: hello goodbye, we would first try
	// to find 'hello.exe' and then 'hello goodbye.exe'
	var (
		trialName    string
		quoteCmdLine bool
		argsIndex    int
	)
	args := splitArgs(commandLine)

	// Loop through each element of the commandline and try and determine if any of them are executables.
	//
	// For example given the commandline: foo bar baz
	// if foo.exe is successfully found we will stop and return with the full path to 'foo.exe'. If foo doesn't succeed we
	// then try 'foo bar.exe' and 'foo bar baz.exe'.
	for argsIndex < len(args) {
		trialName += args[argsIndex]
		fullPath, err := searchPathForExe(trialName, searchPath)
		if err == nil {
			result = fullPath
			break
		}
		trialName += " "
		quoteCmdLine = true
		argsIndex++
	}

	// If we searched through every argument and didn't find an executable, we need to error out.
	if argsIndex == len(args) {
		return "", "", fmt.Errorf("failed to find executable %q", commandLine)
	}

	// If we found an executable but after we concatenated two arguments together,
	// we need to adjust the commandline to be quoted.
	//
	// For example given the commandline: foo bar
	// if 'foo bar.exe' is found, we need to adjust the commandline to
	// be quoted as this is what the platform expects (CreateProcess call).
	adjustedCommandLine := commandLine
	if quoteCmdLine {
		trialName = "\"" + trialName + "\""
		trialName += " " + strings.Join(args[argsIndex+1:], " ")
		adjustedCommandLine = strings.TrimSpace(trialName) // Take off trailing space at beginning and end.
	}

	return result, adjustedCommandLine, nil
}

// searchPathForExe calls the Windows API function `SearchPathW` to try and locate
// `fileName` by searching in `pathsToSearch`. `pathsToSearch` is generally a semicolon
// separated string of paths to search that `SearchPathW` will iterate through one by one.
// If the path resolved for `fileName` ends up being a directory, this function will return an
// error.
func searchPathForExe(fileName, pathsToSearch string) (string, error) {
	fileNamePtr, err := windows.UTF16PtrFromString(fileName)
	if err != nil {
		return "", err
	}

	pathsToSearchPtr, err := windows.UTF16PtrFromString(pathsToSearch)
	if err != nil {
		return "", err
	}

	extension, err := windows.UTF16PtrFromString(".exe")
	if err != nil {
		return "", err
	}

	path := make([]uint16, windows.MAX_PATH)
	_, err = winapi.SearchPath(
		pathsToSearchPtr,
		fileNamePtr,
		extension,
		windows.MAX_PATH,
		&path[0],
		nil,
	)
	if err != nil {
		return "", err
	}

	exePath := windows.UTF16PtrToString(&path[0])
	// Need to check if we just found a directory with the name of the executable and
	// .exe at the end. ping.exe is a perfectly valid directory name for example.
	attrs, err := os.Stat(exePath)
	if err != nil {
		return "", err
	}

	if attrs.IsDir() {
		return "", fmt.Errorf("found directory instead of executable %q", exePath)
	}

	return exePath, nil
}

// Returns the system paths (system32, system, and windows) as a search path,
// including a terminating ;.
//
// Typical output would be `C:\WINDOWS\system32;C:\WINDOWS\System;C:\WINDOWS;`
func getSystemPaths() (string, error) {
	var searchPath string
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", errors.Wrap(err, "failed to get system directory")
	}
	searchPath += systemDir + ";"

	windowsDir, err := windows.GetWindowsDirectory()
	if err != nil {
		return "", errors.Wrap(err, "failed to get Windows directory")
	}

	searchPath += windowsDir + "\\System;" + windowsDir + ";"
	return searchPath, nil
}
