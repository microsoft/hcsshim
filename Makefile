GO:=go
GO_FLAGS:=-ldflags "-s -w" # strip Go binaries
GO_BUILD:=CGO_ENABLED=0 $(GO) build $(GO_FLAGS)

CFLAGS:=-O2
LDFLAGS:=-static -s # strip C binaries

SRCROOT=$(dir $(abspath $(lastword $(MAKEFILE_LIST))))

BASE:=base.tar.gz

# The link aliases for gcstools
GCS_TOOLS=\
	tar2vhd \
	vhd2tar \
	exportSandbox \
	netnscfg \
	remotefs

.PHONY: all always rootfs test

all: out/initrd.img out/rootfs.tar.gz out/rootfs2vhd.exe

test:
	cd $(SRCROOT) && go test ./service/gcsutils/...
	cd $(SRCROOT)/service/gcs && ginkgo -r -keepGoing

rootfs: .rootfs-done

.rootfs-done: init/init bin/vsockexec bin/gcs bin/gcstools Makefile
	rm -rf rootfs
	mkdir -p rootfs/bin/
	cp $(SRCROOT)/init/init rootfs/
	chmod 755 rootfs/init
	cp bin/vsockexec rootfs/bin/
	cp bin/gcs rootfs/bin/
	cp bin/gcstools rootfs/bin/
	for tool in $(GCS_TOOLS); do ln -s gcstools rootfs/bin/$$tool; done
	git -C $(SRCROOT) rev-parse HEAD > rootfs/gcs.commit && \
	git -C $(SRCROOT) rev-parse --abbrev-ref HEAD > rootfs/gcs.branch
	touch .rootfs-done
	ls -lR rootfs

out/rootfs.tar.gz: $(BASE) .rootfs-done
	@mkdir -p out
	# Append the added files to the base archive
	bsdtar -C rootfs -zcf $@ @$(abspath $(BASE)) .

out/initrd.img: out/rootfs.tar.gz
	# Convert from the rootfs tar to newc cpio
	bsdtar -zcf $@ --format newc @out/rootfs.tar.gz

out/rootfs2vhd.exe:
	# Compile rootfs2vhd for Windows
	env GOOS=windows GOARCH=amd64 go get -u github.com/Microsoft/hcsshim/internal/cmd/rootfs2vhd
	cp /go/bin/windows_amd64/rootfs2vhd.exe out/
	
bin/gcs.always: always
	@mkdir -p bin
	$(GO_BUILD) -o $@ github.com/Microsoft/opengcs/service/gcs

bin/gcstools.always: always
	@mkdir -p bin
	$(GO_BUILD) -o $@ github.com/Microsoft/opengcs/service/gcsutils/gcstools

VPATH=$(SRCROOT)

bin/vsockexec: vsockexec/vsockexec.o
	@mkdir -p bin
	$(CC) $(LDFLAGS) -o $@ $<

%.o: %.c
	@mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(CPPFLAGS) -c -o $@ $<

# For programs are always rebuilt, so don't update the actual result file if the
# result of the compilation did not change.
%: %.always
	@if ! cmp -s $@ $< ; then cp $< $@ ; fi
	@rm $<
