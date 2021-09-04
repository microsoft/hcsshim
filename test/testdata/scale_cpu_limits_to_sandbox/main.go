package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"
)

const defaultDuration = 5

// This implementation is a simplified version of https://github.com/vikyd/go-cpu-load
func main() {
	cores := runtime.NumCPU()
	runtime.GOMAXPROCS(cores)

	loadDuration := defaultDuration
	// Check if duration has been passed explicitly
	if len(os.Args) > 1 {
		var err error
		loadDuration, err = strconv.Atoi(os.Args[1])
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "first argument must be integer: %s", err)
			os.Exit(1)
		}
	}

	for i := 0; i < cores; i++ {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()

			begin := time.Now()
			for {
				if time.Now().Sub(begin) > time.Duration(loadDuration)*time.Second {
					break
				}
			}
		}()
	}
	time.Sleep(time.Duration(loadDuration) * time.Second)
}
