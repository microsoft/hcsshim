# platform=linux
#
# John Howard Feb 2018. Based on github.com/linuxkit/lcow/pkg/init-lcow/Dockerfile
# This Dockerfile builds initrd.img and rootfs.tar.gz from local opengcs sources. 
# It can be used on a Windows machine running in LCOW mode.
#
# Manual steps:
#   git clone https://github.com/Microsoft/opengcs c:\go\src\github.com\Microsoft\opengcs
#   cd c:\go\src\github.com\Microsoft\opengcs
#   docker build --platform=linux -t opengcs .
#   docker run --rm -v c:\target:/out opengcs cp /initrd.img /out
#   docker run --rm -v c:\target:/out opengcs cp /rootfs.tar.gz /out
#   copy c:\target\initrd.img "c:\Program Files\Linux Containers"
#   <TODO: Additional step to generate VHD from rootfs.tar.gz and install>
#   <Restart the docker daemon to pick up the new initrd>

FROM linuxkit/runc:7c39a68490a12cde830e1922f171c451fb08e731 AS runc

FROM linuxkit/alpine:b1a36f0dd41e60142dd84dab7cd333ce7da1d1f8
ENV GOPATH=/go PATH=$PATH:/go/bin
RUN \
    # Create all the directories
    mkdir -p /target/etc/apk &&  \
    mkdir -p /target/bin && \
    mkdir -p /target/sbin && \
    mkdir -p /go/src/github.com/Microsoft/opengcs && \
    \
    # Generate base filesystem in /target
    cp -r /etc/apk/* /target/etc/apk/ && \
    apk add --no-cache --initdb -p /target alpine-baselayout busybox e2fsprogs musl && \
    rm -rf /target/etc/apk /target/lib/apk /target/var/cache && \
    \
    # Install the build packages
    apk add --no-cache build-base curl git go musl-dev && \
    \
    # Grab udhcpc_config.script
    curl -fSL "https://raw.githubusercontent.com/mirror/busybox/38d966943f5288bb1f2e7219f50a92753c730b14/examples/udhcp/simple.script" -o /target/sbin/udhcpc_config.script && \
    chmod ugo+rx /target/sbin/udhcpc_config.script
COPY --from=runc / /target/

# Add the init script
ADD https://raw.githubusercontent.com/linuxkit/lcow/b17397d2a79e1f375e2dd3a03daf34b39f8ee880/pkg/init-lcow/init /target/

# Add the sources for opengcs
COPY . /go/src/github.com/Microsoft/opengcs

# Build the binaries and add them to the target
RUN chmod 0755 /target/init && \
    cd /go/src/github.com/Microsoft/opengcs/service && \
	make && \
	cp -r bin /target && \
    cd .. && \
	git rev-parse              HEAD > /target/gcs.commit && \
	git rev-parse --abbrev-ref HEAD > /target/gcs.branch

# Generate the root filesystem in both initrd.img and rootfs.tar.gz formats
RUN cd /target && \
	find . | cpio -o --format="newc" | gzip -c > /initrd.img && \
	tar -c . | gzip -c > /rootfs.tar.gz && \
	printf "\nTargets:\n" && ls -l /initrd.img /rootfs.tar.gz && printf "\n"