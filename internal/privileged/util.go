package privileged

import (
	"strings"
)

// Converts an integer to a bitmask to be used by SetProcessAffinityMask
// (or just setting the affinity field for a job object wide limit).
func int32ToBitmask(num int32) int32 {
	var mask int32 = 0
	for i := int32(0); i < num; i++ {
		mask += (1 << i)
	}
	return mask
}

// Seperates path to executable/cmd from it's arguments
func seperateArgs(cmdline string) (string, []string) {
	split := strings.Fields(cmdline)
	return split[0], split[1:]
}
