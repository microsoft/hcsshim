#!/bin/sh
# Creates Makefile rules that enumerate all the input files
# for the Go package provided on the command line.

set -e

target_bin="$1"
target_pkg="$2"
shift 2
fmt=$(cat <<EOF
{{ if not .Standard }}
$target_bin: {{\$dir := .Dir}}{{ range .GoFiles }}{{\$dir}}/{{.}} {{end}}
{{if .Module}}$target_bin: {{.Module.GoMod}}{{end}}
{{end}}
EOF
)

go list "$@" -f '{{ .ImportPath }} {{ join .Deps "\n" }}' "$target_pkg" |
    xargs go list "$@" -find -f "$fmt"
