//go:build windows

// This package allows tests can include the .syso to manifest them to pick up the right Windows build
//
// Manifested object files are generated via CMake (see: test\CMakeLists.txt)
package manifest
