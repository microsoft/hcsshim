package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func Test_Paths_EmptyString_NotAllowed(t *testing.T) {
	args := []string{
		"wait-paths",
		"-p",
		"",
	}
	app := newCliApp()
	err := app.Run(args)
	if err == nil {
		t.Fatal("expected error, got nothing")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("expected 'cannot be empty' error, got: %s", err)
	}
}

func Test_InvalidWaitPath_DefaultTimeout(t *testing.T) {
	args := []string{
		"wait-paths",
		"-p",
		"non-existent",
	}
	app := newCliApp()
	err := app.Run(args)
	if err == nil {
		t.Fatalf("expected error, got nothing")
	}
	if !strings.Contains(err.Error(), "timeout while waiting") {
		t.Fatalf("expected 'timeout error', got: %s", err)
	}
}

func Test_InvalidWaitPath_5SecondTimeout(t *testing.T) {
	args := []string{
		"wait-paths",
		"-p",
		"non-existent",
		"-t",
		"5",
	}
	app := newCliApp()
	start := time.Now()
	err := app.Run(args)
	if err == nil {
		t.Fatal("expected timeout error, got nothing")
	}
	if !strings.Contains(err.Error(), "timeout while waiting") {
		t.Fatalf("expected 'timeout error', got: %s", err)
	}

	end := time.Now()
	diff := end.Sub(start)
	diffSeconds := math.Round(diff.Seconds())
	if diffSeconds != 5 {
		t.Fatalf("expected 5 second timeout, got: %f", diffSeconds)
	}
}

func Test_Valid_Paths_AlreadyPresent(t *testing.T) {
	tmpDir := t.TempDir()
	var files []string
	for _, name := range []string{"file1", "file2"} {
		filePath := filepath.Join(tmpDir, name)
		if f, err := os.Create(filePath); err != nil {
			t.Fatalf("failed to create temporary file %s: %s", name, err)
		} else {
			_ = f.Close()
		}
		files = append(files, filePath)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()
	pathsArg := strings.Join(files, ",")

	args := []string{
		"wait-paths",
		"-p",
		pathsArg,
	}
	app := newCliApp()
	if err := app.Run(args); err != nil {
		t.Fatalf("expected no error, got: %s", err)
	}
}

func Test_Valid_Paths_BecomeAvailableLater(t *testing.T) {
	tmpDir := t.TempDir()
	var files []string
	for _, name := range []string{"file1", "file2"} {
		files = append(files, filepath.Join(tmpDir, name))
	}
	pathsArg := strings.Join(files, ",")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	errChan := make(chan error)
	args := []string{
		"wait-paths",
		"-p",
		pathsArg,
	}
	app := newCliApp()
	go func() {
		errChan <- app.Run(args)
	}()

	go func() {
		time.Sleep(5 * time.Second)
		for _, fileName := range files {
			if f, err := os.Create(fileName); err != nil {
				errChan <- err
				return
			} else {
				_ = f.Close()
			}
		}
	}()

	if err := <-errChan; err != nil {
		close(errChan)
		t.Fatalf("expected no error, got: %s", err)
	}
}
