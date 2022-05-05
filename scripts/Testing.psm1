function New-TestCommand {
    [CmdletBinding()]
    param (
        [ValidateSet('Test', 'Bench', 'List')]
        [alias('a')]
        [string]
        $Action = 'Bench',

        [Parameter(Mandatory)]
        [string]
        $Path,

        [Parameter(Mandatory)]
        [string]
        $Name,

        [Parameter(Mandatory)]
        [string]
        $OutDirectory ,

        [DateTime]
        $Date = (Get-Date),

        [string]
        $Note = '',

        # test parameters
        [alias('tv')]
        [switch]
        $TestVerbose = $false,

        [int]
        $Count = 1,

        [string]
        $BenchTime = '5s',

        [string]
        $Timeout = '10m',

        [string]
        $Run = '',

        [string]
        $Feature = ''
    )

    $OutDirectory = Resolve-Path $OutDirectory
    Write-Verbose "creating $OutDirectory"

    New-Item -ItemType 'directory' -Path $OutDirectory -Force > $null

    $testcmd = "$Path `'-test.timeout=$Timeout`' `'-test.shuffle=on`' `'-test.count=$Count`' "

    if ( $TestVerbose ) {
        $testcmd += ' ''-test.v'' '
    }

    switch ( $Action ) {
        'List' {
            if ( $Run -eq '' ) {
                $Run = '.'
            }
            $testcmd += " `'-test.list=$Run`' "
        }
        'Test' {
            if ( $Run -ne '' ) {
                $testcmd += " `'-test.run=$Run`' "
            }
        }
        'Bench' {
            if ( $Run -eq '' ) {
                $Run = '.'
            }
            $testcmd += ' ''-test.run=^#'' ''-test.benchmem'' ' + `
                " `'-test.bench=$Run`' `'-test.benchtime=$BenchTime`' "
        }
    }

    if ( $Feature -ne '' ) {
        $testcmd += " `'-feature=$Feature`' "
    }

    $f = $Name + '-' + $Action
    if ($Note -ne '' ) {
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
        $OutputFile = 'nul',

        [string]
        $OutputCmd,

        [switch]
        $Preamble,

        [DateTime]
        $Date = (Get-Date),

        [string]
        $Note
    )

    if ($OutputFile -eq '' ) {
        $OutputFile = 'nul'
    }

    Write-Verbose "Saving output to: $OutputFile"
    if ( $Preamble ) {
        & {
            Write-Output "test.date: $(Get-Date -Date $Date -UFormat '%FT%R%Z' -AsUTC)"
            if ( $Note -ne '' ) {
                Write-Output "note: $Note"
            }
            Write-Output "test.command: $TestCmdPreamble"
            Write-Output "pkg.commit: $(git rev-parse HEAD)"
        } | Tee-Object -Append -FilePath $OutputFile
    }

    Write-Verbose "Running command: $TestCmd"
    Invoke-Expression $TestCmd | Tee-Object -Append -FilePath $OutputFile

    if ( $OutputCmd -ne '' -and $OutputFile -ne 'nul' ) {
        $oc = "$OutputCmd $OutputFile"
        Write-Verbose "Running command: $oc"
        Invoke-Expression $oc
    }

}