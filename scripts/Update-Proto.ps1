$ErrorActionPreference = 'Stop'
$VerbosePreference = 'Continue'

$root = Split-Path -Path $PSScriptRoot -Parent

@('go', 'protoc') | ForEach-Object {
    if ( $null -eq (Get-Command $_ -CommandType Application -ErrorAction Ignore) ) {
        Write-Error -ErrorAction Stop "Missing executable: $_"
    }
}

function run([string]$cmd, [string[]]$params) {
    Write-Verbose "$cmd $params"
    & $cmd @params 2>&1
    if ( $LASTEXITCODE -ne 0 ) {
        Write-Error "Command failed: $cmd $params"
    }
}

# Install protobuild and co.
& (Join-Path $root '\tools\Install-Tools.ps1')

# difficult to exclude a directory from Get-ChildItem
Get-ChildItem -Filter *.proto -Recurse -Name -Path $root |
    Where-Object { ($_ -notlike 'vendor*') -and ($_ -notlike 'protobuf*') -and ($_ -notlike 'test*') } |
    ForEach-Object {
        $dir = Join-Path $root (Split-Path -Parent $_)
        $pkg = go list -f '{{.ImportPath}}' $dir

        run protobuild $pkg

        $acronyms = 'Id|Io|Guid|Uuid|Os'
        if ( $dir -like (Join-Path $root 'cmd\containerd-shim-runhcs-v1\stats') ) {
            $acronyms += '|Vm|Ns'
        }
        Get-ChildItem -Filter *.pb.go -Name -Path $dir |
            ForEach-Object {
                $f = Join-Path $dir $_
                run go-fix-acronym ('-w', '-a', "($acronyms)`$", $f)
            }
        }
