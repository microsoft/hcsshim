package jobobject

import (
	"context"
	"testing"
)

func TestJobNilOptions(t *testing.T) {
	_, err := Create(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestJobCreateAndOpen(t *testing.T) {
	var (
		ctx     = context.Background()
		options = &Options{Name: "test"}
	)

	_, err := Create(ctx, options)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Open(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
}
