# TODO: Add this a parameter for debugging. ie "functional-tests -debug=$true"
#$env:HCSSHIM_FUNCTIONAL_TESTS_DEBUG="yes please"
go test -v -tags "functional create sandbox scsi vpmem vsmb p9"