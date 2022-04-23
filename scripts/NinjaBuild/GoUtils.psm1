
Import-Module (Join-Path $PSScriptRoot Utils) -Scope Local -Force

function Get-GoGenPackage {
    [CmdletBinding()]
    [OutputType([string[]])]
    param (
        [string]
        $Module = '.'
    )

    $Module = Resolve-Path $Module
    Write-Verbose "Searching for go pacakges with `"//go:generate`" directives in `"$Module`""

    $gens = foreach ( $d in Get-GoFile -Module $Module ) {
        foreach ($p in Get-ChildItem -Path $d -Filter *.go -Recurse -Name) {
            if ( $p ) {
                $p = Join-Path $d $p
                if ( Select-String -Path $p -Pattern '^//go:generate' -CaseSensitive -List -Quiet ) {
                    Split-Path -Parent $p
                }
            }
        }
    }

    return [string[]]($gens | Get-Unique)
}

function Get-ProtoFile {
    [CmdletBinding()]
    [OutputType([string[]])]
    param (
        [string]
        $Module = '.'
    )

    $Module = Resolve-Path $Module
    Write-Verbose "Searching for `"*.proto`" files in $Module"

    $protos = foreach ($d in Get-GoFile -Module $Module) {
        foreach ( $f in Get-ChildItem -Path $d -Filter *.proto -Recurse -Name ) {
            if ( $f ) {
                Join-Path $d $f
            }
        }
    }

    return [string[]]($protos | Get-Unique)
}

function Get-GoFile {
    [CmdletBinding()]
    [OutputType([string[]])]
    param (
        [Parameter(Mandatory)]
        [string]
        $Module
    )

    # dont use .FullName, need relative to module path

    [string[]] $a = foreach ( $p in Get-ChildItem -Path . -Exclude .git, .github, .vscode, bin, deps, hack, out, protobuf, scripts, vendor ) {
        if ( $p.Attributes -eq [System.IO.FileAttributes]::Directory ) {
            foreach ($f in Get-ChildItem -Path $p -Exclude vendor -Name) {
                Join-Path $p.Name $f
            }
        } else {
            $p.Name
        }
    }
    return $a
}

# this is a really bad idea, since this will not take into account changes to
# vendoring that do not affect go.mod (if that is at all possible), nor does it
# consider replace directives
# just use go build and its internal caching-logic
#
# todo, add better logic for tests and use the `.TestGoFiles` field for the
# original test directory
#
# examples:
#  Get-GoPackageDependency -vb -Package .\cmd\tar2ext4\
#  Get-GoPackageDependency -vb -Package .\internal\guest\runtime -GOOS 'linux'
#  Get-GoPackageDependency -vb -Package .\cmd\gcs -GOOS 'linux'
#  Get-GoPackageDependency -vb -Package .\test\cri-containerd -Tags "functional,test"

function Get-GoPackageDependency {
    [CmdletBinding()]
    [OutputType([string[]])]
    param (
        [Parameter(Mandatory)]
        [string]
        $Package,

        [string]
        $Tags,

        [ValidateSet('windows', 'linux')]
        [string]
        $GOOS = 'windows'
    )

    # $Package = Resolve-Path $Package
    $listcmd = @('list', "-tags=`'$tags`'", '-f' )

    $Module = (Invoke-GoCommand -GOOS $GOOS @listcmd '''{{ if .Module }}{{ .Module.Dir }}{{ end }}''' $Package)
    if ( -not $Module ) {
        return $null
    }

    $modname = (Invoke-GoCommand -GOOS $GOOS @listcmd '''{{ .ImportPath }}''' $Module)
    if ( -not $modname ) {
        # try the parent directory (for .\test)
        $Module = "$((Get-Item $Module).Parent)"
        $modname = (Invoke-GoCommand -GOOS $GOOS @listcmd '''{{ .ImportPath }}''' $Module)
        if ( -not $modname.Length ) {
            Write-Error "Could not get module import name for $Module"
            return $null
        }
    }
    Write-Verbose "Listing dependent files for package `"$Package`" in module `"$modname`""

    # Write-Verbose "got module location $Module"
    $gomodpath = (Invoke-GoCommand -GOOS $GOOS @listcmd '''{{ if .Module }}{{ .Module.GoMod }}{{ end }}''' $Package)

    $deps = ( Invoke-GoCommand -GOOS $GOOS @listcmd '''{{ .ImportPath }}{{ \"\n\" }}{{ join .Deps \"\n\"  }}''' "$Package" ) |
        Where-Object { $_ -and $_ -is [string] -and $_.StartsWith($modname) } |
        ForEach-Object { Invoke-GoCommand -GOOS $GOOS @listcmd '''{{ $dir := .Dir }}{{ range .GoFiles }}{{ $dir }}\{{ . }}{{ \"\n\" }}{{ end }}''' $_ }

    return [string[]]@($gomodpath, $deps)
}

function Get-GoModuleName {
    [CmdletBinding()]
    [OutputType([string])]
    param (
        [Parameter(Mandatory)]
        [string]
        $Path,

        [string]
        $Tags,

        [ValidateSet('windows', 'linux')]
        [string]
        $GOOS = 'windows',

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference')
    )

    Invoke-GoCommand -GOOS $GOOS 'list' "-tags=`'$Tags`'" '-f' '{{ .ImportPath }}' $Path
}

function Get-GoEnv {
    [CmdletBinding()]
    [OutputType([string])]
    param (
        [string]
        $Env = '',

        [ValidateSet('windows', 'linux')]
        [string]
        $GOOS = 'windows',

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference')
    )

    Invoke-GoCommand -GOOS $GOOS env ($Env.ToUpper())
}

function Invoke-GoCommand {
    [CmdletBinding(PositionalBinding = $False,
        SupportsShouldProcess)]
    [OutputType([string[]])]
    param (
        [Parameter(Mandatory,
            Position = 0,
            ValueFromRemainingArguments)]
        [AllowEmptyString()]
        [string[]]
        $Command,

        [ValidateSet('windows', 'linux')]
        [string]
        $GOOS = 'windows'
    )

    $Command = $Command | Get-NonEmpty
    $line = { param([string]$OS, [string[]]$cmd)
        $env:GOOS = $OS
        # Invoke-Expression @('go', $cmd) | Join-String -Separator ' ')
        go @cmd
    }
    if ( $PSCmdlet.ShouldProcess("Executing `"`$env:GOOS='$GOOS' ; go $Command`"", '.', 'Invoke-GoCommand') ) {
        pwsh -NoProfile -NoLogo -NonInteractive -Command $line -args $GOOS, $Command
    }
}