package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"unicode/utf16"

	"github.com/Microsoft/go-winio/pkg/guid"
)

func HCSIDToGUID(id string) (guid.GUID, error) {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, utf16.Encode([]rune(strings.ToUpper(id)))); err != nil {
		return guid.GUID{}, err
	}
	// Namespace GUID: cab70344-facb-41e4-b5e5-ab6592283e6e
	g, err := guid.NewV5(guid.GUID{Data1: 0xcab70344, Data2: 0xfacb, Data3: 0x41e4, Data4: [8]byte{0xb5, 0xe5, 0xab, 0x65, 0x92, 0x28, 0x3e, 0x6e}}, buf.Bytes())
	if err != nil {
		return guid.GUID{}, err
	}
	return g, nil
}

func main() {
	if len(os.Args) != 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Printf("usage: %s <CONTAINER ID>\n", os.Args[0])
		os.Exit(1)
	}
	g, err := HCSIDToGUID(os.Args[1])
	if err != nil {
		fmt.Printf("error: %s\n", err)
	}
	fmt.Printf("%s\n", g)
}
