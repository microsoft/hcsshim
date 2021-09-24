package main

import (
	"fmt"
	"time"
)

// The contents of the program are irrelevant. This binary will be placed in an image that is used for testing the working directory behavior
// for job containers. So as long as the binary is launched is all that's being tested.
func main() {
	for {
		fmt.Println("Hello world")
		time.Sleep(time.Second * 5)
	}
}
