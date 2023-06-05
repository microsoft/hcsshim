// Package computeagent contains the proto and compiled go files for the compute
// agent service.
//
// A mock service under `mock` is used for unit testing the various services
// used for ncproxy.
package computeagent

//go:generate go run github.com/golang/mock/mockgen -source=computeagent.pb.go -package=computeagent_mock -destination=mock\computeagent_mock.pb.go
