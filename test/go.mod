module github.com/Microsoft/hcsshim/test

go 1.13

require (
	github.com/Microsoft/go-winio v0.4.16-0.20201130162521-d1ffc52c7331
	github.com/Microsoft/hcsshim v0.8.10
	github.com/containerd/containerd v1.4.1-0.20201210185422-f8992f451cf6
	github.com/containerd/go-runc v0.0.0-20200220073739-7016d3ce2328
	github.com/containerd/ttrpc v1.0.2
	github.com/containerd/typeurl v1.0.1
	github.com/gogo/protobuf v1.3.1
	github.com/opencontainers/runtime-spec v1.0.3-0.20200728170252-4d89ac9fbff6
	github.com/opencontainers/runtime-tools v0.0.0-20181011054405-1d69bd0f9c39
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.7.0
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	google.golang.org/grpc v1.30.0
	k8s.io/cri-api v0.20.1
)

replace github.com/Microsoft/hcsshim => ../
