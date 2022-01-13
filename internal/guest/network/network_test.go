//go:build linux
// +build linux

package network

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func Test_GenerateEtcHostsContent(t *testing.T) {
	type testcase struct {
		name string

		hostname string

		expectedContent string
	}
	testcases := []*testcase{
		{
			name:     "Net BIOS Name",
			hostname: "Test",
			expectedContent: `127.0.0.1 localhost
127.0.0.1 Test

# The following lines are desirable for IPv6 capable hosts
::1     ip6-localhost ip6-loopback
fe00::0 ip6-localnet
ff00::0 ip6-mcastprefix
ff02::1 ip6-allnodes
ff02::2 ip6-allrouters
`,
		},
		{
			name:     "FQDN",
			hostname: "test.rules.domain.com",
			expectedContent: `127.0.0.1 localhost
127.0.0.1 test.rules.domain.com test

# The following lines are desirable for IPv6 capable hosts
::1     ip6-localhost ip6-loopback
fe00::0 ip6-localnet
ff00::0 ip6-mcastprefix
ff02::1 ip6-allnodes
ff02::2 ip6-allrouters
`,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			c := GenerateEtcHostsContent(context.Background(), tc.hostname)
			if c != tc.expectedContent {
				t.Fatalf("expected content: %q got: %q", tc.expectedContent, c)
			}
		})
	}
}

// create a test FileInfo so we can return back a value to ReadDir
type testFileInfo struct {
	FileName    string
	IsDirectory bool
}

func (t *testFileInfo) Name() string {
	return t.FileName
}
func (t *testFileInfo) Size() int64 {
	return 0
}
func (t *testFileInfo) Mode() fs.FileMode {
	if t.IsDirectory {
		return fs.ModeDir
	}
	return 0
}
func (t *testFileInfo) ModTime() time.Time {
	return time.Now()
}
func (t *testFileInfo) IsDir() bool {
	return t.IsDirectory
}
func (t *testFileInfo) Sys() interface{} {
	return nil
}

var _ = (os.FileInfo)(&testFileInfo{})

func Test_InstanceIDToName(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)

	vmBusGUID := "1111-2222-3333-4444"
	testIfName := "test-eth0"

	vmbusWaitForDevicePath = func(_ context.Context, vmBusGUIDPattern string) (string, error) {
		vmBusPath := filepath.Join("/sys/bus/vmbus/devices", vmBusGUIDPattern)
		return vmBusPath, nil
	}

	storageWaitForFileMatchingPattern = func(_ context.Context, pattern string) (string, error) {
		return pattern, nil
	}

	ioutilReadDir = func(dirname string) ([]os.FileInfo, error) {
		info := &testFileInfo{
			FileName:    testIfName,
			IsDirectory: false,
		}
		return []fs.FileInfo{info}, nil
	}
	actualIfName, err := InstanceIDToName(ctx, vmBusGUID, false)
	if err != nil {
		t.Fatalf("expected no error, instead got %v", err)
	}
	if actualIfName != testIfName {
		t.Fatalf("expected to get %v ifname, instead got %v", testIfName, actualIfName)
	}
}

func Test_InstanceIDToName_VPCI(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)

	vmBusGUID := "1111-2222-3333-4444"
	testIfName := "test-eth0-vpci"

	pciFindDeviceFullPath = func(_ context.Context, vmBusGUID string) (string, error) {
		return filepath.Join("/sys/bus/vmbus/devices", vmBusGUID), nil
	}

	storageWaitForFileMatchingPattern = func(_ context.Context, pattern string) (string, error) {
		return pattern, nil
	}

	ioutilReadDir = func(dirname string) ([]os.FileInfo, error) {
		info := &testFileInfo{
			FileName:    testIfName,
			IsDirectory: false,
		}
		return []os.FileInfo{info}, nil
	}
	actualIfName, err := InstanceIDToName(ctx, vmBusGUID, true)
	if err != nil {
		t.Fatalf("expected no error, instead got %v", err)
	}
	if actualIfName != testIfName {
		t.Fatalf("expected to get %v ifname, instead got %v", testIfName, actualIfName)
	}
}
