module github.com/Microsoft/hcsshim

go 1.17

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.5.1
	github.com/cenkalti/backoff/v4 v4.1.2
	github.com/containerd/cgroups v1.0.2
	github.com/containerd/console v1.0.3
	github.com/containerd/containerd v1.5.10
	github.com/containerd/containerd/api v1.6.0-beta.3
	github.com/containerd/go-runc v1.0.0
	github.com/containerd/ttrpc v1.1.0
	github.com/containerd/typeurl v1.0.2
	github.com/gogo/protobuf v1.3.2
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.5.6
	github.com/google/go-containerregistry v0.5.1
	github.com/google/uuid v1.3.0
	github.com/linuxkit/virtsock v0.0.0-20201010232012-f8cee7dfc7a3
	github.com/mattn/go-shellwords v1.0.12
	github.com/opencontainers/runc v1.0.3
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli v1.22.2
	github.com/vishvananda/netlink v1.1.1-0.20210330154013-f5de75959ad5
	github.com/vishvananda/netns v0.0.0-20210104183010-2eb08e3e575f
	go.etcd.io/bbolt v1.3.6
	go.opencensus.io v0.22.4
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20211216021012-1d35b9e2eb4e
	google.golang.org/grpc v1.43.0
)

require (
	github.com/containerd/fifo v1.0.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.0 // indirect
	github.com/docker/cli v0.0.0-20191017083524-a8ff7f821017 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v1.4.2-0.20190924003213-a8608b5b67c7 // indirect
	github.com/docker/docker-credential-helpers v0.6.3 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/godbus/dbus/v5 v5.0.4 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/moby/sys/mountinfo v0.5.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/russross/blackfriday/v2 v2.0.1 // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	golang.org/x/net v0.0.0-20211216030914-fe4d6282115f // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/genproto v0.0.0-20211208223120-3a66f561d7aa // indirect
	google.golang.org/protobuf v1.27.1 // indirect
)

replace (
	github.com/containerd/containerd => github.com/kevpar/containerd v1.2.1-0.20220329205807-c612c11661fa
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20200224152610-e50cd9704f63
)
