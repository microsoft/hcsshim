cmake_minimum_required(VERSION 3.25)

project(hcsshim
    DESCRIPTION "project contains Golang interface for the Windows Host Compute Service and OCI shim used by containerd"
    LANGUAGES NONE
)

cmake_path(SET CMAKE_MODULE_PATH ${CMAKE_SOURCE_DIR}/cmake)
find_package(Go 1.24 REQUIRED
    OPTIONAL_COMPONENTS Linter
)

# NOTES:
# add_custom_target -> always out of date; we always call go since it has its own source tree dependency graph
# USES_TERMINAL -> go doesn't like being called in parallel (i.e., go fmt, go mod, go build), so force only one single instances

# TODO:
#  - update `.github\workflows\ci.yml` when Linux host support is finished
#  - update `FindGo.cmake` flow to use functions and better control flow
#  - use ctest to run `gotestsum`
#    - add `gotest.tools/gotestsum` as tool to go.mod
#  - once nested test go module is removed (or workspaces are enabled), remove go_mod/go_fmt functions
#  - use clang both as CGo backend/linker and manifest via /MANIFEST:EMBED /MANIFESTINPUT:
#  - build Linux init binaries regardless of host (use WSL on Windows?)
#  - make CMakeLists.txt next to each binary/proto target instead of consolidated one here
#  - dedicated protoc `custom_command`s that tracks source .proto file
#    - download protoc and protobuf imports
#    - may not need protobuild, since its configuration is somewhat limited (i.e., edition vs version)
#  - call scripts\New-ResourceObjectFile.ps1 (not in go generate so its tracked as output)
#    - break out manifest generation in CMake (if not via clang)
#  - call simplify scripts\Set-VersionInfo.ps1 logic (and reuse with manifest/resource file)
#    - look at WSL's findVersion.cmake

if(WIN32)
    # don't rely on GOEXE since that depends on GOOS
    set(binary_ext .exe)
endif()

set(go_common_env GOTOOLCHAIN=local GO111MODULE=on GOWORK=off)

# want the configuration-specific directory, but not the "build" directory
cmake_path(GET CMAKE_BINARY_DIR PARENT_PATH OUT_DIR)
# have one directory for both regular and testing binaries
cmake_path(SET BIN_DIR ${OUT_DIR}/bin)

cmake_path(SET CMAKE_RUNTIME_OUTPUT_DIRECTORY ${BIN_DIR})
# mostly for completeness's sake
cmake_path(SET CMAKE_ARCHIVE_OUTPUT_DIRECTORY ${OUT_DIR}/lib)
cmake_path(SET CMAKE_LIBRARY_OUTPUT_DIRECTORY ${OUT_DIR}/lib)

file(MAKE_DIRECTORY ${BIN_DIR})

message(STATUS "bin directory: ${BIN_DIR}")

if(Go_GOBIN)
    cmake_path(SET go_bin Go_GOBIN)
elseif(Go_GOPATH)
    cmake_path(SET go_bin "${Go_GOPATH}/bin")
else()
    message(FATAL_ERROR "Missing both 'GOPATH' and 'GOBIN' env settings")
endif()
message(STATUS "go bin directory: ${go_bin}")

# need pwsh for ./scripts
find_program(PWSH pwsh PATH_SUFFIXES PowerShell 7)
if (NOT EXISTS ${PWSH})
    message(FATAL_ERROR "Missing pwsh binary")
endif()
mark_as_advanced(PWSH)
message(VERBOSE "pwsh executable: ${PWSH}")

##########################################################################################
# go mod tidy and vendor
##########################################################################################

# catch all target for `go mod tidy/vendor` on entire repo
add_custom_target(go_mod ALL)
set_target_properties(go_mod PROPERTIES FOLDER validation)

# go_mod(name [VENDOR] [DEPENDS <depend>...])
function(go_mod name)
    cmake_parse_arguments(PARSE_ARGV 1 arg "VENDOR" "" "DEPENDS")

    string(PREPEND name "go_mod_")

    set(flags "-e")
    set(comment "tidying")
    if (arg_VENDOR)
        set(vendor_cmd ${CMAKE_COMMAND} -E env ${go_common_env} -- ${Go_EXECUTABLE} mod vendor ${flags})
        string(APPEND comment " and vendoring")
    endif()

    add_custom_target(${name}
        COMMAND ${CMAKE_COMMAND} -E env ${go_common_env} -- ${Go_EXECUTABLE} mod tidy -v ${flags}
        COMMAND ${vendor_cmd}
        DEPENDS "${arg_DEPENDS}"
        COMMENT "${comment} go module ${CMAKE_CURRENT_SOURCE_DIR}"
        WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
        VERBATIM
        USES_TERMINAL
    )

    add_dependencies(go_mod ${name})
    set_target_properties(${name} PROPERTIES FOLDER validation)

    message(VERBOSE "go mod ${name}")
endfunction()

go_mod(root VENDOR)

##########################################################################################
# go generate
##########################################################################################

# go generate (specifically, the New-ResourceObjectFile.ps1 calls) require the version to be set
add_custom_target(set_version
    COMMAND ${PWSH} -NonInteractive -NoProfile -NoLogo -Command "./scripts/Set-VersionInfo.ps1"
    COMMENT "run Set-Version script"
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    VERBATIM
)

# catch all target for `go generate` on entire repo
add_custom_target(go_gen ALL)
set_target_properties(go_gen PROPERTIES FOLDER generation)

function(go_gen name)
    string(PREPEND name "go_gen_")

    add_custom_target(${name}
        COMMAND ${CMAKE_COMMAND} -E env ${go_common_env} GOOS=windows -- ${Go_EXECUTABLE} generate -x ./...
        DEPENDS set_version
        COMMENT "running 'go generate' on ${CMAKE_CURRENT_SOURCE_DIR}"
        WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
        VERBATIM
        USES_TERMINAL
    )

    add_dependencies(go_gen ${name})
    set_target_properties(${name} PROPERTIES FOLDER generation)

    message(VERBOSE "go gen ${name}")
endfunction()

go_gen(root)

##########################################################################################
# lint
##########################################################################################

add_custom_target(lint_all)
set_target_properties(lint_all PROPERTIES FOLDER validation)

if (Go_Linter_FOUND)
    cmake_path(SET _GO_LINT_CONFIG ${CMAKE_SOURCE_DIR}/.golangci.yml)
    cmake_path(SET _GO_LINT_VALIDATE_CONFIG_TARGET ${CMAKE_BINARY_DIR}/CmakeFiles/.golangci)

    # validate the config, but only if its been modified
    add_custom_command(
        OUTPUT ${_GO_LINT_VALIDATE_CONFIG_TARGET}
        COMMAND ${Go_Linter_EXECUTABLE} config verify "--config=${_GO_LINT_CONFIG}"
        COMMAND ${CMAKE_COMMAND} -E touch ${_GO_LINT_VALIDATE_CONFIG_TARGET}
        DEPENDS ${_GO_LINT_CONFIG}
        COMMENT "validating linter config ${_GO_LINT_CONFIG}"
        VERBATIM
        USES_TERMINAL
    )
endif()

function(go_lint name)
    if (NOT Go_Linter_FOUND)
        message(WARNING "skipping go lint target \"${name}\" due to missing linter binary")
    endif()

    cmake_parse_arguments(PARSE_ARGV 1 arg "" "TARGET_OS" "TAGS")

    string(PREPEND name "go_lint_")

    parse_goos("${arg_TARGET_OS}" os)
    set(env ${go_common_env} "GOOS=${os}")
    set(comment_extra "")

    set(flags "--timeout=10m" "--max-issues-per-linter=0" "--max-same-issues=0"
        "--modules-download-mode=readonly" "--verbose" "--fix" "--path-mode=abs"
        "--config=")

    if (NOT "${arg_TAGS}" STREQUAL "")
        list(APPEND flags "--build-tags=$<JOIN:${arg_TAGS},,>")
        set(comment_extra " with tags ${arg_TAGS}")
    endif()

    add_custom_target(${name}
        COMMAND ${CMAKE_COMMAND} -E env ${env} -- ${Go_Linter_EXECUTABLE} run "${flags}"
        DEPENDS ${_GO_LINT_VALIDATE_CONFIG_TARGET}
        COMMENT "linting ${CMAKE_CURRENT_SOURCE_DIR} for ${os}${comment_extra}"
        WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
        COMMAND_EXPAND_LISTS
        VERBATIM
        USES_TERMINAL
    )
    add_dependencies(lint_all ${name})
    set_target_properties(${name} PROPERTIES FOLDER validation)

    message(VERBOSE "golangci-lint ${name}")
endfunction()

go_lint(root TAGS admin integration)
go_lint(root_lcow TAGS lcow admin integration)
go_lint(root_wcow TAGS wcow admin integration)
go_lint(root_linux TARGET_OS linux TAGS admin integration)

##########################################################################################
# protobuf
##########################################################################################

add_custom_target(proto
    COMMAND ${PWSH} -NonInteractive -NoProfile -NoLogo -Command "./scripts/Update-Proto.ps1"
    COMMENT building protofiles
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    VERBATIM
)
set_target_properties(proto PROPERTIES FOLDER generation)

##########################################################################################
# build (testing) binaries
##########################################################################################

add_custom_target(build_all
    COMMENT "build all binaries"
)
set_target_properties(build_all PROPERTIES FOLDER build)

add_custom_target(build_all_test
    COMMENT "build all testing binaries"
)
set_target_properties(build_all_test PROPERTIES FOLDER build/test)

# go_build(package [NAME <name>] [TARGET_OS <WINDOWS|LINUX>] [TEST] [RACE] [COVERAGE] [DEPENDS <depend>...] [TAGS <tags>...])
function(go_build package)
    set(options TEST RACE COVERAGE)
    set(oneValueArgs NAME TARGET_OS BINARY_NAME)
    set(multiValueArgs DEPENDS TAGS)
    cmake_parse_arguments(PARSE_ARGV 1 arg "${options}" "${oneValueArgs}" "${multiValueArgs}")


    if ("${arg_BINARY_NAME}" STREQUAL "")
        cmake_path(GET package FILENAME binary)
    else()
        set(binary ${arg_BINARY_NAME})
    endif()

    cmake_path(SET package "${CMAKE_CURRENT_SOURCE_DIR}/${package}")
    cmake_path(SET out_dir "${BIN_DIR}")

    # for now, building will always require running `go gen` to create the needed syso files
    # will have to move those targets directly into CMake
    list(PREPEND arg_DEPENDS "set_version" "go_mod_root" "go_gen")

    parse_goos("${arg_TARGET_OS}" os)
    set(env ${go_common_env} "GOOS=${os}")

    if (arg_TEST)
        set(go_cmd "test")
        string(APPEND binary ".test")

        list(APPEND flags "-c" "-gcflags=all=-d=checkptr")

        set(comment_type "testing binary")
    else()
        set(go_cmd "build")

        list(APPEND ld_flags "$<$<NOT:$<CONFIG:Debug>>:-s;-w>")
        list(APPEND flags "$<$<NOT:$<CONFIG:Debug>>:-trimpath>")

        set(comment_type "binary")
    endif()

    if (arg_RACE)
        list(APPEND flags "-race")
        list(APPEND env "CGO_ENABLED=1")
    endif()

    if (os STREQUAL "windows")
        string(APPEND binary ".exe")
        list(APPEND ld_flags "-linkmode=external")
    endif()

    cmake_path(SET out "${out_dir}/${binary}")

    if (arg_COVERAGE)
        # only enable coverage for debug builds
        list(APPEND flags "$<$<CONFIG:Debug>:-cover;-coverpkg=\"github.com/Microsoft/hcsshim/...\">")
    endif()

    if (NOT "${ld_flags}" STREQUAL "")
        list(APPEND flags "-ldflags=$<JOIN:${ld_flags}, >")
    endif()

    if (NOT "${arg_TAGS}" STREQUAL "")
        list(APPEND flags "-tags=$<JOIN:${arg_TAGS},,>")
    endif()

    if ("${arg_NAME}" STREQUAL "")
        set(arg_NAME ${binary})
    endif()

    add_custom_target(${arg_NAME}
        BYPRODUCTS ${out}
        COMMAND ${CMAKE_COMMAND} -E env ${env} -- ${Go_EXECUTABLE} ${go_cmd} "${flags}" -o=${out}
        DEPENDS ${arg_DEPENDS}
        COMMENT "building go ${comment_type} ${binary}"
        WORKING_DIRECTORY "${package}"
        COMMAND_EXPAND_LISTS
        VERBATIM
        USES_TERMINAL
    )

    if (arg_TEST)
        add_dependencies(build_all_test ${arg_NAME})
        set_target_properties(${arg_NAME} PROPERTIES FOLDER build/test)
    else()
        add_dependencies(build_all ${arg_NAME})
        set_target_properties(${arg_NAME} PROPERTIES FOLDER build)
    endif()

    message(STATUS "go ${comment_type} ${arg_NAME}: ${out}")
endfunction()

go_build(cmd/containerd-shim-runhcs-v1 NAME shim)
go_build(cmd/containerd-shim-lcow-v2 TAGS lcow NAME shim-lcow)
go_build(cmd/device-util)
go_build(cmd/gcs TARGET_OS linux)
go_build(cmd/gcs-sidecar)
go_build(cmd/gcstools TARGET_OS linux)
go_build(cmd/hooks/wait-paths TARGET_OS linux)
go_build(cmd/jobobject-util)
go_build(cmd/mkuvmcim)
go_build(cmd/ncproxy)
go_build(cmd/runhcs)
go_build(cmd/shimdiag)
go_build(cmd/tar2ext4)
go_build(cmd/tar2ext4 TARGET_OS linux NAME tar2ext4)
go_build(cmd/wclayer)

go_build(internal/tools/extendedtask)
go_build(internal/tools/grantvmgroupaccess)
go_build(internal/tools/hvsocketaddr)
go_build(internal/tools/networkagent)
go_build(internal/tools/policyenginesimulator)
go_build(internal/tools/policyenginesimulator TARGET_OS linux)
go_build(internal/tools/rootfs)
go_build(internal/tools/rootfs TARGET_OS linux)
go_build(internal/tools/securitypolicy)
go_build(internal/tools/securitypolicy TARGET_OS linux)
go_build(internal/tools/snp-report TARGET_OS linux)
go_build(internal/tools/uvmboot)
go_build(internal/tools/zapdir)

##########################################################################################
# validation
##########################################################################################

add_custom_target(validate DEPENDS go_mod go_gen proto lint_all)
set_target_properties(validate PROPERTIES FOLDER validation)

##########################################################################################
# add test directory after the custom commands and functions have been defined
##########################################################################################

add_subdirectory(test)
