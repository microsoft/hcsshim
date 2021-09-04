package main

import (
	"github.com/sirupsen/logrus"
)

type nopFormatter struct{}

// Format does nothing and returns a nil slice.
func (nopFormatter) Format(*logrus.Entry) ([]byte, error) { return nil, nil }
