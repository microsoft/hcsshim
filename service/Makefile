.PHONY: all gobins clean

WORKDIR=$(abspath $(dir $(lastword $(MAKEFILE_LIST))))
BINDIR=$(WORKDIR)/bin

GO_PACKAGES=\
	./gcs \
	./gcsutils/gcstools

GCS_TOOLS=\
	tar2vhd \
	vhd2tar \
	createSandbox \
	exportSandbox

# For now, disable optimizations to improve debugging.
GO_FLAGS=-gcflags "-N -l"

all: gobins

gobins:
	(cd $(WORKDIR) && GOBIN="$(BINDIR)" CGO_ENABLED=0 go install $(GO_FLAGS) $(GO_PACKAGES))
#	mv "$(BINDIR)/gogcs" "$(BINDIR)/gcs" # Fix name for gcs until the directory name changes
	(cd $(BINDIR) && $(foreach tool,$(GCS_TOOLS), ln -f "gcstools" "$(tool)";))

clean:
	rm -f "$(BINDIR)/"*
