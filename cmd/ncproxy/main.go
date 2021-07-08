package main

import (
	"github.com/sirupsen/logrus"
)

func main() {
	if err := run(); err != nil {
		logrus.Fatal(err)
	}
}
