module github.com/Microsoft/hcsshim

go 1.13

require (
	github.com/Microsoft/go-winio v0.4.16
	github.com/containerd/cgroups v0.0.0-20200824123100-0b889c03f102
	github.com/containerd/console v1.0.1
	github.com/containerd/containerd v1.5.0-beta.1
	github.com/containerd/go-runc v0.0.0-20200220073739-7016d3ce2328
	github.com/containerd/ttrpc v1.0.2
	github.com/containerd/typeurl v1.0.1
	github.com/gogo/protobuf v1.3.2
	github.com/opencontainers/runtime-spec v1.0.3-0.20200929063507-e6143ca7d51d
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.7.0
	github.com/urfave/cli v1.22.2
	go.opencensus.io v0.22.3
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	golang.org/x/sys v0.0.0-20201202213521-69691e467435
	google.golang.org/grpc v1.30.0
)
