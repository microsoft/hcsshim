package hcsshim

import (
	"crypto/sha1"
	"fmt"
)

type guid [16]byte

func newGuid(source string) *guid {
	h := sha1.Sum([]byte(source))
	var g guid
	copy(g[0:], h[0:16])
	return &g
}

func (g *guid) ToString() string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", g[0:4], g[4:6], g[6:8], g[8:10], g[10:])
}
