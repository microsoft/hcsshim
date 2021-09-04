package main

import (
	"os"

	"github.com/sirupsen/logrus"
)

func main() {
	if err := app().Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
