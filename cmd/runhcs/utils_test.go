package main

import (
	"os"
	"testing"
)

func Test_AbsPathOrEmpty(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get test wd: %v", err)
	}

	tests := []string{
		"",
		safePipePrefix + "test",
		safePipePrefix + "test with spaces",
		"test",
		"C:\\test..\\test",
	}
	expected := []string{
		"",
		safePipePrefix + "test",
		safePipePrefix + "test%20with%20spaces",
		wd + "\\test",
		"C:\\test..\\test",
	}
	for i, test := range tests {
		actual, err := absPathOrEmpty(test)
		if err != nil {
			t.Fatalf("absPathOrEmpty: error '%v'", err)
		}
		if actual != expected[i] {
			t.Fatalf("absPathOrEmpty: actual '%s' != '%s'", actual, expected[i])
		}
	}
}

func Test_SafePipePath(t *testing.T) {
	tests := []string{"test", "test with spaces", "test/with\\\\.\\slashes", "test.with..dots..."}
	expected := []string{"test", "test%20with%20spaces", "test%2Fwith%5C%5C.%5Cslashes", "test.with..dots..."}
	for i, test := range tests {
		actual := safePipePath(test)
		e := safePipePrefix + expected[i]
		if actual != e {
			t.Fatalf("safePipePath: actual '%s' != '%s'", actual, expected[i])
		}
	}
}
