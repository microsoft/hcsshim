# platform=linux
#
# John Howard Feb 2018. Based on github.com/linuxkit/lcow/pkg/init-lcow/Dockerfile
# This Dockerfile builds initrd.img from local opengcs sources. It can be used
# on a Windows machine building in LCOW mode.
#
# Manual steps:
#   git clone https://github.com/Microsoft/opengcs c:\go\src\github.com\Microsoft\opengcs
#   cd c:\go\src\github.com\Microsoft\opengcs
#   docker build --platform=linux -t opengcs .
#   docker run --rm -v c:\initrd:/out opengcs cp /initrd.img /out
#   copy c:\initrd\initrd.img "c:\Program Files\Linux Containers"
#   <Restart the docker daemon to pick up the new initrd>

FROM linuxkit/runc:7b15b00b4e3507d62e3ed8d44dfe650561cd35ff AS runc

FROM linuxkit/alpine:585174df463ba33e6c0e2050a29a0d9e942d56cb
ENV GOPATH=/go PATH=$PATH:/go/bin
RUN \
	# Create all the directories
	mkdir -p /initrd/etc/apk &&  \
    mkdir -p /initrd/bin && \
	mkdir -p /initrd/sbin && \
	mkdir -p /go/src/github.com/Microsoft/opengcs && \
	\
	# Generate base filesystem in /initrd
    cp -r /etc/apk/* /initrd/etc/apk/ && \
    apk add --no-cache --initdb -p /initrd alpine-baselayout busybox e2fsprogs musl && \
    rm -rf /initrd/etc/apk /initrd/lib/apk /initrd/var/cache && \
	\
	# Install the build packages
    apk add --no-cache build-base curl git go musl-dev && \
	\
	# Grab udhcpc_config.script
    curl -fSL "https://raw.githubusercontent.com/mirror/busybox/38d966943f5288bb1f2e7219f50a92753c730b14/examples/udhcp/simple.script" -o /initrd/sbin/udhcpc_config.script && \
    chmod ugo+rx /initrd/sbin/udhcpc_config.script
COPY --from=runc / /initrd/
ADD https://raw.githubusercontent.com/linuxkit/lcow/b17397d2a79e1f375e2dd3a03daf34b39f8ee880/pkg/init-lcow/init /initrd/
COPY . /go/src/github.com/Microsoft/opengcs
RUN chmod 0755 /initrd/init && \
    cd /go/src/github.com/Microsoft/opengcs/service && make && cp -r bin /initrd && \
	cd /initrd && find . | cpio -o --format="newc" | gzip -c > /initrd.img
	