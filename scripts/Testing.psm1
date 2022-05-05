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

        [switch]
        $TestVerbose,

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

    return $testcmd, $out
}

function Invoke-TestCommand {
    [CmdletBinding()]
    param (
        [Parameter(Mandatory)]
        [string]
        $TestCmd,

        [string]
        $TestCmdPreamble = $TestCmd,

        [string]
        $OutputFile = '',

        [string]
        $OutputCmd,

        [switch]
        $Preamble,

        [DateTime]
        $Date = (Get-Date),

        [string]
        $Note
    )
    Write-Verbose "Running command: $TestCmd"

    if ( -not $OutputFile ) {
        $OutputFile = 'nul'
    } else {
        Write-Verbose "Saving output to: $OutputFile"
    }


    if ( $Preamble ) {
        & {
            Write-Output "test.date: $(Get-Date -Date $Date -UFormat '%FT%R%Z' -AsUTC)"
            if ( $Note ) {
                Write-Output "note: $Note"
            }
            Write-Output "test.command: $TestCmdPreamble"
            if ( Get-Command -ErrorAction Ignore 'git' ) {
                Write-Output "pkg.commit: $(git rev-parse HEAD 2>$null)"
            }
        } | Tee-Object -Encoding utf8 -FilePath $OutputFile
    }
    Invoke-Expression $TestCmd |
        Tee-Object -Encoding utf8 -Append -FilePath $OutputFile

    if ( $OutputCmd -and $OutputFile -ne 'nul' ) {
        $oc = "$OutputCmd $OutputFile"
        Write-Verbose "Running command: $oc"
        Invoke-Expression $oc
    }
}
