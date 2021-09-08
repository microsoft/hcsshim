package shimdiag

import (
	fmt "fmt"
	"os"
	"path/filepath"
	"sort"
	strings "strings"

	"github.com/Microsoft/go-winio"
	"github.com/containerd/ttrpc"
	"golang.org/x/sys/windows"
)

const (
	shimPrefix = `\\.\pipe\ProtectedPrefix\Administrators\containerd-shim-`
	shimSuffix = `-pipe`
)

func findPipes(pattern string) ([]string, error) {
	path := `\\.\pipe\*`
	path16, err := windows.UTF16FromString(path)
	if err != nil {
		return nil, err
	}
	var data windows.Win32finddata
	h, err := windows.FindFirstFile(&path16[0], &data)
	if err != nil {
		return nil, &os.PathError{Op: "FindFirstFile", Path: path, Err: err}
	}
	var names []string
	for {
		name := `\\.\pipe\` + windows.UTF16ToString(data.FileName[:])
		if matched, _ := filepath.Match(pattern, name); matched {
			names = append(names, name)
		}
		err = windows.FindNextFile(h, &data)
		if err == windows.ERROR_NO_MORE_FILES {
			break
		}
		if err != nil {
			return nil, &os.PathError{Op: "FindNextFile", Path: path, Err: err}
		}
	}
	return names, nil
}

func FindShims(name string) ([]string, error) {
	pipes, err := findPipes(shimPrefix + name + "*" + shimSuffix)
	if err != nil {
		return nil, err
	}
	for i, p := range pipes {
		pipes[i] = p[len(shimPrefix) : len(p)-len(shimSuffix)]
	}
	sort.Strings(pipes)
	return pipes, nil
}

func findShim(name string) (string, error) {
	if strings.ContainsAny(name, "*?\\/") {
		return "", fmt.Errorf("invalid shim name %s", name)
	}
	shims, err := FindShims(name)
	if err != nil {
		return "", err
	}
	if len(shims) == 0 {
		return "", fmt.Errorf("no such shim %s", name)
	}
	if len(shims) > 1 && shims[0] != name {
		return "", fmt.Errorf("multiple shims beginning with %s", name)
	}
	return shims[0], nil
}

func GetShim(name string) (*ttrpc.Client, error) {
	shim, err := findShim(name)
	if err != nil {
		return nil, err
	}
	conn, err := winio.DialPipe(shimPrefix+shim+shimSuffix, nil)
	if err != nil {
		return nil, err
	}
	return ttrpc.NewClient(conn), nil
}
