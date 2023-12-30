package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	fmt.Println("Waiting for OS signal...")

	signalChannel := make(chan os.Signal)
	wait := make(chan int, 1)

	signal.Notify(signalChannel, syscall.SIGHUP)
	signal.Notify(signalChannel, syscall.SIGINT)
	signal.Notify(signalChannel, syscall.SIGTERM)

	go func() {
		sig := <-signalChannel
		switch sig {
		case syscall.SIGHUP:
			fmt.Println("SIGHUP")
			wait <- 1
		case syscall.SIGTERM:
			fmt.Println("SIGTERM")
			wait <- 1
		case syscall.SIGINT:
			fmt.Println("SIGINT")
			wait <- 1
		}
	}()

	<-wait

	fmt.Println("Exiting in 60 seconds...")
	for i := 60; i > 0; i-- {
		fmt.Printf("%d\n", i)
		time.Sleep(1 * time.Second)
	}

	fmt.Println("Goodbye")
}
