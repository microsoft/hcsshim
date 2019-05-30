// +build linux

package network

import (
	"context"
	"testing"
)

func Test_GenerateResolvConfContent(t *testing.T) {
	type testcase struct {
		name string

		searches []string
		servers  []string
		options  []string

		expectedContent string
		expectErr       bool
	}
	testcases := []*testcase{
		{
			name: "Empty",
		},
		{
			name:      "MaxSearches",
			searches:  []string{"1", "2", "3", "4", "5", "6", "7"},
			expectErr: true,
		},
		{
			name:            "ValidSearches",
			searches:        []string{"a.com", "b.com"},
			expectedContent: "search a.com b.com\n",
		},
		{
			name:            "ValidServers",
			servers:         []string{"8.8.8.8", "8.8.4.4"},
			expectedContent: "nameserver 8.8.8.8\nnameserver 8.8.4.4\n",
		},
		{
			name:            "ValidOptions",
			options:         []string{"timeout:30", "inet6"},
			expectedContent: "options timeout:30 inet6\n",
		},
		{
			name:            "All",
			searches:        []string{"a.com", "b.com"},
			servers:         []string{"8.8.8.8", "8.8.4.4"},
			options:         []string{"timeout:30", "inet6"},
			expectedContent: "search a.com b.com\nnameserver 8.8.8.8\nnameserver 8.8.4.4\noptions timeout:30 inet6\n",
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := GenerateResolvConfContent(context.Background(), tc.searches, tc.servers, tc.options)
			if tc.expectErr && err == nil {
				t.Fatal("expected err got nil")
			} else if !tc.expectErr && err != nil {
				t.Fatalf("expected no error got %v:", err)
			}

			if c != tc.expectedContent {
				t.Fatalf("expected content: %q got: %q", tc.expectedContent, c)
			}
		})
	}
}

func Test_MergeValues(t *testing.T) {
	type testcase struct {
		name string

		first  []string
		second []string

		expected []string
	}
	testcases := []*testcase{
		{
			name: "BothEmpty",
		},
		{
			name:     "FirstEmpty",
			second:   []string{"a", "b"},
			expected: []string{"a", "b"},
		},
		{
			name:     "SecondEmpty",
			first:    []string{"a", "b"},
			expected: []string{"a", "b"},
		},
		{
			name:     "AllUnique",
			first:    []string{"a", "c", "d"},
			second:   []string{"b", "e"},
			expected: []string{"a", "c", "d", "b", "e"},
		},
		{
			name:     "NonUnique",
			first:    []string{"a", "c", "d"},
			second:   []string{"a", "b", "c", "d"},
			expected: []string{"a", "c", "d", "b"},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			m := MergeValues(tc.first, tc.second)
			if len(m) != len(tc.expected) {
				t.Fatalf("expected %d entries got: %d", len(tc.expected), len(m))
			}
			for i := 0; i < len(tc.expected); i++ {
				if tc.expected[i] != m[i] {
					t.Logf("%v :: %v", tc.expected, m)
					t.Fatalf("expected value: %q at index: %d got: %q", tc.expected[i], i, m[i])
				}
			}
		})
	}
}
