// +build functional

package runhcs

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	runhcs "github.com/Microsoft/hcsshim/pkg/go-runhcs"
)

func Test_CreateScratch_EmptyDestpath_Fail(t *testing.T) {
	rhcs := runhcs.Runhcs{
		Debug: true,
	}

	ctx := context.TODO()
	err := rhcs.CreateScratch(ctx, "")
	if err == nil {
		t.Fatal("Should have failed 'CreateScratch' command.")
	}
}

func Test_CreateScratch_DirDestpath_Failure(t *testing.T) {
	rhcs := runhcs.Runhcs{
		Debug: true,
	}

	td, err := ioutil.TempDir("", "CreateScratch")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(td)

	ctx := context.TODO()
	err = rhcs.CreateScratch(ctx, td)
	if err == nil {
		t.Fatal("Should have failed 'CreateScratch' command with dir destpath")
	}
}

func Test_CreateScratch_ValidDestpath_Success(t *testing.T) {
	rhcs := runhcs.Runhcs{
		Debug: true,
	}

	td, err := ioutil.TempDir("", "CreateScratch")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(td)

	scratchPath := filepath.Join(td, "scratch.vhdx")

	ctx := context.TODO()
	err = rhcs.CreateScratch(ctx, scratchPath)
	if err != nil {
		t.Fatalf("Failed 'CreateScratch' command with: %v", err)
	}
	_, err = os.Stat(scratchPath)
	if err != nil {
		t.Fatalf("Failed to stat scratch path with: %v", err)
	}
}

func Test_CreateScratchWithOpts_SizeGB_Success(t *testing.T) {
	rhcs := runhcs.Runhcs{
		Debug: true,
	}

	td, err := ioutil.TempDir("", "CreateScratchWithSize")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(td)

	scratchPath := filepath.Join(td, "scratch.vhdx")

	ctx := context.TODO()
	opts := &runhcs.CreateScratchOpts{
		SizeGB: 30,
	}
	err = rhcs.CreateScratchWithOpts(ctx, scratchPath, opts)
	if err != nil {
		t.Fatalf("Failed 'CreateScratch' command with: %v", err)
	}
	_, err = os.Stat(scratchPath)
	if err != nil {
		t.Fatalf("Failed to stat scratch path with: %v", err)
	}
}
