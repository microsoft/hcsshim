BASE:=base.tar.gz
DEV_BUILD:=0

GO:=go
GO_FLAGS:=-ldflags "-s -w" # strip Go binaries
CGO_ENABLED:=0
GOMODVENDOR:=

CFLAGS:=-O2 -Wall
LDFLAGS:=-static -s # strip C binaries

GO_FLAGS_EXTRA:=
ifeq "$(GOMODVENDOR)" "1"
GO_FLAGS_EXTRA += -mod=vendor
endif
GO_BUILD_TAGS:=
ifneq ($(strip $(GO_BUILD_TAGS)),)
GO_FLAGS_EXTRA += -tags="$(GO_BUILD_TAGS)"
endif
GO_BUILD:=CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GO_FLAGS) $(GO_FLAGS_EXTRA)

SRCROOT=$(dir $(abspath $(firstword $(MAKEFILE_LIST))))
# additional directories to search for rule prerequisites and targets
VPATH=$(SRCROOT)

DELTA_TARGET=out/delta.tar.gz

ifeq "$(DEV_BUILD)" "1"
DELTA_TARGET=out/delta-dev.tar.gz
endif

# The link aliases for gcstools
GCS_TOOLS=\
	generichook \
	install-drivers

# supply on the command line, eg 
#kegordo@kegordosurface5:~/work/oct16/src/github.com/Microsoft/hcsshim$ make GO_BUILD_TAGS=rego SRC=/home/kegordo/work/oct16 BASE=/home/kegordo/work/oct12/linux/core-image-minimal-aci-rootfs.tar snp all simple

SRC:=
VMGS_TOOL:=src/Parma/bin/vmgstool
#IGVM_TOOL:=src/Parma/kernel-files/5.15/igvmfile.py
IGVM_TOOL:=src/github.com/Microsoft/igvm-tooling/src/igvm/igvmgen.py
# this is now a 5.15 kernel
#KERNEL_PATH:=linux/linux/arch/x86/boot/bzImage
# 6.1 kernel
KERNEL_PATH:=linux/linux6.1/arch/x86/boot/bzImage

.PHONY: all always rootfs test

.DEFAULT_GOAL := all

all: out/initrd.img out/rootfs.tar.gz

clean:
	find -name '*.o' -print0 | xargs -0 -r rm
	rm -rf bin deps rootfs out

test:
	cd $(SRCROOT) && $(GO) test -v ./internal/guest/...

rootfs: out/rootfs.vhd

snp: out/kernelinitrd.vmgs out/containerd-shim-runhcs-v1.exe out/rootfs.hash.vhd out/rootfs.vhd out/v2056.vmgs out/v2056dm.vmgs

simple: out/simple.vmgs out/oldstyle.vmgs snp



#out/hash-device.vhd: out/hash_device
#	#cp out/hash_device $@
#	#./bin/cmd/dmverity-vhd -v convert --to-vhd --fst $@ -o foo
#	./bin/cmd/blob2vhd -i $< -o $@
#
#out/hash_device: out/rootfs.vhd
#	veritysetup format --no-superblock --salt 0000000000000000000000000000000000000000000000000000000000000000 out/rootfs.vhd $@ > out/rootfs.info
#	
#    # Retrieve info required by dm-verity at boot time
#    # Get the blocksize of rootfs
#	cat out/rootfs.info | awk '/^Root hash:/{ print $$3 }' > out/rootfs.rootdigest
#	cat out/rootfs.info | awk '/^Salt:/{ print $$2 }' > out/rootfs.salt
#	cat out/rootfs.info | awk '/^Data block size:/{ print $$4 }' > out/rootfs.datablocksize
#	cat out/rootfs.info | awk '/^Hash block size:/{ print $$4 }' > out/rootfs.hashblocksize
#	cat out/rootfs.info | awk '/^Data blocks:/{ print $$3 }' > out/rootfs.datablocks

	
%.hash %.hash.info %.hash.datablocks %.hash.rootdigest %.hash.salt %hash.datablocksize %.hash.datasectors %.hash.hashblocksize: %.ext4
	veritysetup format --no-superblock --salt 0000000000000000000000000000000000000000000000000000000000000000 $< $*.hash > $*.hash.info
    # Retrieve info required by dm-verity at boot time
    # Get the blocksize of rootfs
	cat $*.hash.info | awk '/^Root hash:/{ print $$3 }' > $*.hash.rootdigest
	cat $*.hash.info | awk '/^Salt:/{ print $$2 }' > $*.hash.salt
	cat $*.hash.info | awk '/^Data block size:/{ print $$4 }' > $*.hash.datablocksize
	cat $*.hash.info | awk '/^Hash block size:/{ print $$4 }' > $*.hash.hashblocksize
	cat $*.hash.info | awk '/^Data blocks:/{ print $$3 }' > $*.hash.datablocks
	echo $$(( $$(cat $*.hash.datablocks) * $$(cat $*.hash.datablocksize) / 512 )) > $*.hash.datasectors


## made by side effect of the above
# %.hash.datasectors: %.hash

# %.hash.datablocksize: %.hash

# %.hash.hashblocksize: %.hash

# %.hash.datablocks: %.hash

# %.hash.rootdigest: %.hash


# %.datasectors: %.info
# 	echo $$(( $$(cat $@.datablocks) * $$(cat $@.datablocksize) / 512 )) > $@.datasectors


# %.datablocksize: %.info
# 	cat $< | awk '/^Data block size:/{ print $$4 }' > $@

# %.hashblocksize: %.info
# 	cat $<| awk '/^Hash block size:/{ print $$4 }' > $@

# %.datablocks: %.info
# 	cat $< | awk '/^Data blocks:/{ print $$3 }' > $@

# %.rootdigest: %.info
# 	cat $< | awk '/^Root hash:/{ print $$3 }' > $@


%.vhd: % bin/cmd/blob2vhd
	./bin/cmd/blob2vhd -i $< -o $@	


%.vmgs: %.bin
	rm -f $@
	# du -BM returns the size of the bin file in M, eg 7M. The sed command replaces the M with *1024*1024 and then bc does the math to convert to bytes
	$(SRC)/$(VMGS_TOOL) create --filepath $@ --filesize `du -BM $< | sed  "s/M.*/*1024*1024/" | bc`
	$(SRC)/$(VMGS_TOOL) write --filepath $@ --datapath $< -i=8


ROOTFS_DEVICE:=/dev/sda
VERITY_DEVICE:=/dev/sdb
# ^^^ is this ok?
# SALT=$(shell cat out/rootfs.salt)
# ROOT_HASH=$(shell cat out/rootfs.hash)
# DATA_BLOCK_COUNT=$(shell cat out/rootfs.blockcount)
# DATA_BLOCK_SIZE=$(shell cat out/rootfs.datablocksize)
# HASH_BLOCK_SIZE=$(DATA_BLOCK_SIZE)
# NUM_SECTORS=$(shell cat out/rootfs.datasectors)

out/simple.bin: out/initrd.img $(SRC)/$(KERNEL_PATH) startup_simple.sh
	# easy case we know works to check the kernel is good without the complication of the dm-verity mounting via the kernel command line
	python3 $(SRC)/$(IGVM_TOOL) -o $@ -kernel $(SRC)/$(KERNEL_PATH) -append "8250_core.nr_uarts=0 panic=-1 debug loglevel=7 rdinit=/startup_simple.sh" -rdinit out/initrd.img -vtl 0

	
out/v2056.bin: out/rootfs.vhd out/rootfs.hash.vhd $(SRC)/$(KERNEL_PATH) out/rootfs.hash.datablocksize out/rootfs.hash.hashblocksize out/rootfs.hash.datablocks out/rootfs.hash.rootdigest out/rootfs.hash.salt startup_v2056.sh
	# easy case we know works to check the kernel is good without the complication of the dm-verity mounting via the kernel command line
	python3 $(SRC)/$(IGVM_TOOL) -o $@ -kernel $(SRC)/$(KERNEL_PATH) -append "8250_core.nr_uarts=0 panic=-1 debug loglevel=7 root=/dev/dm-0 dm-mod.create=\"jp1dmverityrfs,,,ro,0 $(shell cat out/rootfs.hash.datasectors) verity 1 $(ROOTFS_DEVICE) $(VERITY_DEVICE) $(shell cat out/rootfs.hash.datablocksize) $(shell cat out/rootfs.hash.hashblocksize) $(shell cat out/rootfs.hash.datablocks) 0 sha256 $(shell cat out/rootfs.hash.rootdigest) $(shell cat out/rootfs.hash.salt) 1 ignore_corruption\" init=/startup_v2056.sh"  -vtl 0

out/oldstyle.bin: out/initrd.img $(SRC)/$(KERNEL_PATH) startup.sh
	# easy case we know works to check the kernel is good without the complication of the dm-verity mounting via the kernel command line
	python3 $(SRC)/$(IGVM_TOOL) -o $@ -kernel $(SRC)/$(KERNEL_PATH) -append "8250_core.nr_uarts=0 panic=-1 debug loglevel=7 rdinit=/startup.sh" -rdinit out/initrd.img -vtl 0

out/v2056dm.bin:  out/rootfs.hash.datasectors out/rootfs.hash.datablocksize out/rootfs.hash.hashblocksize out/rootfs.hash.datablocks out/rootfs.hash.rootdigest out/rootfs.hash.salt $(SRC)/$(KERNEL_PATH) startup_v2056.sh
	rm -f $@ 
# experimental - works
	python3 $(SRC)/$(IGVM_TOOL) -o $@ -kernel $(SRC)/$(KERNEL_PATH) -append "8250_core.nr_uarts=0 panic=-1 debug loglevel=7 root=/dev/dm-0 dm-mod.create=\"jp1dmverityrfs,,,ro,0 $(shell cat out/rootfs.hash.datasectors) verity 1 $(ROOTFS_DEVICE) $(VERITY_DEVICE) $(shell cat out/rootfs.hash.datablocksize) $(shell cat out/rootfs.hash.hashblocksize) $(shell cat out/rootfs.hash.datablocks) 0 sha256 $(shell cat out/rootfs.hash.rootdigest) $(shell cat out/rootfs.hash.salt) 1 ignore_corruption\" rdinit=/startup_v2056.sh" -rdinit out/initrd.img  -vtl 0
    # Remember to REFORMAT the VHD WITH --no-superblock
    # dm-verity, <name> x
    # <blank>,   <uuid> x
    # 3,         <minor> x go blank
    # ro,        <flags> x
    # <TABLE>
    # 0        <start_sector> x
    # 1638400  <num_sectors>  x
    # verity   <target_type> x
    # <TARGET_ARGS>
    # 1          <version> x
    # /dev/sdc1  <dev>   @ROOTFS_DEVICE@ ???
    # /dev/sdc2  <hash_dev>   @VERITY_DEVICE@ ???
    # 4096       <data_block_size> x
    # 4096       <hash_block_size> x
    # 204800     <num_data_blocks> x
    # 1          <hash_start_block> x go with 0
    # sha256     <algorithm>  x
    # ac87db56303c9c1da433d7209b5a6ef3e4779df141200cbd7c157dcb8dd89c42 <digest>  x
    # 5ebfe87f7df3235b80a117ebc4078e44f55045487ad4a96581d1adb564615b51 <salt> x

out/kernelinitrd.bin: out/rootfs.vhd out/rootfs.hash.vhd out/rootfs.hash.datasectors out/rootfs.hash.datablocksize out/rootfs.hash.hashblocksize out/rootfs.hash.datablocks out/rootfs.hash.rootdigest out/rootfs.hash.salt $(SRC)/$(KERNEL_PATH) startup.sh
	rm -f $@
# works
	python3 $(SRC)/$(IGVM_TOOL) -o $@ -kernel $(SRC)/$(KERNEL_PATH) -append "8250_core.nr_uarts=0 panic=-1 debug loglevel=7 root=/dev/dm-0 dm-mod.create=\"jp1dmverityrfs,,,ro,0 $(shell cat out/rootfs.hash.datasectors) verity 1 $(ROOTFS_DEVICE) $(VERITY_DEVICE) $(shell cat out/rootfs.hash.datablocksize) $(shell cat out/rootfs.hash.hashblocksize) $(shell cat out/rootfs.hash.datablocks) 0 sha256 $(shell cat out/rootfs.hash.rootdigest) $(shell cat out/rootfs.hash.salt) 1 ignore_corruption\" init=/startup.sh"  -vtl 0


out/rootfs.ext4: out/rootfs.tar.gz bin/cmd/tar2ext4
	gzip -f -d ./out/rootfs.tar.gz
	./bin/cmd/tar2ext4 -i ./out/rootfs.tar -o $@

	
%.vhd: %.ext4 bin/cmd/blob2vhd
	./bin/cmd/blob2vhd -i $< -o $@

out/rootfs.tar.gz: out/initrd.img
	rm -rf rootfs-conv
	mkdir rootfs-conv
	gunzip -c out/initrd.img | (cd rootfs-conv && cpio -imd)
	tar -zcf $@ -C rootfs-conv .
	#rm -rf rootfs-conv

out/initrd.img: $(BASE) $(DELTA_TARGET) $(SRCROOT)/hack/catcpio.sh
	$(SRCROOT)/hack/catcpio.sh "$(BASE)" $(DELTA_TARGET) > out/initrd.img.uncompressed
	gzip -c out/initrd.img.uncompressed > $@
	rm out/initrd.img.uncompressed

# This target includes utilities which may be useful for testing purposes.
out/delta-dev.tar.gz: out/delta.tar.gz bin/internal/tools/snp-report
	rm -rf rootfs-dev
	mkdir rootfs-dev
	tar -xzf out/delta.tar.gz -C rootfs-dev
	cp bin/internal/tools/snp-report rootfs-dev/bin/
	tar -zcf $@ -C rootfs-dev .
	rm -rf rootfs-dev

out/delta.tar.gz: bin/init bin/vsockexec bin/cmd/gcs bin/cmd/gcstools bin/cmd/hooks/wait-paths Makefile  bin/internal/tools/snp-report bin/debuginit startup_v2056.sh startup_simple.sh startup.sh startup_2.sh
	@mkdir -p out
	rm -rf rootfs
	mkdir -p rootfs/bin/
	mkdir -p rootfs/info/
	cp bin/init rootfs/
	cp bin/debuginit rootfs/
	cp bin/vsockexec rootfs/bin/
	cp bin/cmd/gcs rootfs/bin/
	cp bin/cmd/gcstools rootfs/bin/
	cp bin/cmd/hooks/wait-paths rootfs/bin/
	cp startup_v2056.sh rootfs/startup_v2056.sh
	cp startup_simple.sh rootfs/startup_simple.sh
	cp startup.sh rootfs/startup.sh
	cp startup_2.sh rootfs/startup_2.sh
	cp bin/internal/tools/snp-report rootfs/bin/
	chmod a+x rootfs/startup_v2056.sh
	chmod a+x rootfs/startup_2.sh
	chmod a+x rootfs/startup_simple.sh
	chmod a+x rootfs/startup.sh
	for tool in $(GCS_TOOLS); do ln -s gcstools rootfs/bin/$$tool; done
	git -C $(SRCROOT) rev-parse HEAD > rootfs/info/gcs.commit && \
	git -C $(SRCROOT) rev-parse --abbrev-ref HEAD > rootfs/info/gcs.branch && \
	date --iso-8601=minute --utc > rootfs/info/tar.date
	$(if $(and $(realpath $(subst .tar,.testdata.json,$(BASE))), $(shell which jq)), \
		jq -r '.IMAGE_NAME' $(subst .tar,.testdata.json,$(BASE)) 2>/dev/null > rootfs/info/image.name && \
		jq -r '.DATETIME' $(subst .tar,.testdata.json,$(BASE)) 2>/dev/null > rootfs/info/build.date)
	tar -zcf $@ -C rootfs .
	#rm -rf rootfs

out/containerd-shim-runhcs-v1.exe:
	GOOS=windows $(GO_BUILD) -o $@ $(SRCROOT)/cmd/containerd-shim-runhcs-v1

bin/cmd/gcs bin/cmd/gcstools bin/cmd/hooks/wait-paths bin/cmd/tar2ext4 bin/internal/tools/snp-report bin/cmd/dmverity-vhd bin/cmd/blob2vhd:
	@mkdir -p $(dir $@)
	GOOS=linux $(GO_BUILD) -o $@ $(SRCROOT)/$(@:bin/%=%)

bin/vsockexec: vsockexec/vsockexec.o vsockexec/vsock.o
	@mkdir -p bin
	$(CC) $(LDFLAGS) -o $@ $^

bin/init: init/init.o vsockexec/vsock.o
	@mkdir -p bin
	$(CC) $(LDFLAGS) -o $@ $^

bin/debuginit: debuginit/debuginit.o vsockexec/vsock.o
	@mkdir -p bin
	$(CC) $(LDFLAGS) -o $@ $^


%.o: %.c
	@mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(CPPFLAGS) -c -o $@ $<
