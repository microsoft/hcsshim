package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	fmt.Print(strings.Join(os.Args, " "))
}
