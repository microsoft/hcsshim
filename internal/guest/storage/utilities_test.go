//go:build linux
// +build linux

package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Test_WaitForFileMatchingPattern_Success(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 2*time.Second)

	testDir := t.TempDir()

	actualPath := filepath.Join(testDir, "path1")
	err := os.Mkdir(actualPath, 0777)
	if err != nil {
		t.Fatalf("unexpected error creating test path: %v", err)
	}

	pathPattern := filepath.Join(testDir, "path*")
	pathsToTest := []string{actualPath, pathPattern}
	for _, p := range pathsToTest {
		result, err := WaitForFileMatchingPattern(ctx, p)
		if err != nil {
			t.Fatalf("expected to find path %v but got error: %v", p, err)
		}
		if result != actualPath {
			t.Fatalf("expected to return path %s, instead go %s", actualPath, result)
		}
	}
}

func Test_WaitForFileMatchingPattern_Multiple_Matches(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 2*time.Second)

	testDir := t.TempDir()

	actualPaths := []string{"path1", "path2"}
	for _, p := range actualPaths {
		fullPath := filepath.Join(testDir, p)
		err := os.Mkdir(fullPath, 0777)
		if err != nil {
			t.Fatalf("unexpected error creating test path: %v", err)
		}
	}

	pathPattern := filepath.Join(testDir, "path*")
	_, err := WaitForFileMatchingPattern(ctx, pathPattern)
	if err == nil {
		t.Fatalf("expected to fail due to multiple matching files")
	}
}

func Test_WaitForFileMatchingPattern_No_Matches(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 2*time.Second)

	testDir := t.TempDir()

	actualPath := filepath.Join(testDir, "path1")
	err := os.Mkdir(actualPath, 0777)
	if err != nil {
		t.Fatalf("unexpected error creating test path: %v", err)
	}

	badTestPath := filepath.Join(testDir, "path2")
	_, err = WaitForFileMatchingPattern(ctx, badTestPath)
	if err == nil {
		t.Fatalf("expected to fail due to no matching files")
	}
}
