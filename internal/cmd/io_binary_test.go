//go:build windows

package cmd

import (
	"context"
	"errors"
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
			name:      "Path_With_Fwd_Slashes",
			urlString: "binary:///executable?-key=value",
			expected:  `\executable -key value`,
		},
		{
			name:      "Clean_Path_With_Dots_And_Multiple_Fwd_Slashes",
			urlString: "binary:///../path/to///to/../executable",
			expected:  `\path\to\executable`,
		},
		{
			name:      "Clean_Path_With_Dots_And_Multiple_Back_Slashes",
			urlString: `binary:///..\path\to\\\from\..\executable`,
			expected:  `\path\to\executable`,
		},
		{
			name:      "Stripped_Path_With_Forward_Slashes",
			urlString: "binary:///D:/path/to/executable",
			expected:  `D:\path\to\executable`,
		},
		{
			name:      "Stripped_Path_With_Back_Slashes",
			urlString: `binary:///D:\path\to\executable`,
			expected:  `D:\path\to\executable`,
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

func Test_newBinaryCmd_Unsafe_Path(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type config struct {
		name          string
		urlString     string
		expectedError error
	}

	for _, cfg := range []*config{
		{
			name:          "UNC_Path_With_Back_Slashes",
			urlString:     `binary:///\server\share\executable`,
			expectedError: ErrUnsafePath,
		},
		{
			name:          "UNC_Path_With_Forward_Slashes",
			urlString:     `binary:////server/share/executable`,
			expectedError: ErrUnsafePath,
		},
	} {
		t.Run(cfg.name, func(t *testing.T) {
			u, err := url.Parse(cfg.urlString)
			if err != nil {
				t.Fatalf("failed to parse url: %s", cfg.urlString)
			}

			_, err = newBinaryCmd(ctx, u, nil)
			if err == nil {
				t.Fatalf("no error was returned")
			}
			if !errors.Is(err, cfg.expectedError) {
				t.Fatalf("expected error: %s, actual: %s", cfg.expectedError, err)
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

	expectedPath := `\path\to\binary`
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
