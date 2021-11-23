// Package computeagent contains the proto and compiled go files for the compute
// agent service.
//
// A mock service under `mock` is used for unit testing the various services
// used for ncproxy.
//
// The mock service is compiled using the following command:
//
// mockgen -source="computeagent.pb.go" -package="computeagent_mock" > mock\computeagent_mock.pb.go

package computeagent
