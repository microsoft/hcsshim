function New-TestCommand {
    [CmdletBinding()]
    param (
        [Parameter(Mandatory)]
        [ValidateSet('Test', 'Bench', 'List')]
        [string]
        $Action,

        [Parameter(Mandatory)]
        [string]
        $Path,

        [Parameter(Mandatory)]
        [string]
        $Name,

        [Parameter(Mandatory)]
        [string]
        $OutDirectory,

        [string]
        $OutFile,

        [DateTime]
        $Date = (Get-Date),

        [string]
        $Note,

        # test parameters
        [switch]
        $Shuffle,

        # whether to enable verbose testing logs, ie the `-test.v` flag
        [switch]
        $TestVerbose,

        # the log level of the shim code itself (ie, logrus, via the `-log-level` flag)
        [string]
        [ValidateSet('', 'panic', 'fatal', 'error', 'warn', 'warning', 'info', 'debug', 'trace')]
        $LogLevel,

        [int]
        $Count,

        [string]
        $BenchTime,

        [string]
        $Timeout,

        [string]
        $Run,

        [string[]]
        $Features
    )
    Write-Verbose "creating $OutDirectory"
    New-Item -ItemType 'directory' -Path $OutDirectory -Force > $null

    $testcmd = "$Path `'-test.timeout=$Timeout`' `'-test.count=$Count`' "

    if ( $Shuffle ) {
        $testcmd += '''-test.shuffle=on'' '
    }

    if ( $TestVerbose ) {
        $testcmd += '''-test.v'' '
    }

    switch ( $Action ) {
        'List' {
            if ( -not $Run ) {
                $Run = '.'
            }
            $testcmd += "`'-test.list=$Run`' "
        }
        'Test' {
            if ( $Run ) {
                $testcmd += "`'-test.run=$Run`' "
            }
        }
        'Bench' {
            if ( -not $Run ) {
                $Run = '.'
            }
            $testcmd += '''-test.run=^#'' ''-test.benchmem'' ' + `
                "`'-test.bench=$Run`' `'-test.benchtime=$BenchTime`' "
        }
    }

    if ( -not [string]::IsNullOrWhiteSpace($LogLevel) ) {
        $testcmd += "'-log-level=$($LogLevel.ToLower())' "
    }

    foreach ( $Feature in $Features ) {
        $Feature = $Feature -replace ' ', ''
        if ( $Feature ) {
            $testcmd += "`'-feature=$Feature`' "
        }
    }

    $f = $Name + '-' + ($Action.ToLower())
    if ( $Note ) {
        $f += '-' + $Note
    }
    $out = Join-Path $OutDirectory "$f-$(Get-Date -Date $date -Format FileDateTime).txt"

    return $testcmd.Trim(), $out
}

function Invoke-TestCommand {
    [CmdletBinding()]
    param (
        [Parameter(Mandatory)]
        [string]
        $TestCmd,

        [string]
        $OutputFile = '',

        [string]
        $OutputCmd
    )
    Write-Verbose "Running command: $TestCmd"

    if ( -not $OutputFile ) {
        $OutputFile = 'nul'
    } else {
        Write-Verbose "Saving output to: $OutputFile"
    }

    Invoke-Expression $TestCmd |
        Tee-Object -Encoding utf8 -Append -FilePath $OutputFile

    if ( $OutputCmd -and $OutputFile -ne 'nul' ) {
        $oc = "$OutputCmd $OutputFile"
        if ( Test-Path -PathType Leaf $OutputFile ) {
            Write-Verbose "Running output command: $oc"
            Invoke-Expression $oc
        } else {
            Write-Warning "Cannot run output command with non-existant output file: $oc"
        }
    }
}
