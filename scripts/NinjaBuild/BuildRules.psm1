# see details https://ninja-build.org/manual.html
# based off of https://github.com/ninja-build/ninja/blob/master/misc/ninja_syntax.py

Import-Module (Join-Path $PSScriptRoot GoUtils) -Scope Local -Force
Import-Module (Join-Path $PSScriptRoot Utils) -Scope Local -Force

foreach ($d in @(
        @{ N = 'EmptyStringArray'  ; V = ([string[]]'') }
        @{ N = 'CommentPrefix' ; V = ([string[]]'#') }
        @{ N = 'PoolPrefix'; V = ([string[]]'pool') }
        @{ N = 'RulePrefix' ; V = ([string[]]'rule') }
        @{ N = 'BuildPrefix' ; V = ([string[]]'build') }
        @{ N = 'IncludePrefix' ; V = ([string[]]'include') }
        @{ N = 'SubNinjaPrefix' ; V = ([string[]]'subninja') }
        @{ N = 'DefaultPrefix' ; V = ([string[]]'default') }
        @{ N = 'IndentValue' ; V = '  ' }
        @{ N = 'DefaultLineWidth' ; V = 80 }
    ) ) {
    Set-Variable -Name $d['N'] -Value $d['V'] -Option Constant -Force
}

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

        [Parameter(ParameterSetName = 'Build')] # for dyndeps
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
        [AllowEmptyString()]
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
                    Write-Line -Path $Path -LineContinuation '#' -LineBreak '' -NoIndentBreak
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
                [string[]]$ImplicitOutputs = $ImplicitOutputs | Get-NonEmpty | Format-Path
                if ( -not ($Build -or $ImplicitOutputs) ) {
                    throw 'Build targets cannot be empty.'
                }

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

                if ( $Restat ) {
                    Update-NinjaFile -Quiet -Path $Path -Indent 1 -Variable restat -Value 1
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
        if ( -not $NoIndentBreak ) {
            $ind += $IndentValue
        }

        $n = $s.Length
        $sep_n = $Separator.Length
        $ind_n += $ind.Length
    }
    process {
        $l = $Line
        $break = $False
        if ($l -eq "`n" ) {
            # has to be exactly one character thats a newline
            $break = $True
        }
        $l = $l.Trim()
        $l = $l.Replace("`n", '')

        if ( (-not $l) -and (-not $break) ) {
            return
        }

        if ( ($n -ge $LineWidth) -or ($break) ) {
            [void]$s.AppendJoin('', $newline, $ind)
            $n = $ind_n
            if ( -not $LineContinuation ) {
                $first = $True
            }
        }

        if ( $break ) {
            return
        }

        if ( $first ) {
            $first = $False
        } else {
            if ( $Separator ) {
                [void]$s.Append($Separator)
                $n += $sep_n
            }
        }

        [void]$s.Append($l)
        $n += $l.Length
    }
    end {
        $v = $s.ToString()
        if ( $PSCmdlet.ShouldProcess("Appending `"$v`" to ninja build file `"$Path`"", $Path, 'Write-Line') ) {
            $v | Out-File -FilePath $Path -Append
        }
    }
}
