// Package v1 contains the proto and compiled go files for the node
// network service v1 implementation.
//
// A mock service under `mock` is used for unit testing the various services
// used for ncproxy.
package v1

//go:generate go run github.com/golang/mock/mockgen -source=nodenetsvc.pb.go -package=nodenetsvc_v1_mock -destination=mock\nodenetsvc_mock.pb.go
