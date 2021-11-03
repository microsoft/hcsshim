// This package contains the proto and compiled go files for the node
// network service.
//
// A mock service under `mock` is used for unit testing the various services
// used for ncproxy.
//
// The mock service is compiled using the following command:
//
// mockgen -source="nodenetsvc.pb.go" -package="nodenetsvc_mock" > mock\nodenetsvc_mock.pb.go

package v1
