package main

import (
	"fmt"
	"log"

	"golang.org/x/sys/windows"
)

func main() {
	var tz windows.Timezoneinformation
	_, err := windows.GetTimeZoneInformation(&tz)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf(windows.UTF16ToString(tz.StandardName[:]))
}
