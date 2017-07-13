.PHONY: clean test-all test-tar2vhd test-exportSandbox

TESTDIR=test
INPUTPATH=$(TESTDIR)/input.tar
VHDPATH=$(TESTDIR)/input.vhd
OUTPUTPATH=$(TESTDIR)/output.tar
TESTFOLDER=$(TESTDIR)/folder

test-all: test-tar2vhd test-exportSandbox

test-tar2vhd:
	$(BINDIR)/tar2vhd < $(INPUTPATH) > $(VHDPATH)
	$(BINDIR)/vhd2tar < $(VHDPATH) > $(OUTPUTPATH)
	cmp $(INPUTPATH) $(OUTPUTPATH)
	rm -rf $(VHDPATH) $(OUTPUTPATH)

test-exportSandbox:
	mkdir -p $(TESTFOLDER)/upper
	tar xf $(INPUTPATH) -C $(TESTFOLDER)/upper
	$(BINDIR)/exportSandbox -path $(TESTFOLDER) > $(OUTPUTPATH)
	cmp $(INPUTPATH) $(OUTPUTPATH)
	rm -rf $(OUTPUTPATH) $(TESTFOLDER)
