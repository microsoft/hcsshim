package winapi

import (
	"testing"
	"unicode/utf16"
	"unsafe"
)

func wideStringsEqual(target, actual []uint16, actualLengthInBytes int) bool {
	actualLength := actualLengthInBytes / 2
	if len(target) != actualLength {
		return false
	}

	for i := range target {
		if target[i] != actual[i] {
			return false
		}
	}
	return true
}

func TestNewUnicodeString(t *testing.T) {
	targetStrings := []string{"abcde", "abcd\n", "C:\\Test", "\\&_Test"}
	for _, target := range targetStrings {
		targetLength := uint16(len(target) * 2)
		targetWideString := utf16.Encode(([]rune)(target))

		uni, err := NewUnicodeString(target)
		if err != nil {
			t.Fatalf("failed to convert target string %s to Unicode String with %v", target, err)
		}

		if uni.Length != targetLength {
			t.Fatalf("Expected new Unicode String length to be %d for target string %s, got %d instead", targetLength, target, uni.Length)
		}
		if uni.MaximumLength != targetLength {
			t.Fatalf("Expected new Unicode String maximum length to be %d for target string %s, got %d instead", targetLength, target, uni.MaximumLength)
		}

		uniBufferStringAsSlice := (*[32768]uint16)(unsafe.Pointer(uni.Buffer))[:]

		// since we have to do casting to convert the unicode string's buffer into a uint16 slice
		// the length of the actual slice will not be the true length of the contents in the unicode buffer
		// therefore we need to use the unicode string's length field when comparing
		if !wideStringsEqual(targetWideString, uniBufferStringAsSlice, int(uni.Length)) {
			t.Fatalf("Expected wide string %v, got %v instead", targetWideString, uniBufferStringAsSlice[:uni.Length])
		}
	}
}

func TestUnicodeToString(t *testing.T) {
	targetStrings := []string{"abcde", "abcd\n", "C:\\Test", "\\&_Test"}
	for _, target := range targetStrings {
		uni, err := NewUnicodeString(target)
		if err != nil {
			t.Fatalf("failed to convert target string %s to Unicode String with %v", target, err)
		}

		actualString := uni.String()
		if actualString != target {
			t.Fatalf("Expected unicode string function to return %s, instead got %s", target, actualString)
		}
	}
}
