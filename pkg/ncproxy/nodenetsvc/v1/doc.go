// Package v1 contains the proto and compiled go files for the node
// network service v1 implementation.
//
// A mock service under `mock` is used for unit testing the various services
// used for ncproxy.
package v1

//go:generate go run go.uber.org/mock/mockgen -source=nodenetsvc_grpc.pb.go -package=nodenetsvc_v1_mock -destination=mock\nodenetsvc_mock.pb.go
