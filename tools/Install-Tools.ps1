$ErrorActionPreference = 'Stop'
$VerbosePreference = 'Continue'

# make sure we are running from inside the tools module
Push-Location $PSScriptRoot
try {
    # Install protobuild and co.
    # calling `go install` without a version will install from current module, using version from `tools/go.mod`
    @(
        'github.com/containerd/protobuild',
        'github.com/containerd/protobuild/cmd/go-fix-acronym',
        'github.com/containerd/ttrpc/cmd/protoc-gen-go-ttrpc',
        'google.golang.org/grpc/cmd/protoc-gen-go-grpc',
        'google.golang.org/protobuf/cmd/protoc-gen-go'
        'github.com/josephspurrier/goversioninfo/cmd/goversioninfo',
        'github.com/Microsoft/go-winio/tools/mkwinsyscall',
        'github.com/golang/mock/mockgen'
    ) | ForEach-Object {
        Write-Verbose "go install $_"
        & go install $_

        if ( $LASTEXITCODE -ne 0 ) {
            Write-Error "Failed to install tool: $_"
        }
    }
} finally {
    Pop-Location
}
