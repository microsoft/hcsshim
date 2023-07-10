module tools

go 1.18

require (
	github.com/Microsoft/go-winio v0.6.1
	github.com/Microsoft/hcsshim v0.10.0-rc.7
	github.com/containerd/protobuild v0.3.0
	github.com/containerd/ttrpc v1.2.2
	github.com/golang/mock v1.6.0
	github.com/josephspurrier/goversioninfo v1.4.0
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.1.0
	google.golang.org/protobuf v1.30.0
)

require (
	github.com/akavel/rsrc v0.10.2 // indirect
	github.com/containerd/cgroups/v3 v3.0.1 // indirect
	github.com/containerd/containerd v1.7.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/stretchr/testify v1.8.4 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/mod v0.10.0 // indirect
	golang.org/x/sys v0.9.0 // indirect
	golang.org/x/tools v0.8.0 // indirect
	google.golang.org/genproto v0.0.0-20230323212658-478b75c54725 // indirect
	google.golang.org/grpc v1.55.0 // indirect
)

replace github.com/Microsoft/hcsshim => ../
