package version

import (
	"embed"
	"strings"
)

// Using `//go:embed data/VERSION` (similarly for `data/COMMIT` and `data/BRANCH`) will
// fail at build time if the files don't exist, which will break existing build workflows.
//
// Alternatively, committing those files in git will cause problems as they will be constantly
// updated and overwitten if devs update them locally and commit the changes.
//
// Therefore, we embed a (non-empty) directory and look up the files at run-time so builds
// succeed regardless of whether the files are exist or not.
// `data/.gitignore` is our fallback file, which keeps `data/` non-empty and prevents [embed]
// from failing.

// Using a dedicated `data` directory allows us to separate out what files to embed.
// (Writing `//go:embed *` would include everything in this directory, including this file.)

// See scripts/Set-VersionInfo.ps1 for an example of setting these values via files in data/.

//go:embed data/*
var data embed.FS

var (
	// Branch is the git branch the binary was built from.
	Branch = readDataFile("BRANCH")

	// Commit is the git commit the binary was built from.
	Commit = readDataFile("COMMIT")

	// Version is the complete semver.
	Version = readDataFile("VERSION")
)

func readDataFile(f string) string {
	b, _ := data.ReadFile("data/" + f)
	return strings.TrimSpace(string(b))
}
