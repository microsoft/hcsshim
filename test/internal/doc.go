// This package provides helper functions for testing hcsshim, primarily aimed at the
// end-to-end, integration, and functional tests in ./test. It can, however, be used
// by unit tests.
//
// These files are primarily intended for tests, and using them in code will cause test
// dependencies to be treated normally, which may cause circular import issues with upstream
// packages that vendor hcsshim.
// See https://github.com/microsoft/hcsshim/issues/1148.
//
// Even though this package is meant for testing, appending _test to all files causes issues
// when running or building tests in other folders (ie, ./test/cri-containerd).
// See https://github.com/golang/go/issues/8279.
// Additionally, adding a `//go:build functional` constraint would require internal tests
// (ie, schemaversion_test.go) and unit tests that import this package to be build with
// the `functional` tag.
package internal
