package debug

import "runtime"

func DumpStacks() string {
	buf := make([]byte, 4096)
	for {
		buf = buf[:runtime.Stack(buf, true)]
		if len(buf) < cap(buf) {
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	return string(buf)
}
