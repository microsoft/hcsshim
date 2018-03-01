.PHONY: all gobins clean

WORKDIR=$(abspath $(dir $(lastword $(MAKEFILE_LIST))))
BINDIR=$(WORKDIR)/bin

GO_PACKAGES=\
	./gcs \
	./gcsutils/gcstools

GCS_TOOLS=\
	tar2vhd \
	vhd2tar \
	exportSandbox \
	netnscfg \
	remotefs

GO_FLAGS=-pkgdir "$(WORKDIR)/pkg" -ldflags "-s -w"

all: gobins

gobins:
	(cd $(WORKDIR) && GOBIN="$(BINDIR)" CGO_ENABLED=0 go install $(GO_FLAGS) $(GO_PACKAGES))
	(cd $(BINDIR) && $(foreach tool,$(GCS_TOOLS), ln -f -s "gcstools" "$(tool)";))

clean:
	rm -f "$(BINDIR)/"*
