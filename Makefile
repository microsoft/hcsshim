BASE:=base.tar.gz

GO:=go
GO_FLAGS:=-ldflags "-s -w" # strip Go binaries

CGO_ENABLED:=0
CFLAGS:=-O2 -Wall
LDFLAGS:=-static -s # strip C binaries

GO_BUILD=CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GO_FLAGS)
SRCROOT=$(dir $(abspath $(lastword $(MAKEFILE_LIST))))

# The link aliases for gcstools
GCS_TOOLS=\
	vhd2tar \
	exportSandbox \
	netnscfg \
	remotefs

.PHONY: all always rootfs test

all: out/initrd.img out/rootfs.tar.gz

clean:
	find -name '*.o' -print0 | xargs -0 rm
	rm bin/*

test:
	cd $(SRCROOT) && go test ./service/gcsutils/...
	cd $(SRCROOT)/service/gcs && ginkgo -r -keepGoing

out/delta.tar.gz: bin/init bin/vsockexec bin/gcs bin/gcstools Makefile
	@mkdir -p out
	rm -rf rootfs
	mkdir -p rootfs/bin/
	cp bin/init rootfs/
	cp bin/vsockexec rootfs/bin/
	cp bin/gcs rootfs/bin/
	cp bin/gcstools rootfs/bin/
	for tool in $(GCS_TOOLS); do ln -s gcstools rootfs/bin/$$tool; done
	git -C $(SRCROOT) rev-parse HEAD > rootfs/gcs.commit && \
	git -C $(SRCROOT) rev-parse --abbrev-ref HEAD > rootfs/gcs.branch
	tar -zcf $@ -C rootfs .
	rm -rf rootfs

out/rootfs.tar.gz: out/initrd.img
	bsdtar -zcf $@ @out/initrd.img

out/initrd.img: $(BASE) out/delta.tar.gz $(SRCROOT)/hack/catcpio.sh
	$(SRCROOT)/hack/catcpio.sh "$(BASE)" out/delta.tar.gz > out/initrd.img.uncompressed
	gzip -c out/initrd.img.uncompressed > $@
	rm out/initrd.img.uncompressed

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

bin/init: init/init.o
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
