# see details https://ninja-build.org/manual.html
# based off of https://github.com/ninja-build/ninja/blob/master/misc/ninja_syntax.py

Import-Module (Join-Path $PSScriptRoot GoUtils) -Scope Local -Force
Import-Module (Join-Path $PSScriptRoot Utils) -Scope Local -Force
# . "$PSScriptRoot\Utils.ps1"
# . "$PSScriptRoot\GoUtils.ps1"

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
        @{ N = 'DefaultLineWidthMin' ; V = 60 }
        @{ N = 'DefaultLineWidth' ; V = 80 }

        @{ N = 'PhonyRule' ; V = 'phony' }
        @{ N = 'DynDepRule' ; V = 'dyndep' }
    ) ) {
    Set-Variable -Name $d['N'] -Value $d['V'] -Option Readonly
}

# #
# .SYNOPSIS

# Updates a ninja build with a declaration.

# .DESCRIPTION

# Appends a declations (new line, commend, pool, rule, build, etc.) to the specified
# ninja build file ("ninja.build") be default.

# .LINK

# https://ninja-build.org/manual.html

# #


function Add-Default {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Position = 0, ValueFromRemainingArguments)]
        [string[]]
        # The build targets to set as default
        $Value,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('nl')]
        [switch]
        # Add a newline.
        $NewLine,

        [Alias('q')]
        [switch]
        # Surpress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    process {
        $Value = [string[]]($Value | Get-NonEmpty)
        Write-Debug "Adding defaults to `"$Path`" : [$($Value -join ',')]."

        $DefaultPrefix + $Value | Write-Line -Path $Path

        Update-File -Path $Path -Quiet:$Quiet -NewLine:$NewLine
    }
}

function Add-SubNinja {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Position = 0, Mandatory)]
        [string]
        # The subninja file to include
        $SubNinja,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('nl')]
        [switch]
        # Add a newline.
        $NewLine,

        [Alias('q')]
        [switch]
        # Surpress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    process {
        Write-Debug "Adding subninja `"$SubNinja`" to `"$Path`""

        $SubNinjaPrefix + $SubNinja | Write-Line -Path $Path

        Update-File -Path $Path -Quiet:$Quiet -NewLine:$NewLine
    }
}

function Add-Include {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Position = 0, Mandatory)]
        [string]
        # The file to include
        $Include,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('nl')]
        [switch]
        # Add a newline.
        $NewLine,

        [Alias('q')]
        [switch]
        # Surpress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    process {
        Write-Debug "Adding include `"$Include`" to `"$Path`""

        $IncludePrefix + $Include | Write-Line -Path $Path

        Update-File -Path $Path -Quiet:$Quiet -NewLine:$NewLine
    }
}

function Add-Phony {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Position = 0, Mandatory)]
        [string]
        # The phony build target
        $Build,

        [string[]]
        $Implicit,

        [string[]]
        $ImplicitOutput,

        [string[]]
        $OrderOnly,

        [switch]
        # Add a default declaration for this build
        $Default,

        [switch]
        # Don't escape paths
        $NoEscape,

        [Parameter(Position = 1, ValueFromRemainingArguments)]
        [string[]]
        # The phony build dependencies
        $Value,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('nl')]
        [switch]
        # Add a newline.
        $NewLine,

        [Alias('q')]
        [switch]
        # Surpress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    process {
        $Value = [string[]]($Value | Get-NonEmpty)
        Write-Debug "Adding phony statement `"$Build`" to `"$Path`": [$($Value -join ',')]."

        if ( -not ($Value) ) {
            throw 'Phony targets cannot be empty.'
        }

        Add-Build -Quiet -Path $Path -Build $Build -Rule $PhonyRule -Value $Value `
            -Implicit $Implicit -ImplicitOutput $ImplicitOutput -OrderOnly $OrderOnly `
            -Default:$Default -NoEscape:$NoEscape

        Update-File -Path $Path -Quiet:$Quiet -NewLine:$NewLine
    }
}
function Add-Dyndep {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Position = 0, Mandatory)]
        [string]
        # The dyndep build target
        $Build,

        [string[]]
        $Implicit,

        [string[]]
        $ImplicitOutput,

        [switch]
        $Restat,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('nl')]
        [switch]
        # Add a newline.
        $NewLine,

        [Alias('q')]
        [switch]
        # Surpress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    process {
        Write-Debug "Adding dyndep build `"$Build`" to `"$Path`""

        Add-Build -Quiet -Path $Path -Build $Build -Rule $DynDepRule `
            -Implicit $Implicit -ImplicitOutput $ImplicitOutput
        if ( $Restat ) {
            Add-Variable -Quiet -Path $Path -Indent 1 restat -Value 1
        }

        Update-File -Path $Path -Quiet:$Quiet -NewLine:$NewLine
    }
}


function Add-Build {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Position = 0, Mandatory)]
        [AllowEmptyString()]
        [string[]]
        # The build targets
        $Build,

        [string]
        # The rule name
        $Rule,

        [string[]]
        $Implicit,

        [string[]]
        $OrderOnly,

        [hashtable]
        $Variables,

        [string[]]
        $ImplicitOutput,

        [string]
        $DynDep,

        [string]
        # Specify the pool to use
        $Pool,

        [switch]
        # Add a default declaration for this build statement
        $Default,

        [string]
        # Add a phony declaration for this build statement
        $Phony,

        [switch]
        # Don't escape paths
        $NoEscape,

        [Parameter(Position = 1, ValueFromRemainingArguments)]
        [string[]]
        # The build statement dependencies
        $Value,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('nl')]
        [switch]
        # Add a newline.
        $NewLine,

        [Alias('q')]
        [switch]
        # Surpress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    process {
        $Value = [string[]]($Value | Get-NonEmpty)
        Write-Debug "Adding build statement `"$Build`" to `"$Path`" : [$($Value -join ',')]."

        $Build = [string[]]($Build | Get-NonEmpty)
        $ImplicitOutput = [string[]]($ImplicitOutput | Get-NonEmpty)
        if ( -not ($Build -or $ImplicitOutput) ) {
            throw 'Build targets cannot be empty.'
        }

        $Implicit = [string[]]($Implicit | Get-NonEmpty)
        $OrderOnly = [string[]]($OrderOnly | Get-NonEmpty)

        if ( -not $NoEscape ) {
            # $Build = ($Build ?? $EmptyStringArray) | Format-Path
            # $ImplicitOutput = ($ImplicitOutput ?? $EmptyStringArray) | Format-Path
            # $Implicit = ($Implicit ?? $EmptyStringArray) | Format-Path
            # $OrderOnly = ($OrderOnly ?? $EmptyStringArray) | Format-Path
            # $Value = ($Value ?? $EmptyStringArray) | Format-Path
            $Build = $Build | Format-Path
            $ImplicitOutput = $ImplicitOutput | Format-Path
            $Implicit = $Implicit | Format-Path
            $OrderOnly = $OrderOnly | Format-Path
            $Value = $Value | Format-Path
        }

        [string[]]$out = $BuildPrefix + $Build
        if ( $ImplicitOutput ) {
            [string[]]$out = $out + '|' + $ImplicitOutput
        }
        $out[-1] = $out[-1] + ':' # format is build <target>: <deps>

        $in = [string[]]($Value ?? $EmptyStringArray)
        if ( $Implicit ) {
            [string[]]$in = $in + '|' + $Implicit
        }
        if ( $OrderOnly ) {
            [string[]]$in = $in + '||' + $OrderOnly
        }

        $out + $Rule + $in | Write-Line -Path $Path

        if ( $Pool ) {
            Add-Variable -Quiet -Path $Path -Indent 1 pool -Value $Pool
        }
        if ( $DynDep ) {
            Add-Variable -Quiet -Path $Path -Indent 1 dyndep -Value $DynDep
        }

        if ( $Variables ) {
            foreach ($v in $Variables.GetEnumerator()) {
                Add-Variable -Quiet -Path $Path -Indent 1 $v.Key -Value $v.Value
            }
        }

        if ( $Default ) {
            Add-Default -Quiet -Path $Path -Value $Value
        }

        if ( $Phony  ) {
            Add-Phony -Quiet -Path $Path -Build $Phony -Value $Build -NoEscape
        }

        Update-File -Path $Path -Quiet:$Quiet -NewLine:$NewLine
    }
}

function Add-Rule {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Position = 0, Mandatory)]
        [string]
        # The rule name
        $Rule,

        [string[]]
        $Description = '',

        [string]
        $Depfile = '',

        [switch]
        $Generator,

        [switch]
        $Restat,

        [string]
        $Rspfile = '',

        [string[]]
        $Rspfile_content = '',

        [ValidateSet('gcc', 'msvc')]
        [string]
        $Deps = '',

        [string]
        # Specify the pool to use
        $Pool,

        [Parameter(Position = 1, ValueFromRemainingArguments)]
        [string[]]
        # The value of the comment, variable, build, rule, or default declaration
        $Value,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('nl')]
        [switch]
        # Add a newline.
        $NewLine,

        [Alias('q')]
        [switch]
        # Surpress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    process {
        $Value = [string[]]($Value | Get-NonEmpty)
        Write-Debug "Adding  `"`" to `"$Path`" : [$($Value -join ',')]."

        $RulePrefix + $Rule | Write-Line -Path $Path
        Add-Variable -Quiet -Path $Path -Indent 1 command -Value $Value

        if ( $Description ) {
            Add-Variable -Quiet -Path $Path -Indent 1 description -Value $Description
        }
        if ( $Depfile ) {
            Add-Variable -Quiet -Path $Path -Indent 1 depfile -Value $Depfile
        }
        if ( $Generator ) {
            Add-Variable -Quiet -Path $Path -Indent 1 generator -Value 1
        }
        if ( $Pool ) {
            Add-Variable -Quiet -Path $Path -Indent 1 pool -Value $Pool
        }
        if ( $Restat ) {
            Add-Variable -Quiet -Path $Path -Indent 1 restat -Value 1
        }
        if ( $Rspfile ) {
            Add-Variable -Quiet -Path $Path -Indent 1 rspfile -Value $Rspfile
        }
        if ( $Rspfile_content ) {
            Add-Variable -Quiet -Path $Path -Indent 1 rspfile_content -Value $Rspfile_content
        }
        if ( $Deps ) {
            Add-Variable -Quiet -Path $Path -Indent 1 deps -Value $deps
        }

        Update-File -Path $Path -Quiet:$Quiet -NewLine:$NewLine
    }
}

function Add-Pool {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Position = 0, Mandatory)]
        [string]
        # The pool name
        $Pool,

        [Parameter(Mandatory)]
        [int]
        $Depth,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('nl')]
        [switch]
        # Add a newline.
        $NewLine,

        [Alias('q')]
        [switch]
        # Surpress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    process {
        $Value = [string[]]($Value | Get-NonEmpty)
        Write-Debug "Adding pool `"$Pool`" to `"$Path`" : [$($Value -join ',')]."

        $PoolPrefix + $Pool | Write-Line -Path $Path
        Add-Variable -Quiet -Path $Path -Indent 1 depth -Value $Depth

        Update-File -Path $Path -Quiet:$Quiet -NewLine:$NewLine
    }
}

function Add-Variable {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Position = 0, Mandatory)]
        [string]
        # The variable name
        $Variable,

        [Parameter(Position = 1, ValueFromRemainingArguments)]
        [string[]]
        # The value of the variable
        $Value,

        [int]
        $Indent = 0,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('nl')]
        [switch]
        # Add a newline.
        $NewLine,

        [Alias('q')]
        [switch]
        # Surpress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    process {
        $Value = [string[]](($Value | Get-NonEmpty) ?? $EmptyStringArray)
        Write-Debug "Adding variable `"$Variable`" to `"$Path`" : [$($Value -join ',')]."

        [string[]]$Variable + '=' + $Value |
            Write-Line -Indent $Indent -Path $Path

        Update-File -Path $Path -Quiet:$Quiet -NewLine:$NewLine
    }
}

function Add-Comment {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Position = 0, ValueFromRemainingArguments)]
        [string[]]
        # The value of the comment
        $Value,

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('nl')]
        [switch]
        # Add a newline.
        $NewLine,

        [Alias('q')]
        [switch]
        # Surpress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    process {
        $Value = [string[]]($Value | Get-NonEmpty)
        Write-Debug "Adding comment to `"$Path`" : [$($Value -join ',')]."

        $CommentPrefix + ($Value ?? $EmptyStringArray) |
            Write-Line -Path $Path -LineContinuation '#' -LineBreak '' -NoIndentBreak

        Update-File -Path $Path -Quiet:$Quiet -NewLine:$NewLine
    }
}

function Add-NewLine {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('q')]
        [switch]
        # Surpress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )
    process {
        Write-Debug "Adding newline to `"$Path`"."
        Update-File -Path $Path -Quiet:$Quiet -NewLine
    }
}

function Update-File {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [switch]
        $NewLine,

        [Parameter(Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('q')]
        [switch]
        # Surpress returning the path.
        $Quiet
    )

    if ( $NewLine ) {
        '' | Out-File -FilePath $Path -Append
    }

    if ( -not $Quiet ) { $Path }
}

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

        [int]
        # Triggers a line break if the current width is over this
        $LineWidth = $DefaultLineWidth,

        # Triggers a line break if the current width is over this and text to
        # write will be over $LineWidth
        [int]
        $LineWidthMin = $DefaultLineWidthMin,

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

        if (
            ($break) -or
            ($n -ge $LineWidth) -or
            (($n -ge $LineWidthMin) -and (($n + $l.Length) -ge $LineWidth))
        ) {
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
