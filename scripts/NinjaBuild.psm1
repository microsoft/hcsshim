# see details https://ninja-build.org/manual.html
# based off of https://github.com/ninja-build/ninja/blob/master/misc/ninja_syntax.py

Set-Variable EmptyStringArray -WhatIf:$false -Option Constant -Value ([string[]]'')
Set-Variable CommentPrefix -WhatIf:$false -Option Constant -Value ([string[]]'#')
Set-Variable PoolPrefix -WhatIf:$false -Option Constant -Value ([string[]]'pool')
Set-Variable RulePrefix -WhatIf:$false -Option Constant -Value ([string[]]'rule')
Set-Variable BuildPrefix -WhatIf:$false -Option Constant -Value ([string[]]'build')
Set-Variable IncludePrefix -WhatIf:$false -Option Constant -Value ([string[]]'include')
Set-Variable SubNinjaPrefix -WhatIf:$false -Option Constant -Value ([string[]]'subninja')
Set-Variable DefaultPrefix -WhatIf:$false -Option Constant -Value ([string[]]'default')

Set-Variable IndentValue -WhatIf:$false -Option Constant -Value '  '
Set-Variable DefaultLineWidth -WhatIf:$false -Option Constant -Value 80

# build and rule parameter sets can be undistinguishable, so often a decsription or depfile is needed

<#
.SYNOPSIS

Updates a ninja build with a declaration.

.DESCRIPTION

Appends a declations (new line, commend, pool, rule, build, etc.) to the specified
ninja build file ("ninja.build") be default.

.LINK

https://ninja-build.org/manual.html

#>
function Update-NinjaFile {
    [CmdletBinding(PositionalBinding = $False,
        DefaultParameterSetName = 'NewLine',
        SupportsShouldProcess)]
    [OutputType([string])]
    param (
        # Newline

        [Parameter(ParameterSetName = 'Comment')]
        [Parameter(ParameterSetName = 'Variable')]
        [Parameter(ParameterSetName = 'Pool')]
        [Parameter(ParameterSetName = 'Rule')]
        [Parameter(ParameterSetName = 'Build')]
        [Parameter(ParameterSetName = 'Include')]
        [Parameter(ParameterSetName = 'SubNinja')]
        [Parameter(ParameterSetName = 'Default')]
        [Parameter(ParameterSetName = 'NewLine', Mandatory)]
        [Alias('nl')]
        [switch]
        # Add a newline.
        $NewLine,

        # Comment

        [Parameter(ParameterSetName = 'Comment', Mandatory)]
        [switch]
        # Add a comment.
        $Comment,

        # Variable

        [Parameter(ParameterSetName = 'Variable', Mandatory)]
        [string]
        # Add a variable declaration.
        $Variable,

        [Parameter(ParameterSetName = 'Variable')]
        [int]
        $Indent,

        # Pool

        [Parameter(ParameterSetName = 'Rule')]
        [Parameter(ParameterSetName = 'Build')]
        [Parameter(ParameterSetName = 'Pool', Mandatory)]
        [string]
        # Add a pool declaration, or specify the pool for a rule or build.
        $Pool,

        [Parameter(ParameterSetName = 'Pool', Mandatory)]
        [int]
        $Depth,

        # Rule

        [Parameter(ParameterSetName = 'Build', Mandatory)]
        [Parameter(ParameterSetName = 'Rule', Mandatory)]
        [string]
        # Add a rule declaration, or specify the rule for a build.
        $Rule,

        [Parameter(ParameterSetName = 'Rule')]
        [string[]]
        $Description = '',

        [Parameter(ParameterSetName = 'Rule')]
        [string]
        $Depfile = '',

        [Parameter(ParameterSetName = 'Rule')]
        [switch]
        $Generator,

        [Parameter(ParameterSetName = 'Rule')]
        [switch]
        $Restat,

        [Parameter(ParameterSetName = 'Rule')]
        [string]
        $Rspfile = '',

        [Parameter(ParameterSetName = 'Rule')]
        [string[]]
        $Rspfile_content = '',

        [Parameter(ParameterSetName = 'Rule')]
        [ValidateSet('gcc', 'msvc')]
        [string]
        $Deps = '',

        # Build

        [Parameter(ParameterSetName = 'Build', Mandatory)]
        [string[]]
        # Add a build declaration
        $Build,

        [Parameter(ParameterSetName = 'Build')]
        [string[]]
        $Implicit,

        [Parameter(ParameterSetName = 'Build')]
        [string[]]
        $OrderOnly,

        [Parameter(ParameterSetName = 'Build')]
        [hashtable]
        $Variables,

        [Parameter(ParameterSetName = 'Build')]
        [string[]]
        $ImplicitOutputs,

        [Parameter(ParameterSetName = 'Build')]
        [string]
        $DynDep,

        # Include

        [Parameter(ParameterSetName = 'Include', Mandatory)]
        [string]
        # Add a include declaration
        $Include,

        # SubNinja

        [Parameter(ParameterSetName = 'SubNinja', Mandatory)]
        [string]
        # Add a subninja declaration
        $SubNinja,

        # Default

        [Parameter(ParameterSetName = 'Build')]
        [Parameter(ParameterSetName = 'Default', Mandatory)]
        [switch]
        # Add a default declaration, are sets a build declaration as default
        $Default,

        [Parameter(ParameterSetName = 'Comment',
            Position = 0,
            ValueFromRemainingArguments)]
        [Parameter(ParameterSetName = 'Variable',
            Position = 0,
            ValueFromRemainingArguments)]
        [Parameter(ParameterSetName = 'Build',
            Position = 0,
            ValueFromRemainingArguments)]
        [Parameter(ParameterSetName = 'Rule',
            Position = 0,
            ValueFromRemainingArguments)]
        [Parameter(ParameterSetName = 'Default',
            Position = 0,
            ValueFromRemainingArguments)]
        [string[]]
        # The value of the comment, variable, build, rule, or default declaration
        $Value,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('q')]
        [switch]
        # Supress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    process {
        [string[]]$Value = $Value | Get-NonEmpty
        Write-Debug "Updating `"$Path`" with declation $($PSCmdlet.ParameterSetName): [$($Value -join ',')]."
        switch ($PSCmdlet.ParameterSetName) {
            # 'NewLine' {
            #     $EmptyStringArray | Write-Line -Path $Path
            # }
            'Comment' {
                # $CommentPrefix + ($Value ?? $EmptyStringArray) | # Value can be null here
                $CommentPrefix + ($Value ?? $EmptyStringArray) |
                    Write-Line -Path $Path -LineContinuation '# ' -LineBreak '' -NoIndentBreak
            }
            'Variable' {
                [string[]]$Variable + '=' + ($Value ?? $EmptyStringArray) |
                    Write-Line -Indent $Indent -Path $Path
            }
            'Pool' {
                $PoolPrefix + $Pool | Write-Line -Path $Path
                Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable depth -Value $Depth > $null
            }
            'Rule' {
                $RulePrefix + $Rule | Write-Line -Path $Path
                Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable command -Value $Value
                if ( $Description ) {
                    Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable description -Value $Description
                }
                if ( $Depfile ) {
                    Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable depfile -Value $Depfile
                }
                if ( $Generator ) {
                    Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable generator -Value 1
                }
                if ( $Pool ) {
                    Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable pool -Value $Pool
                }
                if ( $Restat ) {
                    Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable restat -Value 1
                }
                if ( $Rspfile ) {
                    Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable rspfile -Value $Rspfile
                }
                if ( $Rspfile_content ) {
                    Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable rspfile_content -Value $Rspfile_content
                }
                if ( $Deps ) {
                    Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable deps -Value $deps
                }
            }
            'Build' {
                [string[]]$Build = $Build | Get-NonEmpty | Format-Path
                if ( -not $Build ) {
                    throw 'Build targets cannot be empty.'
                }
                [string[]]$ImplicitOutputs = $ImplicitOutputs | Get-NonEmpty | Format-Path
                [string[]]$Implicit = $Implicit | Get-NonEmpty | Format-Path
                [string[]]$OrderOnly = $OrderOnly | Get-NonEmpty | Format-Path

                [string[]]$out = $BuildPrefix + $Build
                if ( $ImplicitOutputs ) {
                    [string[]]$out = $out + '|' + $ImplicitOutputs
                }
                $out[-1] = $out[-1] + ':' # format is build <target>: <deps>

                [string[]]$in = ($Value ?? $EmptyStringArray) | Format-Path
                if ( $Implicit ) {
                    [string[]]$in = $in + '|' + $Implicit
                }
                if ( $OrderOnly ) {
                    [string[]]$in = $in + '||' + $OrderOnly
                }

                $out + $Rule + $in | Write-Line -Path $Path

                if ( $Pool ) {
                    Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable pool -Value $Pool
                }
                if ( $DynDep ) {
                    Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable dyndep -Value $DynDep
                }

                if ( $Variables ) {
                    foreach ($v in $Variables.GetEnumerator()) {
                        Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable $v.Key -Value $v.Value
                    }
                }

                if ( $Default ) {
                    Update-NinjaFile -Quiet -Path $Path -Default -Value $Value
                }
            }
            'Include' {
                $IncludePrefix + $Include | Write-Line -Path $Path
            }
            'Subninja' {
                $SubNinjaPrefix + $SubNinja | Write-Line -Path $Path
            }
            'Default' {
                $DefaultPrefix + $Value | Write-Line -Path $Path
            }
        }

        if ( $NewLine ) {
            '' | Out-File -FilePath $Path -Append
        }

        if ( -not $Quiet ) { $Path }
    }
}

New-Alias -Name unja -Value Update-NinjaFile

function Write-Line {
    [CmdletBinding(PositionalBinding = $False,
        SupportsShouldProcess)]
    param (
        [Parameter(Mandatory,
            ValueFromPipeline,
            ValueFromPipelineByPropertyName)]
        $Line,

        [string]
        $Separator = ' ',

        [int]
        $Indent = 0,

        # triggers a line break if the current width is over this
        [int]
        $LineWidth = $DefaultLineWidth,

        [string]
        $LineBreak = '$',

        [string]
        $LineContinuation = '',

        [switch]
        $NoIndentBreak,

        [Parameter(Mandatory)]
        [string]
        $Path
    )
    begin {
        $first = $True
        $s = New-Object -TypeName 'System.Text.StringBuilder'
        [void]$s.Append( $IndentValue * $Indent)

        $newline = $Separator + $LineBreak + "`n"
        $ind = $IndentValue * $Indent + $LineContinuation
        # increment indent level to indent breaks more that parent line
        [void] ($NoIndentBreak) || ($ind += $IndentValue)

        $n = $s.Length
        $sep_n = $Separator.Length
        $ind_n += $IndentValue.Length
    }
    process {
        if ( -not $Line ) {
            return
        }

        if ($n -ge $LineWidth ) {
            [void]$s.AppendJoin('', $newline, $ind)
            $n = $ind_n
        }

        if ( -not $first ) {
            if ( $Separator ) {
                [void]$s.Append($Separator)
                $n += $sep_n
            }
        } else {
            $first = $False
        }

        [void]$s.Append($Line)
        $n += $Line.Length
    }
    end {
        $v = $s.ToString()
        if ( $PSCmdlet.ShouldProcess("Appending `"$v`" to ninja build file `"$Path`"", $Path, 'Write-Line') ) {
            $v | Out-File -FilePath $Path -Append
        }
    }
}

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

function Resolve-Command {
    [CmdletBinding()]
    [OutputType([string])]
    param (
        [Parameter(Mandatory)]
        [string]
        $Name,

        [string]
        $Path = ''
    )

    if ( -not $Path ) {
        $Path = (Get-Command $Name).Source 2>$null
    }

    if ( -not $Path ) {
        throw "Could not find executable `"$Name`" on the system."
    }

    if ( -not (Test-Path -Path $Path) ) {
        Write-Warning "Invalid path `"$Path`" to executable `"$Name`"."
        # try again, but search for the command instead
        # hopefully this isnt a stack overflow...
        return Resolve-Command $Name
    }

    return $Path
}

function Resolve-PathError {
    [CmdletBinding()]
    [OutputType([string])]
    param (
        [Parameter(Mandatory)]
        [string]
        $Path,

        [string]
        $Name
    )

    if ( -not $Name ) {
        $Name = 'Path'
    }

    if ( -not $Path ) {
        throw "$Name cannot be an empty path."
    }

    $p = Resolve-Path $Path 2>$null
    if ( -not $p ) {
        throw "Could not resolve $Name (`"$Path`") on the system."
    }

    return $p
}

<#
.SYNOPSIS
Trims strings and returns the non-empty and non-null results.
#>
filter Get-NonEmpty {
    $s = ( $_ -is [string] ) ? $_.Trim() : $_
    if ( $s ) { $s }
}

<#
.SYNOPSIS
Escape spaces(' ') and colons (':') within a string (but not '$')
#>
function Format-Path {
    [CmdletBinding()]
    param (
        [Parameter(Mandatory,
            ValueFromPipeline,
            ValueFromPipelineByPropertyName)]
        $Path
    )

    process {
        if ( $Path -and $Path -is [string]) {
            $Path = $Path.Trim().Replace(' ', '$ ').Replace(':', '$:')
        }
        $Path
    }
}
New-Alias -Name fp -Value Format-Path

<#
.SYNOPSIS
Prepends the value with a `$` and wraps it with the specified quotes.
If specified, $Left and $Right take precedence over $quote.
#>
function Format-Variable {
    [CmdletBinding(DefaultParameterSetName = 'Quote')]
    [OutputType([string])]
    param (
        [Parameter(Mandatory, Position = 0,
            ValueFromPipeline,
            ValueFromPipelineByPropertyName)]
        [Alias('v')]
        [string]
        $Value,

        [Alias('b')]
        [switch]
        $Bracket,

        [Parameter(ParameterSetName = 'Quote', Position = 1)]
        [Alias('q')]
        [string]
        $Quote = '',

        [Parameter(ParameterSetName = 'LeftRight', Position = 1)]
        [Alias('l')]
        [string]
        $Left = '',

        [Parameter(ParameterSetName = 'LeftRight', Position = 2)]
        [Alias('r')]
        [string]
        $Right = ''
    )

    process {
        if ( $Bracket ) {
            $Value = "{$Value}"
        }

        switch ($PSCmdlet.ParameterSetName) {
            'Quote' {
                $Left = $Quote
                $Right = $Quote
            }
        }
        "$Left`$$Value$Right"
    }
}
New-Alias -Name fv -Value Format-Variable

Export-ModuleMember `
    -Function Update-NinjaFile, `
    Get-GoModuleName, Get-GoEnv, Get-GoGenPackage, Get-ProtoFile, `
    Resolve-Command, Resolve-PathError `
    Format-Path, Format-Variable `
    -Alias unja, fv, fp
