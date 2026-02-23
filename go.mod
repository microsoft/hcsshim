module github.com/Microsoft/hcsshim

go 1.24.0

// protobuf/gRPC/ttrpc generation
tool (
	github.com/containerd/protobuild
	github.com/containerd/protobuild/cmd/go-fix-acronym
	github.com/containerd/ttrpc/cmd/protoc-gen-go-ttrpc
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)

// used in go:generate directives
tool (
	// generate Win32 API code
	github.com/Microsoft/go-winio/tools/mkwinsyscall

	// create syso files for manifesting
	github.com/josephspurrier/goversioninfo/cmd/goversioninfo

	// mock gRPC client and servers
	go.uber.org/mock/mockgen
)

require (
	github.com/Microsoft/cosesign1go v1.4.0
	github.com/Microsoft/didx509go v0.0.3
	github.com/Microsoft/go-winio v0.6.3-0.20251027160822-ad3df93bed29
	github.com/blang/semver/v4 v4.0.0
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/containerd/cgroups/v3 v3.0.5
	github.com/containerd/console v1.0.5
	github.com/containerd/containerd/api v1.9.0
	github.com/containerd/containerd/v2 v2.1.2
	github.com/containerd/errdefs v1.0.0
	github.com/containerd/errdefs/pkg v0.3.0
	github.com/containerd/go-runc v1.1.0
	github.com/containerd/log v0.1.0
	github.com/containerd/platforms v1.0.0-rc.1
	github.com/containerd/plugin v1.0.0
	github.com/containerd/ttrpc v1.2.7
	github.com/containerd/typeurl/v2 v2.2.3
	github.com/google/go-cmp v0.7.0
	github.com/google/go-containerregistry v0.20.1
	github.com/linuxkit/virtsock v0.0.0-20241009230534-cb6a20cc0422
	github.com/mattn/go-shellwords v1.0.12
	github.com/moby/sys/user v0.4.0
	github.com/open-policy-agent/opa v0.70.0
	github.com/opencontainers/cgroups v0.0.4
	github.com/opencontainers/runc v1.3.3
	github.com/opencontainers/runtime-spec v1.2.1
	github.com/pelletier/go-toml v1.9.5
	github.com/pkg/errors v0.9.1
	github.com/samber/lo v1.52.0
	github.com/sirupsen/logrus v1.9.3
	github.com/urfave/cli v1.22.16
	github.com/urfave/cli/v2 v2.27.6
	github.com/vishvananda/netlink v1.3.1
	github.com/vishvananda/netns v0.0.5
	go.etcd.io/bbolt v1.4.0
	go.opencensus.io v0.24.0
	go.uber.org/mock v0.6.0
	golang.org/x/net v0.43.0
	golang.org/x/sync v0.16.0
	golang.org/x/sys v0.39.0
	google.golang.org/grpc v1.75.0
	google.golang.org/protobuf v1.36.7
)

require (
	cyphar.com/go-pathrs v0.2.1 // indirect
	github.com/OneOfOne/xxhash v1.2.8 // indirect
	github.com/agnivade/levenshtein v1.2.0 // indirect
	github.com/akavel/rsrc v0.10.2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/checkpoint-restore/go-criu/v6 v6.3.0 // indirect
	github.com/containerd/continuity v0.4.5 // indirect
	github.com/containerd/fifo v1.1.0 // indirect
	github.com/containerd/protobuild v0.3.0 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.15.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.5 // indirect
	github.com/cyphar/filepath-securejoin v0.6.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/docker/cli v24.0.0+incompatible // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/docker v28.3.0+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.7.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/josephspurrier/goversioninfo v1.5.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/lestrrat-go/backoff/v2 v2.0.8 // indirect
	github.com/lestrrat-go/blackmagic v1.0.2 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/jwx v1.2.29 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/mdlayher/socket v0.5.1 // indirect
	github.com/mdlayher/vsock v1.2.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/moby/sys/capability v0.4.0 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/mrunalp/fileutils v0.5.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/opencontainers/selinux v1.13.0 // indirect
	github.com/prometheus/client_golang v1.22.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/seccomp/libseccomp-golang v0.10.0 // indirect
	github.com/tchap/go-patricia/v2 v2.3.2 // indirect
	github.com/vbatts/tar-split v0.11.5 // indirect
	github.com/veraison/go-cose v1.1.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	github.com/yashtewari/glob-intersection v0.2.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel v1.37.0 // indirect
	go.opentelemetry.io/otel/metric v1.37.0 // indirect
	go.opentelemetry.io/otel/sdk v1.37.0 // indirect
	go.opentelemetry.io/otel/trace v1.37.0 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.5.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20250428153025-10db94c68c34
