package winapi

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func wideStringsEqual(target, actual []uint16) bool {
	if len(target) != len(actual) {
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
	// include UTF8 chars which take more than 8 bits to encode
	targetStrings := []string{"abcde", "abcd\n", "C:\\Test", "\\&_Test", "Äb", "\u8483\u119A2\u0041"}
	for _, target := range targetStrings {
		targetWideString := utf16.Encode(([]rune)(target))
		targetLength := uint16(len(targetWideString) * 2)

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

		uniBufferStringAsSlice := Uint16BufferToSlice(uni.Buffer, int(targetLength/2))

		if !wideStringsEqual(targetWideString, uniBufferStringAsSlice) {
			t.Fatalf("Expected wide string %v, got %v instead", targetWideString, uniBufferStringAsSlice)
		}
	}
}

func TestUnicodeToString(t *testing.T) {
	targetStrings := []string{"abcde", "abcd\n", "C:\\Test", "\\&_Test", "Äb", "\u8483\u119A2\u0041"}
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

func TestUnicodeStringLimit(t *testing.T) {
	var sb strings.Builder

	// limit in bytes of how long the input string can be
	// -1 to account for null character.
	limit := NTSTRSAFE_UNICODE_STRING_MAX_CCH - 1

	lengths := []int{limit - 1, limit, limit + 1}
	testStrings := []string{}
	for _, len := range lengths {
		sb.Reset()
		for i := 0; i < len; i++ {
			// We are deliberately writing byte 41 here as it takes only 8
			// bits in UTF-8 encoding.  If we use non-ASCII chars the limit
			// calculations used above won't work.
			if err := sb.WriteByte(41); err != nil {
				t.Fatalf("string creation failed: %s", err)
			}
		}
		testStrings = append(testStrings, sb.String())
	}

	for i, testStr := range testStrings {
		_, err := NewUnicodeString(testStr)
		if lengths[i] > limit && err == nil {
			t.Fatalf("input string of length %d should throw ENAMETOOLONG error", lengths[i])
		} else if lengths[i] <= limit && err != nil {
			t.Fatalf("unexpected error for length %d: %s", lengths[i], err)
		}
	}
}
