package cmd

import (
	"context"
	"net/url"
	"testing"
)

func Test_newBinaryCmd_Key_Value_Pair(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type config struct {
		name      string
		urlString string
		expected  string
	}

	tests := []*config{
		{
			name:      "use-path",
			urlString: "binary:///executable?-key=value",
			expected:  "/executable -key value",
		},
		{
			name:      "use-host",
			urlString: "binary://executable?-key=value",
			expected:  "/executable -key value",
		},
		{
			name:      "use-host-and-path",
			urlString: "binary://path/to/executable?flag",
			expected:  "/path/to/executable flag",
		},
	}

	for _, cfg := range tests {
		t.Run(cfg.name, func(t *testing.T) {
			u, err := url.Parse(cfg.urlString)
			if err != nil {
				t.Fatalf("failed to parse url: %s", cfg.urlString)
			}

			cmd, err := newBinaryCmd(ctx, u, nil)
			if err != nil {
				t.Fatalf("error while creating cmd: %s", err)
			}

			if cmd.String() != cfg.expected {
				t.Fatalf("failed to create cmd. expected: '%s', actual '%s'", cfg.expected, cmd.String())
			}
		})
	}
}

func Test_newBinaryCmd_Empty_Path(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	u, _ := url.Parse("scheme://")

	cmd, err := newBinaryCmd(ctx, u, nil)

	if cmd != nil {
		t.Fatalf("cmd is not nil: %s", cmd)
	}

	if err == nil {
		t.Fatalf("err is not expected to be nil")
	}
}

func Test_newBinaryCmd_flags(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	urlString := "schema:///path/to/binary?foo&bar&baz"
	uri, _ := url.Parse(urlString)

	expectedPath := "/path/to/binary"
	expectedFlags := map[string]bool{"foo": true, "bar": true, "baz": true}

	cmd, err := newBinaryCmd(ctx, uri, nil)

	if err != nil {
		t.Fatalf("error creating binary cmd: %s", err)
	}

	if cmd.Path != expectedPath {
		t.Fatalf("invalid cmd path: %s", cmd.Path)
	}

	for _, f := range cmd.Args[1:] {
		if _, ok := expectedFlags[f]; !ok {
			t.Fatalf("flag missing: '%s' in cmd: '%s'", f, cmd.String())
		}
	}
}
