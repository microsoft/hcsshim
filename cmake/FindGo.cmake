# based heavily on
# https://github.com/corrosion-rs/corrosion/blob/14d5ecf8b85e15abc19f24f309df81f5f1d0d7ca/cmake/FindGo.cmake

#[=======================================================================[.rst:
FindGo
--------

Find Go

This module finds a golang installation, along with other components (eg, golangci-lint).

You can override the location by setting the `Go_EXECUTABLE` (or other component) variable
in your `UserConfig.cmake` file.

Imported Targets
^^^^^^^^^^^^^^^^

``Go::go``
  The golang binary.
``Go::linter``
  The golangci-lint linter.

Result Variables
^^^^^^^^^^^^^^^^

``Go_FOUND``
  True if golang is installed.
``Go_EXECUTABLE``
  Path to go executable.
``Go_VERSION``
  The go version.

``Go_GOBIN``, ``Go_GOPATH``, ``Go_GOROOT``
  Golang environment variables.

``Go_Linter_FOUND``
  True if golangci-lint is installed.
``Go_Linter_EXECUTABLE``
  Path to the golangci-lint executable.
``Go_LINTER_VERSION``
  The golangci-lint executable version.

#]=======================================================================]

# TODO:
#  - use `message(CHECK_* ...) for better reporting
#     https://cmake.org/cmake/help/latest/command/message.html#reporting-checks

cmake_minimum_required(VERSION 3.25)

##########################################################################################
# finding Go/Linter binaries
##########################################################################################

#
# setup
#

include(FindPackageHandleStandardArgs)

macro(_find_go_message)
    if (NOT "${Go_FIND_QUIETLY}")
        message(${ARGN})
    endif()
endmacro()

# error and return.
macro(_find_go_fail)
    if("${Go_FIND_REQUIRED}")
        message(FATAL_ERROR ${ARGN})
    else()
        _find_go_message(WARNING ${ARGN})
    endif()
    return()
endmacro()

#
# go
#

find_program(Go_EXECUTABLE go HINTS ENV GOROOT)
_find_go_message(STATUS "Go_EXECUTABLE: ${Go_EXECUTABLE}")

if (NOT EXISTS ${Go_EXECUTABLE})
    _find_go_fail("Failed to find Go binary")
endif()

execute_process(
    COMMAND "${Go_EXECUTABLE}" version
    OUTPUT_VARIABLE _GO_VERSION_RAW
    RESULT_VARIABLE _GO_VERSION_RESULT
)
if(NOT ( "${_GO_VERSION_RESULT}" EQUAL "0" ))
    _find_go_fail("Failed to get go version: `${Go_EXECUTABLE} version` failed with: ${_GO_VERSION_RAW}")
endif()

if (_GO_VERSION_RAW MATCHES "^go version go([0-9]+)\\.([0-9]+)\\.([0-9]+)")
    _find_go_message(VERBOSE "parsed go version string: ${_GO_VERSION_RAW}")
    set(Go_VERSION_MAJOR "${CMAKE_MATCH_1}")
    set(Go_VERSION_MINOR "${CMAKE_MATCH_2}")
    set(Go_VERSION_PATCH "${CMAKE_MATCH_3}")
    set(Go_VERSION "${Go_VERSION_MAJOR}.${Go_VERSION_MINOR}.${Go_VERSION_PATCH}")
    _find_go_message(STATUS "Go_VERSION: ${Go_VERSION}")
else()
    _find_go_fail("Failed to parse go version string from `${Go_EXECUTABLE} version`: ${_GO_VERSION_RAW}")
endif()

function(_find_go_get_env var name is_path)
    execute_process(
        COMMAND ${Go_EXECUTABLE} env ${name}
        OUTPUT_VARIABLE ${var}
    )

    string(STRIP ${${var}} ${var})
    if( ${var} AND ${is_path} )
        cmake_path(SET ${var} ${${var}})
    endif()

    _find_go_message(STATUS "${var}: ${${var}}")

    return(PROPAGATE ${var})
endfunction()

_find_go_get_env(Go_GOROOT GOROOT TRUE)
_find_go_get_env(Go_GOPATH GOPATH TRUE)
_find_go_get_env(Go_GOBIN GOBIN TRUE)

if(NOT TARGET Go::go)
    add_executable(Go::go IMPORTED GLOBAL)
    set_property(
        TARGET Go::go
        PROPERTY IMPORTED_LOCATION "${Go_EXECUTABLE}"
    )
endif()

# hide the Go_EXECUTABLE cache variable from users
mark_as_advanced(Go_EXECUTABLE)

#
# components
#

set(_go_comps ${Go_FIND_COMPONENTS})

# Linter

if(Linter IN_LIST _go_comps)
    list(REMOVE_ITEM _go_comps Linter)

    _find_go_message(VERBOSE "Finding Linter `golangci-lint`")
    find_program(Go_Linter_EXECUTABLE golangci-lint HINTS ENV GOBIN "${Go_GOPATH}/bin")
    _find_go_message(STATUS "Go_Linter_EXECUTABLE: ${Go_Linter_EXECUTABLE}")
endif()

if (EXISTS ${Go_Linter_EXECUTABLE})
    execute_process(
        COMMAND "${Go_Linter_EXECUTABLE}" --version
        OUTPUT_VARIABLE _GO_LINTER_VERSION_RAW
        RESULT_VARIABLE _GO_LINTER_VERSION_RESULT
    )
    if(NOT ( "${_GO_LINTER_VERSION_RESULT}" EQUAL "0" ))
        _find_go_fail("Failed to get golangci-lint version: `${Go_Linter_EXECUTABLE} version` failed with: ${_GO_LINTER_VERSION_RAW}")
    endif()

    if (_GO_LINTER_VERSION_RAW MATCHES "^golangci-lint has version [v]?([0-9]+)\\.([0-9]+)\\.([0-9]+)")
        _find_go_message(VERBOSE "parsed golangci-lint version string: ${_GO_LINTER_VERSION_RAW}")
        set(Go_Linter_VERSION_MAJOR "${CMAKE_MATCH_1}")
        set(Go_Linter_VERSION_MINOR "${CMAKE_MATCH_2}")
        set(Go_Linter_VERSION_PATCH "${CMAKE_MATCH_3}")
        set(Go_Linter_VERSION "${Go_Linter_VERSION_MAJOR}.${Go_Linter_VERSION_MINOR}.${Go_Linter_VERSION_PATCH}")
        _find_go_message(STATUS "Go_Linter_VERSION: ${Go_Linter_VERSION}")
    else()
        _find_go_fail("Failed to parse golangci-lint version string from `${Go_Linter_EXECUTABLE} version`: ${_GO_LINTER_VERSION_RAW}")
    endif()

    if(NOT TARGET Go::linter)
        add_executable(Go::linter IMPORTED GLOBAL)
        set_property(
            TARGET Go::linter
            PROPERTY IMPORTED_LOCATION "${Go_Linter_EXECUTABLE}"
        )
    endif()
    # for find_package_handle_standard_args
    set(Go_Linter_FOUND TRUE)

    # hide the Go_Linter_EXECUTABLE cache variable from users
    mark_as_advanced(Go_Linter_EXECUTABLE)
endif()

#
# cleanup
#

if(_go_comps)
    list(JOIN _go_comps ", " _c)
    _find_go_fail("unknown components: ${_c}")
endif()

##########################################################################################
# helper functions
##########################################################################################

# parse_goos(<input> <output_variable>)
# set `<output_variable>` to the canonicalized GOOS string, or error if the OS is unsupported
function(parse_goos in_goos out_var)
    if (in_goos STREQUAL "")
        set(in_goos windows)
    endif()
    string(TOLOWER "${in_goos}" in_goos)

    set(_supported_os linux windows)
    if (NOT in_goos IN_LIST _supported_os)
        message(FATAL_ERROR "unsupported GOOS value: ${in_goos}")
    endif()

    if (NOT out_var STREQUAL "")
        set(${out_var} "${in_goos}" PARENT_SCOPE)
    endif()
endfunction()

find_package_handle_standard_args(Go
    REQUIRED_VARS Go_EXECUTABLE
    VERSION_VAR Go_VERSION
    HANDLE_COMPONENTS
)
