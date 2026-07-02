#
# cmake for test directory, to be imported from root directory
#

##########################################################################################
# general validation
##########################################################################################

go_mod(test DEPENDS go_mod_root)
go_gen(test)
go_lint(test TAGS functional)
go_lint(test_linux TARGET_OS linux TAGS functional)

##########################################################################################
# build go test binaries
##########################################################################################

go_build(containerd-shim-runhcs-v1 TEST TAGS functional DEPENDS go_mod_test NAME shim_test)
go_build(cri-containerd TEST TAGS functional DEPENDS go_mod_test NAME cri_test)
go_build(functional TEST TAGS functional DEPENDS go_mod_test NAME func_test)
go_build(runhcs TEST TAGS functional DEPENDS go_mod_test NAME runhcs_test)
go_build(gcs TEST TARGET_OS linux TAGS functional DEPENDS go_mod_test NAME gcs_test)
go_build(cri-containerd/helpers DEPENDS go_mod_test BINARY_NAME sample-logging-driver)